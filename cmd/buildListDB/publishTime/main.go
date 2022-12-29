package main

import (
	//"bufio"

	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

/*
type BLEle struct {
	Path    string `json:"Path" bson:"Path"`
	Version string `json:"Version" bson:"Version"`
	IsDir   bool   `bson:"IsDirect"`
}
*/
type BLDBEle struct {
	ID      primitive.ObjectID `json:"_id" bson:"_id"`
	Path    string             `json:"Path" bson:"Path"`
	Version string             `json:"Version" bson:"Version"`
	//	BuildList []BLEle            `bson:"BuildList"`
	//	State     int                `bson:"State"`
}

type VersionInfo struct {
	Version     string `json:"Version"`
	PublishTime string `json:"Time"`
}

/*
type versionRange struct {
	Vleft  string
	Vright string
}
*/
var (
	batchNum   int
	maxCoreNum int
	curCoreNum int
	muxThread  sync.Mutex
)

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

func downLoadPublishTime(versions []BLDBEle, collectionBL *mongo.Collection, client *http.Client) {
	if len(versions) > 0 {
		for _, item := range versions {
			versioninfo := VersionInfo{}
			resp, err := client.Get("https://goproxy.cn/" + trans(item.Path) + "/@v/" + trans(item.Version) + ".info")
			if err != nil {
				time.Sleep(8 * time.Second)
				resp, err = client.Get("https://goproxy.cn/" + trans(item.Path) + "/@v/" + trans(item.Version) + ".info")
				if err != nil {
					resp, err = client.Get("https://proxy.golang.org/" + trans(item.Path) + "/@v/" + trans(item.Version) + ".info")
					if err != nil {
						logFile, err := os.OpenFile("./err.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
						if err != nil {
							panic(err)
						}
						log.SetOutput(logFile)
						log.Println(item)
					}
				}
			}

			infoText, err := ioutil.ReadAll(resp.Body)
			if err == nil {
				json.Unmarshal(infoText, &versioninfo)
			}
			resp.Body.Close()
			filter := bson.D{{Key: "_id", Value: item.ID}}
			update := bson.D{
				{Key: "$set", Value: bson.D{
					{Key: "PublishTime", Value: versioninfo.PublishTime},
				}},
			}
			_, err = collectionBL.UpdateOne(context.TODO(), filter, update)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
	muxThread.Lock()
	curCoreNum--
	muxThread.Unlock()
}

func main() {
	var (
		client       *mongo.Client
		err          error
		db           *mongo.Database
		collectionBL *mongo.Collection
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
	collectionBL = db.Collection("BLDB")

	cur, err := collectionBL.Find(context.TODO(), bson.D{{}})
	if err != nil {
		log.Fatal(err)
	}

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

	count := 0
	maxCoreNum = 10
	curCoreNum = 0
	batchNum = 10000
	var elems []BLDBEle
	for cur.Next(context.TODO()) {

		var elem BLDBEle
		err := cur.Decode(&elem)
		if err != nil {
			log.Fatal(err)
		}

		if count >= 900000 && count%batchNum == 0 {
			fmt.Println(count)
			muxThread.Lock()
			curCoreNum++
			muxThread.Unlock()
			for curCoreNum > maxCoreNum {
			}
			go downLoadPublishTime(elems, collectionBL, httpClient)
			elems = make([]BLDBEle, 0)
		}
		count++
		if count < 900000 {
			if count%100000 == 0 {
				fmt.Println(count)
			}
			continue
		}
		elems = append(elems, elem)
	}
	muxThread.Lock()
	curCoreNum++
	muxThread.Unlock()
	for curCoreNum > maxCoreNum {
	}
	go downLoadPublishTime(elems, collectionBL, httpClient)

	for curCoreNum > 0 {
	}
}
