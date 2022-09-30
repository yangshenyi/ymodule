package loadmod

import (
	"context"
	"fmt"
	"time"

	"strings"
	//"errors"
	"golang.org/x/mod/semver"
	"github.com/yangshenyi/ymodule/mymvs"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/mod/module"
)
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
	if r, ok := replace[module.Version{Path: mod.Path}]; ok {
		return "", r
	}
	return "", mod
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

func LoadModGraph(target module.Version) (*mymvs.Graph, bool) {

	// connect mongodb
	var (
		client     *mongo.Client
		err        error
		db         *mongo.Database
		collection *mongo.Collection
	)
	if client, err = mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:27017").SetConnectTimeout(10*time.Second)); err != nil {
		fmt.Print(err)
		return nil, false
	}
	defer func() {
		if err := client.Disconnect(context.TODO()); err != nil {
			panic(err)
		}
	}()
	db = client.Database("godep")
	collection = db.Collection("modData")

	// load main module's mod info
	var targetModInfo modInfo = modInfo{}
	if err = collection.FindOne(context.TODO(), bson.M{"Path": target.Path, "Version": target.Version}).Decode(&targetModInfo); err != nil {
		fmt.Println(err, "read main module mod file fail! [1]")
		return nil, false
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
		} else if words[1] == "=>" {
			if len(words) == 3 {
				fmt.Println("Replace by LocalPath [!]")
				return nil, false
			} else if len(words) == 4 {
				replaceInfo[module.Version{Path: words[0], Version: ""}] = module.Version{Path: words[2], Version: words[3]}
			}
		} else if words[2] == "=>" {
			if len(words) == 4 {
				fmt.Println("Replace by LocalPath [!]")
				return nil, false
			} else if len(words) == 5 {
				replaceInfo[module.Version{Path: words[0], Version: words[1]}] = module.Version{Path: words[3], Version: words[4]}
			}
		} else {
			fmt.Println("Replace resolve fail")
			return nil, false
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
	roots := getRequiredList(targetModInfo, excludeInfo)
	module.Sort(roots)
	mg.Require(module.Version{Path: target.Path, Version: ""}, roots)

	// has a module been expanded?
	expandedQueue := make(map[module.Version]bool, len(roots))
	// queue of modules waiting for expanding
	//expandingQueue := make(map[module.Version]bool, len(roots))
	// load transitive dependency
	// add successor nodes of selected node with replace and exclude applied
	loadOne := func(m module.Version) ([]module.Version, bool, error) {
		/*if m.Path == target.Path {
			return nil, false, errors.New("Same Path as Target")
		}*/
		_, actual := replacement(m, replaceInfo)
		// load current module's mod info
		var currentModInfo modInfo = modInfo{}
		//fmt.Println("*****", m, "&&", actual)
		if err = collection.FindOne(context.TODO(), bson.M{"Path": actual.Path, "Version": actual.Version}).Decode(&currentModInfo); err != nil {
			// fmt.Println(err, "read current module", m.Path, m.Version, "=>", actual.Path, actual.Version, "mod file fail! [1]")
			return nil, false, err
		}

		requiredList := getRequiredList(currentModInfo, excludeInfo)
		mg.Require(m, requiredList)

		return requiredList, pruningForGoVersion(currentModInfo.Mod.GoVersion), nil
	}

	var enqueue func(m module.Version, pruning bool)
	enqueue = func(m module.Version, pruning bool) {
		if m.Version == "none" {
			return
		}
		if _, ok := expandedQueue[m]; ok {
			return
		}
		requireList, curPruning, err := loadOne(m)
		if err != nil {
			return
		}
		expandedQueue[m] = true

		if !pruning || !curPruning {
			nextPruning := curPruning
			if !pruning {
				nextPruning = false
			}
			for _, r := range requireList {
				enqueue(r, nextPruning)
			}
		}
	}
	//fmt.Println(roots)
	for _, m := range roots {
		//fmt.Println("((((((((", m)
		enqueue(m, pruning)
	}

	return mg, true
}

/*

// canonicalizeReplacePath ensures that relative, on-disk, replaced module paths
// are relative to the workspace directory (in workspace mode) or to the module's
// directory (in module mode, as they already are).
func canonicalizeReplacePath(r module.Version, modRoot string) module.Version {
	if filepath.IsAbs(r.Path) || r.Version != "" {e).
func canonicalizeReplacePath(r module.Version,modRoot string) module.Version {
	if filepath.IsAbs(r.Path) || r.Version != ""{
		returnrh.IsAbs(r.Path) || r.Version != "" {
		turn rh.IsAbs(r.Path) || r.Version != "" {
		turn r
	}
	workFilePath := WorkFiPath(
	if workFePath == ""modRoot, r.Path)
	i re err := filepath.Rel(filepath.Dir(workFilePat, abs); err == nil
		urn module.Version{Path: rel, Version: r.Version
	}
	// We couldn't make the version's path relative o the worksce's path
// so just return the absolute path. It's the best can do
trn module.Version{Path: abs, Version: r.Version


*/
