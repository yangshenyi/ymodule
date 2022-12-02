package main

import (
	//"bufio"

	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type BLEle struct {
	Path    string `json:"Path" bson:"Path"`
	Version string `json:"Version" bson:"Version"`
	IsDir   bool   `bson:"IsDirect"`
}

type BLDBEle struct {
	Path      string  `json:"Path" bson:"Path"`
	Version   string  `json:"Version" bson:"Version"`
	BuildList []BLEle `bson:"BuildList"`
	State     int     `bson:"State"`
}

type version struct {
	Path    string
	Version string
}
type entryWrapper struct {
	array []version
	by    func(p, q *version) bool
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

	count := 0
	Comp := make(map[string]bool, 0)
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
			//write.WriteString(fmt.Sprintln(elem.Path, elem.Version, elem.State))
			continue
		}

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
		Comp[removeSuffix(elem.Path)] = true

	}

	fmt.Println(len(Comp))

	file, err := os.OpenFile("list.csv", os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Println("文件打开失败", err)
	}
	write := bufio.NewWriter(file)
	versions := make([]version, 0)
	count = 0
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
			//write.WriteString(fmt.Sprintln(elem.Path, elem.Version, elem.State))
			continue
		}

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

		for _, item := range elem.BuildList {
			if item.Version == "" {
				continue
			}

			if _, ok := Comp[removeSuffix(item.Path)]; !ok {
				Comp[removeSuffix(item.Path)] = true
				versions = append(versions, version{item.Path, item.Version})
			}
		}
	}

	sort.Sort(entryWrapper{versions, func(e1, e2 *version) bool {
		if e1.Path == e2.Path {
			return e1.Version < e2.Version
		} else {
			return e2.Path < e1.Path
		}
	}})
	for _, v := range versions {
		write.WriteString(fmt.Sprintln(v.Path, v.Version))
	}

	fmt.Println(len(Comp))
	write.Flush()
	file.Close()
}
