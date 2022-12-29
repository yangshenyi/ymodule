package main

import (
	//"bufio"

	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type BLEle struct {
	Path    string `json:"Path" bson:"Path"`
	Version string `json:"Version" bson:"Version"`
	IsDir   bool   `bson:"IsDirect"`
}

type BLDBEle struct {
	ID          primitive.ObjectID `json:"_id" bson:"_id"`
	Path        string             `json:"Path" bson:"Path"`
	Version     string             `json:"Version" bson:"Version"`
	BuildList   []BLEle            `bson:"BuildList"`
	State       int                `bson:"State"`
	PublishTime string             `bson:"PublishTime"`
}

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

	deprecatedMap := make(map[string]bool, 0)
	cur, err := collectionR.Find(context.TODO(), bson.D{{}})
	if err != nil {
		log.Fatal(err)
	}
	for cur.Next(context.TODO()) {
		var temp depracationModuleInfo
		err := cur.Decode(&temp)
		if err != nil {
			log.Fatal(err)
		}
		if temp.Deprecated {
			for _, v := range temp.Versions {
				deprecatedMap[temp.Path+v.Version] = true
			}
		} else {
			for _, v := range temp.Versions {
				if v.Retracted {
					deprecatedMap[temp.Path+v.Version] = true
				}
			}
		}
	}

	num1 := 0
	num2 := 0
	count := 0
	cur, _ = collectionBL.Find(context.TODO(), bson.D{{}})
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

	comp := make(map[string]bool, 0)
	for cur.Next(context.TODO()) {
		count += 1
		if count%100000 == 0 {
			fmt.Println(count)
		}
		var elem BLDBEle
		err := cur.Decode(&elem)
		if err != nil {
			log.Fatal(err)
		}
		if elem.State != 1 {
			//write.WriteString(fmt.Sprintln(elem.Path, elem.Version, elem.State))
			continue
		}
		num11 := 0
		num22 := 0
		for _, dependengcy := range elem.BuildList {

			if _, ok1 := deprecatedMap[dependengcy.Path+dependengcy.Version]; ok1 {
				if dependengcy.IsDir {
					num11++
				}
				num22++
			}
		}
		if num11 > 0 {
			num1++
		}
		if num22 > 0 {
			num2++
		}
		if num11 > 0 || num22 > 0 {
			if _, ok1 := comp[removeSuffix(elem.Path)]; !ok1 {
				comp[removeSuffix(elem.Path)] = true
			}
		}

	}

	fmt.Println(num1, num2, len(comp))

}

// 2332
