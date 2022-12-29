package main

import (
	//"bufio"

	"context"
	"fmt"
	"io/ioutil"
	"log"
	"sort"
	"strings"
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

//var mymux, mymux2 sync.Mutex

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
	collectionR := db.Collection("DeprecationModuleInfo")

	//max := 10
	//curN := 0

	cur, err := collectionBL.Find(context.TODO(), bson.D{{}})
	if err != nil {
		log.Fatal(err)
	}

	deprecationML := make(map[string]depracationModuleInfo, 0)
	//file, err := os.OpenFile("deprecation.txt", os.O_RDONLY, 0666)
	f, err := ioutil.ReadFile("deprecation.txt")
	if err != nil {
		fmt.Println("文件打开失败", err)
	}
	for _, line := range strings.Split(string(f), "\n") {
		if line == "" {
			continue
		}
		words := strings.Fields(line)
		if v, ok := deprecationML[words[0]]; ok {
			if strings.Compare(words[4], v.DeprecationTime) < 0 {
				v.DeprecationTime = words[4]
				v.DeprecationVersion = words[2]
				deprecationML[words[0]] = v
			}
		} else {
			var temp depracationModuleInfo
			temp.Deprecated = true
			temp.Path = words[0]
			temp.DeprecationVersion = words[2]
			temp.DeprecationTime = words[4]
			temp.Versions = make([]VersionInfo, 0)
			deprecationML[words[0]] = temp
		}
	}

	f, err = ioutil.ReadFile("retract!!!.txt")
	if err != nil {
		fmt.Println("文件打开失败", err)
	}
	for _, line := range strings.Split(string(f), "\n") {
		if line == "" {
			continue
		}
		words := strings.Fields(line)
		if _, ok := deprecationML[words[0]]; !ok {
			var temp depracationModuleInfo
			temp.Deprecated = false
			temp.Path = words[0]
			temp.DeprecationVersion = ""
			temp.DeprecationTime = ""
			temp.Versions = make([]VersionInfo, 0)
			deprecationML[words[0]] = temp
		}
	}

	count := 0
	Nummodule := make(map[string]bool, 0)

	for cur.Next(context.TODO()) {
		count += 1
		if count%10000 == 0 {
			fmt.Println(count)
		}

		var elem BLDBEle
		err := cur.Decode(&elem)
		if err != nil {
			log.Fatal(err)
		}
		if elem.State != 1 {
			continue
		}
		if v, ok := deprecationML[elem.Path]; ok {
			v.Versions = append(v.Versions, VersionInfo{elem.Version, elem.Retracted})
			deprecationML[elem.Path] = v
		}
		if _, ok := Nummodule[elem.Path]; !ok {
			Nummodule[elem.Path] = true
		}

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

	for k, v := range deprecationML {
		sort.Sort(entryWrapper{v.Versions, func(e1, e2 *VersionInfo) bool {
			return semver.Compare(e1.Version, e2.Version) < 0
		}})
		deprecationML[k] = v
		collectionR.InsertOne(context.TODO(), v)
	}

	fmt.Println(len(Nummodule))

	removeSuffix := func(input string) string {
		ret := input
		sp := strings.Split(input, "/")
		if sp[0] == "gopkg.in" {
			if sp2 := strings.Split(input, "."); len(sp2) == 3 {
				ret = strings.Join(sp2[:2], ".")
			}
		} else if sp[len(sp)-1][0] == 'v' {
			flag := true
			for _, v := range sp[len(sp)-1][1:] {
				if v < '0' || v > '9' {
					flag = false
				}
			}
			if flag {
				ret = strings.Join(sp[:len(sp)-1], "/")
			}
		}
		return ret
	}

	NumComp := make(map[string]bool, 0)
	for k, _ := range Nummodule {
		key := removeSuffix(k)
		if _, ok := NumComp[key]; !ok {
			NumComp[key] = true
		}
	}
	fmt.Println(len(NumComp))

	NumComp = make(map[string]bool, 0)
	for k, _ := range deprecationML {
		key := removeSuffix(k)
		if _, ok := NumComp[key]; !ok {
			NumComp[key] = true
		}
	}
	fmt.Println(len(NumComp))

	time.Sleep(time.Second * 10)
}

// 257537
// 250011
// 635
