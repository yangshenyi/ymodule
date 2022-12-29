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

	compMap := make(map[string]int, 0)
	compDep := make(map[int]map[int]bool, 0)

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

	cur, _ := collectionBL.Find(context.TODO(), bson.D{{}})
	filePath := "E:\\MongoData\\comp_node.csv"
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Println("文件打开失败", err)
	}
	write := bufio.NewWriter(file)
	count := 0
	id := 0
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

		path_ := removeSuffix(elem.Path)
		if _, ok1 := compMap[path_]; !ok1 {
			id++
			compMap[path_] = id
			write.WriteString(fmt.Sprintln(id, ",", path_))
		}

	}
	write.Flush()
	file.Close()

	cur, _ = collectionBL.Find(context.TODO(), bson.D{{}})
	filePath = "E:\\MongoData\\comp_edge.csv"
	file, err = os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Println("文件打开失败", err)
	}
	write = bufio.NewWriter(file)
	count = 0
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

		id := compMap[removeSuffix(elem.Path)]
		if _, ok1 := compDep[id]; !ok1 {
			compDep[id] = make(map[int]bool, 0)
		}
		for _, item := range elem.BuildList {
			if item.IsDir {
				dep_path := removeSuffix(item.Path)
				if dep_id, ok1 := compMap[dep_path]; ok1 {
					compDep[id][dep_id] = true
				}
			}
		}

	}

	for k, v := range compDep {
		for k2 := range v {
			write.WriteString(fmt.Sprintln(k, ",", k2))
		}
	}
	write.Flush()
	file.Close()

}

// 2332
