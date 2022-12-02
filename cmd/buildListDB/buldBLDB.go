package main

import (
	//"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/yangshenyi/ymodule/loadmod"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/mod/module"
)

var (
	batchNum         int
	maxCoreNum       int
	curCoreNum       int
	muxThread, muxDB sync.Mutex
)

type module_ struct {
	Path    string `json:"Path" bson:"Path"`
	Version string `json:"Version" bson:"Version"`
}

type depInfo struct {
	Path        string `json:"Path" bson:"Path"`
	Version     string `json:"Version" bson:"Version"`
	HasValidMod int    `bson:"HasValidMod"`
	IsOnPkg     bool   `bson:"IsOnPkg"`
	Mod         struct {
		DirRequire []module_ `bson:"DirRequire"`
	} `bson:"mod"`
}

// id	path	version		buildList(with replace applied)		state(1 success; -1 error; -2 could not analyze)

type BLEle struct {
	Path        string `json:"Path" bson:"Path"`
	Version     string `json:"Version" bson:"Version"`
	IsDir bool `bson:"IsDirect"`
}

type BLDBEle struct {
	Path      string  `json:"Path" bson:"Path"`
	Version   string  `json:"Version" bson:"Version"`
	BuildList []BLEle `bson:"BuildList"`
	State     int     `bson:"State"`
}

// 4k a batch
// 10 cores
func runBatch(target []depInfo, collection *mongo.Collection, httpClient *http.Client, collectionBL *mongo.Collection, start int, dbNum int) {
	newValue := make([]interface{}, 0)

	for _, item := range target {
		mg, ok, info := loadmod.LoadModGraph(module.Version{Path: item.Path, Version: item.Version}, collection, httpClient)
		if ok == -1 {
			//fmt.Println("load Graph fail[!]", item.Path, item.Version)
			newValue = append(newValue, BLDBEle{item.Path, item.Version, nil, -2})
		} else if ok == -2 {
			//fmt.Println("load Graph fail[!]", item.Path, item.Version)
			newValue = append(newValue, BLDBEle{item.Path, item.Version, nil, -1})
		} else {
			directMap := make(map[module_]bool, 0)
			for _, v := range item.Mod.DirRequire {
				directMap[v] = true
			}
			list := make([]BLEle, 0)
			for _, v := range mg.BuildList()[1:] {
				_, flagDir := directMap[module_{v.Path, v.Version}]

				if k, ok := (*info)[v]; ok {
					//fmt.Println(v.Path, v.Version, "=>", k.Path, k.Version)
					list = append(list, BLEle{k.Path, k.Version, flagDir})
				} else if k, ok := (*info)[module.Version{Path: v.Path, Version: ""}]; ok {
					//fmt.Println(v.Path, v.Version, "=>", k.Path, k.Version)
					list = append(list, BLEle{k.Path, k.Version, flagDir})
				} else {
					list = append(list, BLEle{v.Path, v.Version, flagDir})
				}
			}
			newValue = append(newValue, BLDBEle{item.Path, item.Version, list, 1})
		}
	}

	muxDB.Lock()
	fmt.Println("\n\n", curCoreNum, "\n")
	_, err := collectionBL.InsertMany(context.TODO(), newValue)
	if err != nil {
		log.Fatal(err)
	}
	muxDB.Unlock()

	muxThread.Lock()
	curCoreNum -= 1
	muxThread.Unlock()
	fmt.Println("***[FINISHED]batchNum", start, "\t", dbNum,"\n")
}

func main() {
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
	collectionBL := db.Collection("BLDB")
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

	cur, err := collection.Find(context.TODO(), bson.D{{}})
	if err != nil {
		log.Fatal(err)
	}

	var depInfoBatch []depInfo
	batchNum = 5000
	count := 0
	maxCoreNum = 10
	curCoreNum = 0
	totalNum := 0
	flagNum := 0

	for cur.Next(context.TODO()) {
		var elem depInfo
		err := cur.Decode(&elem)
		if err != nil {
			log.Fatal(err)
		}
		totalNum += 1
	
		//if totalNum%20000 == 0 {
		//	fmt.Println(totalNum)
		//}
		
		if !(elem.IsOnPkg && elem.HasValidMod == 1) {
			continue
		}
		flagNum += 1
		if flagNum <= 3000000 {
			continue
		}

		count += 1
		depInfoBatch = append(depInfoBatch, elem)
		if count == batchNum {
			count = 0
			for curCoreNum >= maxCoreNum {
			}
			muxThread.Lock()
			curCoreNum += 1
			muxThread.Unlock()
			fmt.Println("\nbatchNum", flagNum-5000+1, "\t", totalNum,"\n")
			go runBatch(depInfoBatch, collection, httpClient, collectionBL, flagNum-5000+1, totalNum)
			
			depInfoBatch = make([]depInfo, 0)
			/*if flagNum >= 3000000{
				break
			} */
		}
	}
	if len(depInfoBatch) > 0 {
		muxThread.Lock()
		curCoreNum += 1
		muxThread.Unlock()
		go runBatch(depInfoBatch, collection, httpClient, collectionBL, flagNum-5000+1, totalNum)
	}

	if err := cur.Err(); err != nil {
		log.Fatal(err)
	}

	
	for curCoreNum > 0 {
	}
	cur.Close(context.TODO())
}
