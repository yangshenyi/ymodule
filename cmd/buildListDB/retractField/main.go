package main

import (
	//"bufio"

	"bufio"
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
	"golang.org/x/mod/semver"
)

type RetractInfo struct {
	Path        string   `json:"Path" bson:"Path"`
	Version     string   `json:"Version" bson:"Version"`
	RetractText []string `json:"RetractText" bson:"RetractText"`
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
	Retracted   bool               `bson:"Retracted"`
	PublishTime string             `bson:"PublishTime"`
}

type versionRange struct {
	Vleft  string
	Vright string
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
	collectionR := db.Collection("retarctInfo")

	cur, err := collectionR.Find(context.TODO(), bson.D{{}})
	if err != nil {
		log.Fatal(err)
	}
	retractV := make(map[string]map[versionRange]bool, 0)
	for cur.Next(context.TODO()) {
		var elem RetractInfo
		err := cur.Decode(&elem)
		if err != nil {
			log.Fatal(err)
		}
		if _, ok := retractV[elem.Path]; !ok {
			retractV[elem.Path] = make(map[versionRange]bool, 0)
		}
		for _, line := range elem.RetractText {
			for ix, v := range line {
				if v == '/' {
					break
				} else if v == 'v' {
					RV := strings.Fields(line[ix:])
					retractV[elem.Path][versionRange{RV[0], RV[0]}] = true
					break
				} else if v == '[' {
					for ix2, v := range line {
						if v == ']' {
							RV := strings.Split(line[ix+1:ix2], ", ")
							if len(RV) != 2 {
								fmt.Println(elem, RV)
								break
							}
							retractV[elem.Path][versionRange{RV[0], RV[len(RV)-1]}] = true
							break
						}
					}
					break
				}
			}
		}
	}

	cur, err = collectionBL.Find(context.TODO(), bson.D{{}})
	if err != nil {
		log.Fatal(err)
	}

	file, err := os.OpenFile("retractversion.csv", os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Println("文件打开失败", err)
	}
	write := bufio.NewWriter(file)

	count := 0
	num := 0
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
		if elem.State == 1 {
			if elem.Retracted {
				write.WriteString(fmt.Sprintln(elem.Path, elem.Version, elem.PublishTime))
			}
			continue
		} else {
			continue
		}

		retracted := false
		if _, ok := retractV[elem.Path]; ok {
			for k := range retractV[elem.Path] {
				retracted = (k.Vleft == k.Vright && k.Vleft == elem.Version) || (semver.Compare(k.Vleft, elem.Version) <= 0 && semver.Compare(elem.Version, k.Vright) <= 0)
				if retracted {
					num += 1
					fmt.Println(elem.Path, elem.Version, num)
					break
				}
			}
		}

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

	}
	write.Flush()
	file.Close()
}

// 2332	26
