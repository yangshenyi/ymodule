package main

import (
	//"bufio"

	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type RetractInfo struct {
	Path        string   `json:"Path" bson:"Path"`
	Version     string   `json:"Version" bson:"Version"`
	RetractText []string `json:"RetractText" bson:"RetractText"`
	PublishTime string
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
	BuildList   []BLEle            `bson:"BuildList"`
	State       int                `bson:"State"`
	PublishTime string             `bson:"PublishTime"`
}

type versionRange struct {
	Vleft  string
	Vright string
}

type filterRI struct {
	Version     string
	Range       []versionRange
	PublishTime string
}

type retractedVersionInfo struct {
	Path               string
	Version            string
	RetractedTime      string
	Affect             int
	AffectAfterRetract int
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

	// KEY		PATH + Version
	retractedVersion := make(map[string]retractedVersionInfo, 0)
	content, err := os.ReadFile("retractedVersion.txt")
	if err != nil {
		log.Fatal(err)
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		words := strings.Fields(line)
		var elemR retractedVersionInfo
		elemR.Affect = 0
		elemR.AffectAfterRetract = 0
		elemR.Path = words[0]
		elemR.Version = words[1]
		elemR.RetractedTime = ""
		//elemR.RetractedTime
		retractedVersion[elemR.Path+elemR.Version] = elemR
	}

	cur, err := collectionBL.Find(context.TODO(), bson.D{{}})
	if err != nil {
		log.Fatal(err)
	}

	num1 := 0
	num2 := 0
	count := 0
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

		k1 := 0
		k2 := 0
		for _, dependengcy := range elem.BuildList {
			if entry, ok1 := retractedVersion[dependengcy.Path+dependengcy.Version]; ok1 {
				entry.Affect += 1
				if dependengcy.IsDir {
					k1 += 1
				}
				k2 += 1

			}
		}
		if k1 > 0 {
			num1 += 1
		}
		if k2 > 0 {
			num2 += 1
		}

	}

	fmt.Println(num1, num2)

}

// 2332
//67818 304169
