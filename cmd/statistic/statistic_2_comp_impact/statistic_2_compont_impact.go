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

type entry struct {
	Comp         int
	NumDependent int
	//NumDependentPV float64
}

type entryWrapper struct {
	array []entry
	by    func(p, q *entry) bool
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

// map[comp] map[ID] bool	==>	len
// map[comp] ID

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

	spreadImpact := map[int]map[int]bool{}
	directImpact := map[int]map[int]bool{}
	CompId := map[string]int{}
	ReverseId := map[int]string{}

	/*
		file, err := os.OpenFile("fail_resolve.txt", os.O_WRONLY|os.O_CREATE, 0666)
		if err != nil {
			fmt.Println("文件打开失败", err)
		}
		write := bufio.NewWriter(file)
	*/
	count := 0
	id := 0
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

		mainComp := removeSuffix(elem.Path)
		if _, ok := CompId[mainComp]; !ok {
			CompId[mainComp] = id
			ReverseId[id] = mainComp
			id += 1
		}

		for _, item := range elem.BuildList {
			// get common path
			if item.Version == "" {
				continue
			}
			itemComp := removeSuffix(item.Path)
			if _, ok := CompId[itemComp]; !ok {
				CompId[itemComp] = id
				ReverseId[id] = itemComp
				id += 1
			}

			if _, ok := spreadImpact[CompId[itemComp]]; ok {
				spreadImpact[CompId[itemComp]][CompId[mainComp]] = true
			} else {
				spreadImpact[CompId[itemComp]] = make(map[int]bool, 0)
				spreadImpact[CompId[itemComp]][CompId[mainComp]] = true
			}

			if !item.IsDir {
				continue
			}

			if _, ok := directImpact[CompId[itemComp]]; ok {
				directImpact[CompId[itemComp]][CompId[mainComp]] = true
			} else {
				directImpact[CompId[itemComp]] = make(map[int]bool, 0)
				directImpact[CompId[itemComp]][CompId[mainComp]] = true
			}

		}
	}
	/*
		write.Flush()
		file.Close()
	*/

	sliceR := make([]entry, 0)
	for k, v := range directImpact {
		sliceR = append(sliceR, entry{k, len(v)})
	}
	sort.Sort(entryWrapper{sliceR, func(e1, e2 *entry) bool { return e2.NumDependent < e1.NumDependent }})

	file, err := os.OpenFile("list_dir_nodes.csv", os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Println("文件打开失败", err)
	}
	write := bufio.NewWriter(file)
	for _, item := range sliceR[:1000] {
		write.WriteString(fmt.Sprintln(item.Comp, ",", ReverseId[item.Comp], ",", item.NumDependent))
	}
	write.Flush()
	file.Close()

	file, err = os.OpenFile("list_dir_edges.csv", os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Println("文件打开失败", err)
	}
	write = bufio.NewWriter(file)
	top30id := make(map[int]bool, 0)
	for _, item := range sliceR[:30] {
		top30id[item.Comp] = true
	}
	for _, item := range sliceR[:30] {
		for k := range directImpact[item.Comp] {
			if _, ok := top30id[k]; ok {
				write.WriteString(fmt.Sprintln(item.Comp, ",", k))
			}
		}
	}
	write.Flush()
	file.Close()

	sliceR = make([]entry, 0)
	for k, v := range spreadImpact {
		sliceR = append(sliceR, entry{k, len(v)})
	}
	sort.Sort(entryWrapper{sliceR, func(e1, e2 *entry) bool { return e2.NumDependent < e1.NumDependent }})

	file, err = os.OpenFile("list_all_nodes.csv", os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Println("文件打开失败", err)
	}
	write = bufio.NewWriter(file)
	for _, item := range sliceR[:1000] {
		write.WriteString(fmt.Sprintln(item.Comp, ",", ReverseId[item.Comp], ",", item.NumDependent))
	}
	write.Flush()
	file.Close()

	file, err = os.OpenFile("list_all_edges.csv", os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Println("文件打开失败", err)
	}
	write = bufio.NewWriter(file)
	top30id = make(map[int]bool, 0)
	for _, item := range sliceR[:30] {
		top30id[item.Comp] = true
	}
	for _, item := range sliceR[:30] {
		for k := range spreadImpact[item.Comp] {
			if _, ok := top30id[k]; ok {
				write.WriteString(fmt.Sprintln(item.Comp, ",", k))
			}
		}
	}
	write.Flush()
	file.Close()
}
