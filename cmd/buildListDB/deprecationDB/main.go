package main

import (
	//"bufio"

	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/mod/semver"
)

// package - deprecated?

type VersionInfo struct {
	Version   string
	Retracted bool
}

// DeprecationInfo
type depracationModuleInfo struct {
	Path               string        `json:"Path" bson:"Path"`
	Versions           []VersionInfo `json:"Versions" bson:"Versions"`
	Deprecated         bool          `json:"Deprecated" bson:"Deprecated"`
	DeprecationVersion string        `json:"DeprecationVersion" bson:"DeprecationVersion"`
	DeprecationTime    string        `json:"DeprecationTime" bson:"DeprecationTime"`
}

type BLEle struct {
	Path    string `json:"Path" bson:"Path"`
	Version string `json:"Version" bson:"Version"`
	IsDir   bool   `bson:"IsDirect"`
}

type BLDBEle struct {
	ID          primitive.ObjectID `json:"_id" bson:"_id"`
	Path        string             `json:"Path" bson:"Path"`
	Version     string             `json:"Version" bson:"Version"`
	State       int                `bson:"State"`
	Retracted   bool               `bson:"Retracted"`
	PublishTime string             `bson:"PublishTime"`
}

type module struct {
	Path    string `json:"Path" bson:"Path"`
	Version string `json:"Version" bson:"Version"`
}

type depInfo struct {
	Path    string `json:"Path" bson:"Path"`
	Version string `json:"Version" bson:"Version"`
	Mod     struct {
		ModulePath   string   `bson:"ModulePath"`
		GoVersion    string   `bson:"GoVersion"`
		DirRequire   []module `bson:"DirRequire"`
		IndirRequire []module `bson:"IndirRequire"`
		Exclude      []module `bson:"Exclude"`
		Replace      []string `bson:"Replace"`
		Retract      []string `bson:"Retract"`
	} `bson:"mod"`
	ModFile string `json:"modFile" bson:"ModFile"`
}

type entryWrapper struct {
	array []VersionInfo
	by    func(p, q *VersionInfo) bool
}

func (pw entryWrapper) Len() int {
	return len(pw.array)
}
func (pw entryWrapper) Swap(i, j int) {
	pw.array[i], pw.array[j] = pw.array[j], pw.array[i]
}
func (pw entryWrapper) Less(i, j int) bool {
	return pw.by(&pw.array[i], &pw.array[j])
}

var mymux, mymux2 sync.Mutex

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
	collectionS := db.Collection("modData")
	collectionBL = db.Collection("BLDB")
	//collectionR := db.Collection("DeprecationInfo")

	max := 10
	curN := 0

	cur, err := collectionBL.Find(context.TODO(), bson.D{{}})
	if err != nil {
		log.Fatal(err)
	}

	//deprecationML := make(map[string]depracationModuleInfo, 0)
	file, err := os.OpenFile("deprecation.txt", os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Println("文件打开失败", err)
	}
	write := bufio.NewWriter(file)

	var detectDeprecation func(modinfo *depInfo) bool
	detectDeprecation = func(modinfo *depInfo) bool {
		if semver.Compare("v"+modinfo.Mod.GoVersion, "v1.16") <= 0 {
			return false
		}
		modfile := modinfo.ModFile
		lines := strings.Split(modfile, "\n")
		for _, item := range lines {
			if item == "" {
				continue
			}
			words := strings.Fields(item)
			if len(words) > 1 && words[0] == "//" && words[1] == "Deprecated:" {
				return true
			}
		}

		return false
	}

	count := 0
	for cur.Next(context.TODO()) {
		count += 1
		if count%10000 == 0 {
			fmt.Println(count)
		}
		if count < 430000 {
			continue
		}
		var elem BLDBEle
		err := cur.Decode(&elem)
		if err != nil {
			log.Fatal(err)
		}
		if elem.State != 1 {
			continue
		}

		var ModInfo depInfo

		if curN >= max {

		}
		mymux.Lock()
		curN += 1
		mymux.Unlock()
		go func() {
			collectionS.FindOne(context.TODO(), bson.M{"Path": elem.Path, "Version": elem.Version}).Decode(&ModInfo)
			if detectDeprecation(&ModInfo) {
				mymux2.Lock()
				write.WriteString(fmt.Sprintln(elem.Path, ",", elem.Version, ",", elem.PublishTime))
				fmt.Println(fmt.Sprintln(elem.Path, ",", elem.Version, ",", elem.PublishTime, ModInfo.ModFile))
				mymux2.Unlock()
			}
			mymux.Lock()
			curN -= 1
			mymux.Unlock()
		}()
		/*
			filter := bson.D{{Key: "_id", Value: elem.ID}}

			update := bson.D{
				{Key: "$set", Value: bson.D{
					{Key: "Retracted", Value: retracted},
				}},
			}
			_, err = collectionBL.UpdateOne(context.TODO(), filter, update)
			if err != nil {
				log.Fatal(err)
			}
		*/
	}
	write.Flush()
	file.Close()
	time.Sleep(time.Second * 10)
}

// 2332	26
