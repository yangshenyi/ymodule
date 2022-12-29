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
	collectionR := db.Collection("retarctInfo")

	cur, err := collectionR.Find(context.TODO(), bson.D{{}})
	if err != nil {
		log.Fatal(err)
	}

	retractV := make(map[string][]RetractInfo, 0)
	num := 0
	for cur.Next(context.TODO()) {
		num += 1
		fmt.Println(num)
		var elem RetractInfo
		err := cur.Decode(&elem)
		if err != nil {
			log.Fatal(err)
		}
		var varBL BLDBEle
		if err = collectionBL.FindOne(context.TODO(), bson.M{"Path": elem.Path, "Version": elem.Version}).Decode(&varBL); err != nil {
			fmt.Println("!!!!!!!!!!!!!!!!!![1]")
		}
		elem.PublishTime = varBL.PublishTime
		if _, ok := retractV[elem.Path]; !ok {
			retractV[elem.Path] = make([]RetractInfo, 0)
		}

		if len(retractV[elem.Path]) == 0 {
			retractV[elem.Path] = append(retractV[elem.Path], elem)
		} else {
			if len(retractV[elem.Path]) > 50 {
				fmt.Println(elem.Path, len(retractV[elem.Path]))
			}
			flag := true
			for ix, obj := range retractV[elem.Path] {
				if len(obj.RetractText) == len(elem.RetractText) {
					for i := 0; i < len(obj.RetractText); i++ {
						if obj.RetractText[i] != elem.RetractText[i] {
							flag = false
							break
						}
					}
				} else {
					flag = false
				}

				if flag && strings.Compare(elem.PublishTime, obj.PublishTime) < 0 {
					retractV[elem.Path][ix].Version = elem.Version
					retractV[elem.Path][ix].PublishTime = elem.PublishTime
				}
			}
			if !flag {
				retractV[elem.Path] = append(retractV[elem.Path], elem)
			}
		}

	}
	//fmt.Println(retractV)
	retractV2 := make(map[string][]filterRI, 0)

	num = 0
	for k, v := range retractV {
		num += 1
		fmt.Println(num)
		if _, ok := retractV2[k]; !ok {
			retractV2[k] = make([]filterRI, 0)
		}

		for _, v2 := range v {
			varElem := filterRI{}
			varElem.Version = v2.Version
			varElem.PublishTime = v2.PublishTime
			varElem.Range = make([]versionRange, 0)

			for _, line := range v2.RetractText {
				for ix, v := range line {
					if v == '/' {
						break
					} else if v == 'v' {
						RV := strings.Fields(line[ix:])
						varElem.Range = append(varElem.Range, versionRange{RV[0], RV[0]})
						break
					} else if v == '[' {
						for ix2, v := range line {
							if v == ']' {
								RV := strings.Split(line[ix+1:ix2], ", ")
								if len(RV) != 2 {
									break
								}
								varElem.Range = append(varElem.Range, versionRange{RV[0], RV[len(RV)-1]})
								break
							}
						}
						break
					}
				}
			}
			retractV2[k] = append(retractV2[k], varElem)
		}

	}
	//fmt.Println(retractV2)

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
		for _, v := range retractV2[words[0]] {

			for _, k := range v.Range {
				if (k.Vleft == k.Vright && k.Vleft == elemR.Version) || (semver.Compare(k.Vleft, elemR.Version) <= 0 && semver.Compare(elemR.Version, k.Vright) <= 0) {
					if elemR.RetractedTime == "" || strings.Compare(elemR.RetractedTime, v.PublishTime) > 0 {
						elemR.RetractedTime = v.PublishTime
						fmt.Println(elemR)
					}
				}
			}
		}

		retractedVersion[elemR.Path+elemR.Version] = elemR
	}

	cur, err = collectionBL.Find(context.TODO(), bson.D{{}})
	if err != nil {
		log.Fatal(err)
	}

	file, err := os.OpenFile("retract!!!.txt", os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Println("文件打开失败", err)
	}
	write := bufio.NewWriter(file)
	for _, v := range retractedVersion {
		write.WriteString(fmt.Sprintln(v.Path, v.Version, v.RetractedTime))
	}
	write.Flush()
	file.Close()

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

		for _, dependengcy := range elem.BuildList {
			if entry, ok1 := retractedVersion[dependengcy.Path+dependengcy.Version]; ok1 {
				entry.Affect += 1
				if strings.Compare(entry.RetractedTime, elem.PublishTime) < 0 {
					entry.AffectAfterRetract += 1
				}
				retractedVersion[dependengcy.Path+dependengcy.Version] = entry
			}
		}

	}

	file, err = os.OpenFile("retract!!!again.txt", os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Println("文件打开失败", err)
	}
	write = bufio.NewWriter(file)
	for _, v := range retractedVersion {
		write.WriteString(fmt.Sprintln(v.Path, ",", v.Version, ",", v.RetractedTime, ",", v.Affect, ",", v.AffectAfterRetract))
	}
	write.Flush()
	file.Close()

}

// 2332
