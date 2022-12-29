package loadmod

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"

	//"time"

	"github.com/yangshenyi/ymodule/mymvs"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	//"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"

	"net/http"
	/*
		"crypto/tls"
		"net/url"
	*/)

type Version struct {
	Path    string `json:"Path" bson:"Path"`
	Version string `json:"Version" bson:"Version"`
}

type modInfo struct {
	Path    string `json:"Path" bson:"Path"`
	Version string `json:"Version" bson:"Version"`
	Mod     struct {
		ModulePath   string    `bson:"ModulePath"`
		GoVersion    string    `bson:"GoVersion"`
		DirRequire   []Version `bson:"DirRequire"`
		IndirRequire []Version `bson:"IndirRequire"`
		Exclude      []Version `bson:"Exclude"`
		Replace      []string  `bson:"Replace"`
		Retract      []string  `bson:"Retract"`
	} `bson:"mod"`
}

func pruningForGoVersion(goVersion string) bool {
	return semver.Compare("v"+goVersion, "v1.17") >= 0
}

func cmpVersion(v1, v2 string) int {
	if v2 == "" {
		if v1 == "" {
			return 0
		}
		return -1
	}
	if v1 == "" {
		return 1
	}
	return semver.Compare(v1, v2)
}

func replacement(mod module.Version, replace map[module.Version]module.Version) (fromVersion string, to module.Version) {
	if r, ok := replace[mod]; ok {
		return mod.Version, r
	}
	if r, ok := replace[module.Version{Path: mod.Path, Version: ""}]; ok {
		return "", r
	}
	return "", mod
}

func parseModFile(modFile string, recv *modInfo) {
	lines := strings.Split(string(modFile), "\n")
	if len(lines) < 2 {
		return
	}
	flag := false
	for _, line := range lines {
		var words []string = strings.Fields(line)
		//fmt.Println(modInfo[index])
		if len(words) == 0 || words[0] == "" || words[0][0] == '/' && words[0][1] == '/' {
			continue
		} else if words[0] == "module" {
			recv.Mod.ModulePath = words[1]
		} else if words[0] == "go" {
			recv.Mod.GoVersion = words[1]
		} else if words[0] == ")" {
			flag = false
		} else if flag {
			if len(words) == 1 {
				log.Fatal("local replace: resolve fail[0]")
				break
			} else if len(words) == 2 {
				recv.Mod.DirRequire = append(recv.Mod.DirRequire, Version{Path: words[0], Version: words[1]})
			} else {
				recv.Mod.IndirRequire = append(recv.Mod.IndirRequire, Version{Path: words[0], Version: words[1]})
			}
		} else if words[0] == "require" {
			if words[1] == "(" {
				flag = true
			} else if words[1] == "()" {
				continue
			} else {
				if len(words) < 3 {
					log.Fatal("local replace: resolve fail[1]")
					break
				}
				if len(words) == 3 {
					recv.Mod.DirRequire = append(recv.Mod.DirRequire, Version{Path: words[1], Version: words[2]})
				} else {
					recv.Mod.IndirRequire = append(recv.Mod.IndirRequire, Version{Path: words[1], Version: words[2]})
				}
			}
		}
	}
}

func getRequiredList(modinfo modInfo, excludeInfo map[module.Version]bool) []module.Version {
	var list []module.Version
	hasExclude := len(excludeInfo) > 0
	if modinfo.Mod.DirRequire != nil {
		list = make([]module.Version, 0)
		for _, v := range modinfo.Mod.DirRequire {
			if v.Path[0] == '"' {
				v.Path = v.Path[1 : len(v.Path)-1]
			}
			if !hasExclude {
				list = append(list, module.Version{Path: v.Path, Version: v.Version})
			} else if _, ok := excludeInfo[module.Version{Path: v.Path, Version: v.Version}]; !ok {
				list = append(list, module.Version{Path: v.Path, Version: v.Version})
			}
		}
	}
	if modinfo.Mod.IndirRequire != nil {
		for _, v := range modinfo.Mod.IndirRequire {
			if v.Path[0] == '"' {
				v.Path = v.Path[1 : len(v.Path)-1]
			}
			if !hasExclude {
				list = append(list, module.Version{Path: v.Path, Version: v.Version})
			} else if _, ok := excludeInfo[module.Version{Path: v.Path, Version: v.Version}]; !ok {
				list = append(list, module.Version{Path: v.Path, Version: v.Version})
			}
		}
	}
	module.Sort(list)
	return list
}

func LoadModGraph(target module.Version, collection *mongo.Collection, httpClient *http.Client) (*mymvs.Graph, int, *map[module.Version]module.Version) {

	/*
		// connect mongodb
		var (
			client     *mongo.Client
			err        error
			db         *mongo.Database
			collection *mongo.Collection
		)
		if client, err = mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:27017").SetConnectTimeout(10*time.Second)); err != nil {
			fmt.Print(err)
			return nil, false, nil
		}
		defer func() {
			if err := client.Disconnect(context.TODO()); err != nil {
				panic(err)
			}
		}()
		db = client.Database("godep")
		collection = db.Collection("modData")
	*/

	// load main module's mod info
	var err error
	var targetModInfo modInfo = modInfo{}
	if err = collection.FindOne(context.TODO(), bson.M{"Path": target.Path, "Version": target.Version}).Decode(&targetModInfo); err != nil {
		fmt.Println(err, "read main module mod file fail! [1]")
		return nil, -1, nil
	}

	//fmt.Println("success", targetModInfo)

	// parse replace info into a map
	replaceInfo := make(map[module.Version]module.Version, len(targetModInfo.Mod.Replace))
	for _, line := range targetModInfo.Mod.Replace {
		words := strings.Fields(line)
		for ix, v := range words {
			if v == "//" {
				words = words[:ix]
			}
		}
		if words[0] == "//" {
			continue
		} else if len(words) <= 3 {
			continue
		} else if words[1] == "=>" {
			if len(words) == 3 {
				//fmt.Println("Replace by LocalPath [!]")
				replaceInfo[module.Version{Path: words[0], Version: ""}] = module.Version{Path: words[2], Version: ""}
			} else if len(words) == 4 {
				replaceInfo[module.Version{Path: words[0], Version: ""}] = module.Version{Path: words[2], Version: words[3]}
			}
		} else if words[2] == "=>" {
			if len(words) == 4 {
				//fmt.Println("Replace by LocalPath [!]")
				replaceInfo[module.Version{Path: words[0], Version: words[1]}] = module.Version{Path: words[3], Version: ""}
			} else if len(words) == 5 {
				replaceInfo[module.Version{Path: words[0], Version: words[1]}] = module.Version{Path: words[3], Version: words[4]}
			}
		} else {
			fmt.Println("Replace resolve fail")
			return nil, -2, nil
		}
	}
	// parse exclude info into a map
	excludeInfo := make(map[module.Version]bool, len(targetModInfo.Mod.Exclude))
	for _, v := range targetModInfo.Mod.Exclude {
		excludeInfo[module.Version{Path: v.Path, Version: v.Version}] = true
	}

	// construct requirement graph
	mg := mymvs.NewGraph(cmpVersion, []module.Version{{Path: target.Path, Version: ""}})
	// does main module enable prune ?
	pruning := pruningForGoVersion(targetModInfo.Mod.GoVersion)
	// add main module's explicit requirements into dependency graph
	roots_ := getRequiredList(targetModInfo, excludeInfo)
	var roots []module.Version
	for _, v := range roots_ {
		if v.Path != target.Path {
			roots = append(roots, v)
		}
	}
	module.Sort(roots)
	mg.Require(module.Version{Path: target.Path, Version: ""}, roots)

	// has a module been expanded?
	expandedQueue := make(map[module.Version]bool, len(roots))
	// queue of modules waiting for expanding
	//expandingQueue := make(map[module.Version]bool, len(roots))
	// load transitive dependency
	// add successor nodes of selected node with replace and exclude applied

	flagReplaceLocalError := false
	flagOtherError := false

	loadOne := func(m module.Version) ([]module.Version, bool, error) {
		_, actual := replacement(m, replaceInfo)

		// load current module's mod info
		var currentModInfo modInfo = modInfo{}

		//fmt.Println("*****", m, "&&", actual)
		if actual.Version != "" {
			if err = collection.FindOne(context.TODO(), bson.M{"Path": actual.Path, "Version": actual.Version}).Decode(&currentModInfo); err != nil {
				//fmt.Println(err, "read current module", m.Path, m.Version, "=>", actual.Path, actual.Version, "mod file fail! [1]")
				flagOtherError = true
				return nil, false, err
			}
			// resolve local path
		} else {
			var finalPath string = ""
			if filepath.IsAbs(actual.Path) {
				fmt.Println("abs")
				flagOtherError = true
				return nil, false, errors.New("!")
			} else {
				//fmt.Println(target.Path, actual.Path)
				urlPath := target.Path
				words := strings.Split(urlPath, "/")
				/*
					//set proxy
					proxyUrl := "http://127.0.0.1:7890"
					proxy, _ := url.Parse(proxyUrl)
					tr := &http.Transport{
						Proxy:           http.ProxyURL(proxy),
						TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
					}
					httpClient := &http.Client{
						Transport: tr,
						Timeout:   time.Second * 120,
					}
				*/

				if words[0] != "github.com" {
					// 还可以在 pkg 上查一下，获取 github 仓库 url
					//fmt.Println("??????")
					flagReplaceLocalError = true
					return nil, false, errors.New("!")
				}

				// consider target virtual Path
				flagVirtual := true
				if words[len(words)-1][0] == 'v' {
					for _, v := range words[len(words)-1][1:] {
						if v < '0' && v > '9' {
							flagVirtual = false
							break
						}
					}
				} else {
					flagVirtual = false
				}

				if flagVirtual {
					urlPath = strings.Join(words[:len(words)-1], "/")
				}

				finalPath = strings.Join(strings.Split(filepath.Join(urlPath, actual.Path), "\\"), "/")
				//fmt.Println(flagVirtual, finalPath)

				var finalVersion string
				if v := strings.Split(target.Version, "-"); len(v) > 2 {
					finalVersion = v[len(v)-1]
				} else {
					// submodule label?
					labelPrefix := ""
					if v := strings.Split(urlPath, "/"); len(v) > 3 {
						labelPrefix = strings.Join(v[3:], "/")
					}
					//fmt.Println(labelPrefix)
					if v := strings.Split(target.Version, "+"); len(v) > 1 {
						finalVersion = labelPrefix + v[0]
					} else {
						finalVersion = labelPrefix + "/" + target.Version
					}
				}
				//fmt.Println(finalVersion)

				//fmt.Println("https://raw.githubusercontent.com/" + words[1] + "/" + words[2] + "/" + finalVersion + "/" + strings.Join(strings.Split(finalPath, "/")[3:], "/") + "/go.mod")
				//fmt.Println(target, finalPath)
				if len(strings.Split(finalPath, "/")) <= 2 {
					flagOtherError = true
					return nil, false, errors.New("!")
				}
				resp, err := httpClient.Get("https://raw.githubusercontent.com/" + words[1] + "/" + words[2] + "/" + finalVersion + "/" + strings.Join(strings.Split(finalPath, "/")[3:], "/") + "/go.mod")
				// consider subdirectory
				if flagVirtual && resp.StatusCode != 200 {
					finalPath = strings.Join(strings.Split(filepath.Join(target.Path, actual.Path), "\\"), "/")
					resp, err = httpClient.Get("https://raw.githubusercontent.com/" + words[1] + "/" + words[2] + "/" + finalVersion + "/" + strings.Join(strings.Split(finalPath, "/")[3:], "/") + "/go.mod")
				}
				if err != nil {
					flagOtherError = true
					return nil, false, errors.New("!")
				}
				modFile, _ := ioutil.ReadAll(resp.Body)
				//fmt.Println(string(modFile))
				parseModFile(string(modFile), &currentModInfo)
			}
		}
		if reqs, ok := mg.RequiredBy(m); !ok {
			requiredList := getRequiredList(currentModInfo, excludeInfo)
			mg.Require(m, requiredList)
			return requiredList, pruningForGoVersion(currentModInfo.Mod.GoVersion), nil
		} else {
			return reqs[:len(reqs):len(reqs)], pruningForGoVersion(currentModInfo.Mod.GoVersion), nil
		}

	}

	var enqueue func(m module.Version, pruning bool)
	enqueue = func(m module.Version, pruning bool) {
		if m.Version == "none" {
			return
		} else if v := strings.Split(target.Version, "+"); v[len(v)-1] == "incompatible" {
			return
		}

		if !pruning {
			if _, ok := expandedQueue[m]; ok {
				return
			}
		}

		requireList, curPruning, err := loadOne(m)
		if err != nil {
			flagOtherError = true
			return
		}

		if !pruning || !curPruning {
			nextPruning := curPruning
			if !pruning {
				nextPruning = false
			}
			expandedQueue[m] = true
			for _, r := range requireList {
				/*if r.Path == "gihub.com/go-kit/kit@v0.12.0" {{
					fm.Println(m, r)
				}*/
				enqueue(r, nextPruning)
			}
		}

	}
	//fmt.Println(roots)
	for _, m := range roots {
		//fmt.Println("(((((((", m)
		enqueue(m, pruning)
	}
	if flagReplaceLocalError {
		return mg, -1, &replaceInfo
	} else if flagOtherError {
		return mg, -2, &replaceInfo
	}

	return mg, 1, &replaceInfo
}
