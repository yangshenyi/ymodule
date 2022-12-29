package main

import (
	//"bufio"

	"context"
	"crypto/tls"
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

type DBEle struct {
	ID      primitive.ObjectID `json:"_id" bson:"_id"`
	Path    string             `json:"Path" bson:"Path"`
	Version string             `json:"Version" bson:"Version"`
}

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

func downLoadModFile(versions []DBEle, collectionBL *mongo.Collection, client *http.Client) {
	if len(versions) > 0 {
		for ix, item := range versions {
			resp, err := client.Get("https://goproxy.cn/" + trans(item.Path) + "/@v/" + trans(item.Version) + ".mod")
			if err != nil {
				time.Sleep(8 * time.Second)
				resp, err = client.Get("https://goproxy.cn/" + trans(item.Path) + "/@v/" + trans(item.Version) + ".mod")
				if err != nil {
					resp, err = client.Get("https://proxy.golang.org/" + trans(item.Path) + "/@v/" + trans(item.Version) + ".mod")
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
			resp.Body.Close()
			filter := bson.D{{Key: "_id", Value: item.ID}}
			update := bson.D{
				{Key: "$set", Value: bson.D{
					{Key: "ModFile", Value: string(infoText)},
				}},
			}
			_, err = collectionBL.UpdateOne(context.TODO(), filter, update)
			if err != nil {
				log.Fatal(err)
			}
			if ix == len(versions)/2 {
				fmt.Println(item, infoText)
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
	collectionBL = db.Collection("modData")

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
	var elems []DBEle
	for cur.Next(context.TODO()) {

		var elem DBEle
		err := cur.Decode(&elem)
		if err != nil {
			log.Fatal(err)
		}

		if count > 11000000 && count%batchNum == 0 {
			fmt.Println(count)
			muxThread.Lock()
			curCoreNum++
			muxThread.Unlock()
			for curCoreNum > maxCoreNum {
			}
			go downLoadModFile(elems, collectionBL, httpClient)
			elems = make([]DBEle, 0)
		}
		count++
		if count < 9806615 {
			if count%100000 == 0 {
				fmt.Println(count)
			}
			continue
		} else if count > 12200000 {
			break
		}
		elems = append(elems, elem)
	}
	muxThread.Lock()
	curCoreNum++
	muxThread.Unlock()
	for curCoreNum > maxCoreNum {
	}
	go downLoadModFile(elems, collectionBL, httpClient)

	for curCoreNum > 0 {
	}
}
