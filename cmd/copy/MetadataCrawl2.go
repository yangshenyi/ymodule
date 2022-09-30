package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type module struct {
	Path    string `json:"Path" bson:"Path"`
	Version string `json:"Version" bson:"Version"`
}

type dep struct {
	Path      string `json:"Path" bson:"Path"`
	Version   string `json:"Version" bson:"Version"`
	CacheTime string `json:"Timestamp" bson:"CacheTime"`
	Mod       struct {
		ModulePath   string   `bson:"ModulePath"`
		GoVersion    string   `bson:"GoVersion"`
		DirRequire   []module `bson:"DirRequire"`
		IndirRequire []module `bson:"IndirRequire"`
		Exclude      []module `bson:"Exclude"`
		Replace      []string `bson:"Replace"`
		Retract      []string `bson:"Retract"`
	} `bson:"mod"`
	HasValidMod int  `bson:"HasValidMod"`
	IsValidGo   bool `bson:"IsValidGo"`
	IsOnPkg     bool `bson:"IsOnPkg"`
}

var numOfThread int = 0
var muxThread, muxDB sync.Mutex

/*
type publishTime struct {
	Version, Time string
}
*/

func expHandler(cacheTime string, e error) {
	logFile, err := os.OpenFile("./err.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}
	log.SetOutput(logFile)
	log.Println(cacheTime, e)
}

func trans(src string) string {
	var ret string
	for _, val := range src {
		if val >= 'A' && val <= 'Z' {
			ret += "!" + string(val+'a'-'A')
		} else {
			ret += string(val)
		}
	}
	return ret
}

//each time, we parse 2000 records
//if fail, write the cachetime of first record and err info into a log file to reparse
func parse(modInfo []dep, client *http.Client, collection *mongo.Collection) {
	fmt.Println(modInfo[0].Path, modInfo[0].CacheTime)
	for index, val := range modInfo {
		resp, err := client.Get("https://proxy.golang.org/" + trans(val.Path) + "/@v/" + trans(val.Version) + ".mod")
		modInfo[index].HasValidMod = 1
		var modtext []byte
		var lines []string
		if err != nil {
			expHandler(modInfo[0].CacheTime, err)
			modInfo[index].HasValidMod = -3
		} else {
			if resp.StatusCode != 200 {
				modInfo[index].HasValidMod = 0
			}
			modtext, _ = ioutil.ReadAll(resp.Body)
			//parse mod file
			lines = strings.Split(string(modtext), "\n")
		}

		if modInfo[index].HasValidMod == -3 {
		} else if len(lines) > 2 {
			flagList := make([]bool, 4)
			for _, line := range lines {
				var words []string = strings.Fields(line)
				//fmt.Println(modInfo[index])
				if len(words) == 0 || words[0] == "" || words[0][0] == '/' && words[0][1] == '/' {
					continue
				} else if words[0] == "module" {
					modInfo[index].Mod.ModulePath = words[1]
				} else if words[0] == "go" {
					modInfo[index].Mod.GoVersion = words[1]
				} else if words[0] == ")" {
					for i := 0; i < 4; i++ {
						flagList[i] = false
					}
				} else if flagList[0] {
					if len(words) == 1 {
						modInfo[index].HasValidMod = -1
						break
					} else if len(words) == 2 {
						modInfo[index].Mod.DirRequire = append(modInfo[index].Mod.DirRequire, module{Path: words[0], Version: words[1]})
					} else {
						modInfo[index].Mod.IndirRequire = append(modInfo[index].Mod.IndirRequire, module{Path: words[0], Version: words[1]})
					}
				} else if flagList[1] {
					modInfo[index].Mod.Exclude = append(modInfo[index].Mod.Exclude, module{Path: words[0], Version: words[1]})
				} else if flagList[2] {
					modInfo[index].Mod.Replace = append(modInfo[index].Mod.Replace, line)
				} else if flagList[3] {
					modInfo[index].Mod.Retract = append(modInfo[index].Mod.Retract, line)
				} else if words[0] == "require" {
					if words[1] == "(" {
						flagList[0] = true
					} else if words[1] == "()" {
						continue
					} else {
						if len(words) < 3 {
							modInfo[index].HasValidMod = -1
							break
						}
						if len(words) == 3 {
							modInfo[index].Mod.DirRequire = append(modInfo[index].Mod.DirRequire, module{Path: words[1], Version: words[2]})
						} else {
							modInfo[index].Mod.IndirRequire = append(modInfo[index].Mod.IndirRequire, module{Path: words[1], Version: words[2]})
						}
					}
				} else if words[0] == "exclude" {
					if len(words) <= 1 {
						logFile, err := os.OpenFile("./err.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
						if err != nil {
							panic(err)
						}
						log.SetOutput(logFile)
						log.Println("*****************\n", modInfo[0].CacheTime, modInfo[index])
					} else if words[1] == "(" {
						flagList[1] = true
					} else {
						modInfo[index].Mod.Exclude = append(modInfo[index].Mod.Exclude, module{Path: words[1], Version: words[2]})
					}
				} else if words[0] == "replace" {
					if len(words) <= 1 {
						logFile, err := os.OpenFile("./err.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
						if err != nil {
							panic(err)
						}
						log.SetOutput(logFile)
						log.Println(modInfo[0].CacheTime, modInfo[index])
					} else if words[1] == "(" {
						flagList[2] = true
					} else {
						modInfo[index].Mod.Replace = append(modInfo[index].Mod.Replace, strings.Join(words[1:], " "))
					}
				} else if words[0] == "retract" {
					if len(words) <= 1 {
						logFile, err := os.OpenFile("./err.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
						if err != nil {
							panic(err)
						}
						log.SetOutput(logFile)
						log.Println("*****************\n", modInfo[0].CacheTime, modInfo[index])
					} else if words[1] == "(" {
						flagList[3] = true
					} else {
						modInfo[index].Mod.Retract = append(modInfo[index].Mod.Retract, strings.Join(words[1:], " "))
					}
				}
			}
		} else {
			modInfo[index].HasValidMod = 0
		}
		if resp != nil {
			resp.Body.Close()
		}
		//on pkg.go.dev?
		resp, err = client.Head("https://pkg.go.dev/" + val.Path)
		if err != nil {
			time.Sleep(time.Duration(10) * time.Second)
			resp, err = client.Head("https://pkg.go.dev/" + val.Path)
			if err != nil {
				muxThread.Lock()
				numOfThread--
				muxThread.Unlock()
				expHandler(modInfo[0].CacheTime, err)
				return
			}
		}
		if resp.StatusCode == 200 {
			modInfo[index].IsOnPkg = true
		} else {
			modInfo[index].IsOnPkg = false
		}
		resp.Body.Close()

		modInfo[index].IsValidGo = modInfo[index].IsOnPkg || modInfo[index].HasValidMod == 1
		//Path and modulePath is different
		if modInfo[index].HasValidMod == 1 {
			//fmt.Println("--------------", modInfo[index])
			modulePath_ := modInfo[index].Mod.ModulePath
			if modulePath_[0:1] == "\"" {
				modulePath_ = modulePath_[1 : len(modulePath_)-1]
			}
			if modInfo[index].Path != modulePath_ {
				modInfo[index].HasValidMod = -2
			}
		}
		/*if modInfo[index].HasValidMod == 0 {
			//fmt.Println("****************\n", modInfo[index].Path)
			fmt.Println(modInfo[index])
		}*/
		//fmt.Println(modInfo[index].Path, modInfo[index].Version, modInfo[index].HasValidMod)
	}
	//fmt.Println(modInfo)
	//store into DB

	newValue := make([]interface{}, 0)
	for _, v := range modInfo {
		newValue = append(newValue, v)
	}
	muxDB.Lock()
	fmt.Println("\n\n\n\n\n", numOfThread, "\n", newValue[0], modInfo[0].CacheTime)
	collection.InsertMany(context.TODO(), newValue)
	muxDB.Unlock()
	muxThread.Lock()
	numOfThread--
	muxThread.Unlock()
}

func main() {
	//set core
	runtime.GOMAXPROCS(runtime.NumCPU()) // 12 cores on my PC
	//maxThread := 8
	//Connect to mongodb
	var (
		client     *mongo.Client
		err        error
		db         *mongo.Database
		collection *mongo.Collection
	)
	if client, err = mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:27017").SetConnectTimeout(10*time.Second)); err != nil {
		fmt.Print(err)
		return
	}
	defer func() {
		if err := client.Disconnect(context.TODO()); err != nil {
			panic(err)
		}
	}()
	db = client.Database("godep")
	collection = db.Collection("modData")

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

	//initialize the crawl location
	//lastModCacheTime := "2022-08-25T09:15:51.774796Z"
	/*
		for {


			resp, err := httpClient.Get("https://index.golang.org/index?since=" + lastModCacheTime)
			if err != nil {
				expHandler(lastModCacheTime, err)
				for numOfThread > 0 {
				}
				return
			}
			resp.Close = true
			var modIndexes []dep
			dec := json.NewDecoder(resp.Body)
			for dec.More() {
				var modIndex dep
				if err := dec.Decode(&modIndex); err != nil {
					expHandler(lastModCacheTime, err)
					for numOfThread > 0 {
					}
					return
				}
				modIndexes = append(modIndexes, modIndex)
			}
			//index done
			if len(modIndexes) == 1 {
				break
			}
			lastModCacheTime = modIndexes[len(modIndexes)-1].CacheTime
	*/

	file, err := os.Open("./left.txt")
	if err != nil {
		panic(err)
	}
	defer file.Close()
	content, err := ioutil.ReadAll(file)
	lines := strings.Split(string(content), "\n")
	var modIndexes []dep
	for i := 0; i < len(lines)-1; i += 1 {
		var temp dep
		temp.Path = strings.Fields(lines[i])[0]
		temp.Version = strings.Fields(lines[i])[1]
		temp.CacheTime = strings.Fields(lines[i])[2]
		//fmt.Println(temp.CacheTime[len(temp.CacheTime)-1])
		modIndexes = append(modIndexes, temp)
	}

	//limit the num of goroutine
	/*
		for numOfThread >= maxThread {
		}
		muxThread.Lock()
		numOfThread++
		muxThread.Unlock()
	*/
	parse(modIndexes, httpClient, collection)

}

/*
	create table if not exists `goversion`(
		`id` int unsigned auto_increment,
		`path` varchar(100) not null,
		`version` varchar(100) not null,
		`timestamp` varchar(40) not null,
		`publishtime` varchar(40) not null,
		`isGopackage`    boolean default false,
		`isVersionOnDev` boolean default false,
		`validgomod` boolean default false,
		primary key(id)
		)ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=UTF8MB4 ;
*/

/*
func Parse(mod []module, client *http.Client, quit chan bool, db *sql.DB) {
	stmt, err := db.Prepare("INSERT INTO goversion(path, version, timestamp, publishtime, isGopackage, isVersionOnDev, validgomod) VALUES(?,?,?,?,?,?,?)")
	if err != nil {
		log.Fatal(err, "!!!")
	}
	defer stmt.Close()

	for _, val := range mod {
		//package on pkg.go.dev？
		resp, err := client.Head("https://pkg.go.dev/" + val.Path)
		if err != nil {
			log.Fatal("[Error]", err, 1, val)
		}
		if resp.StatusCode == 200 {
			val.IsPackageOnDev = true
			respMod, errMod := client.Get("https://proxy.golang.org/" + trans(val.Path) + "/@v/" + trans(val.Version) + ".mod")
			if errMod != nil {
				log.Fatal("[Error]", err, 2, val)
			}

			if respMod.StatusCode == 200 {
				bytesmod, _ := ioutil.ReadAll(respMod.Body)
				s := string(bytesmod)
				if len(strings.Split(s, " ")) > 2 {
					val.ValidMod = true
				}
			}
			respMod.Body.Close()

			//Publish Time
			//$base/$module/@v/$version.info
			respPubTime, errPubTime := client.Get("https://proxy.golang.org/" + trans(val.Path) + "/@v/" + trans(val.Version) + ".info")
			if errPubTime != nil {
				log.Fatal("[Error]", err, 3, val)
			}

			if respPubTime.StatusCode == 200 {
				dec := json.NewDecoder(respPubTime.Body)
				var temp publishTime
				dec.Decode(&temp)
				val.publishTime = temp.Time
			}
			respPubTime.Body.Close()


			//version on pkg.go.dev?
			respVersion, errVersion := client.Head("https://pkg.go.dev/" + val.Path + "@" + val.Version)
			if errVersion != nil {
				log.Fatal("[Error]", err, 4, val)
			}
			if respVersion.StatusCode == 200 {
				val.IsVersionOnDev = true
				/*
					//valid mod file?
					doc, err := goquery.NewDocumentFromReader(respVersion.Body)
					if err != nil {
						log.Fatal(err)
					}
					doc.Find("ul.UnitMeta-details").Each(func(i int, s *goquery.Selection) {
						if _, ok := s.Find("li").Eq(0).Find("a").Attr("href"); ok {
							val.ValidMod = true
						}
					})
			}
			respVersion.Body.Close()
		}
		resp.Body.Close()

		_, err1 := stmt.Exec(val.Path, val.Version, val.Timestamp, val.publishTime, val.IsPackageOnDev, val.IsVersionOnDev, val.ValidMod)
		if err1 != nil {
			log.Fatal(err, 6, val)
		}

		//fmt.Println(val)
	}
	quit <- true
}*/
