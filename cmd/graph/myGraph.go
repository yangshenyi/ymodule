package main

import (
	//"bufio"
	"fmt"
	"os"

	"context"
	"crypto/tls"
	"net/http"
	"net/url"
	"time"

	"github.com/yangshenyi/ymodule/loadmod"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/mod/module"
)

func runGraph(target module.Version) {
	// connect mongodb
	var (
		client     *mongo.Client
		err        error
		db         *mongo.Database
		collection *mongo.Collection
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
	collection = db.Collection("modData")

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

	mg, ok, info := loadmod.LoadModGraph(target, collection, httpClient)
	if ok != 1 {
		fmt.Println("load Graph fail[!]", info)

	}
	/*
		w := bufio.NewWriter(os.Stdout)
		defer w.Flush()

		format := func(m module.Version) {
			w.WriteString(m.Path)
			if m.Version != "" {
				w.WriteString("@")
				w.WriteString(m.Version)
			}
		}

		mg.WalkBreadthFirst(func(m module.Version) {
			reqs, _ := mg.RequiredBy(m)
			for _, r := range reqs {
				format(m)
				w.WriteByte(' ')
				format(r)
				w.WriteByte('\n')
			}
		})*/
	//fmt.Println(*info, "\n\n")
	//fmt.Println(mg. Selected("null"))

	fmt.Println(target.Path)
	for _, v := range mg.BuildList()[1:] {
		if k, ok := (*info)[v]; ok {
			fmt.Println(v.Path, v.Version, "=>", k.Path, k.Version)
		} else if k, ok := (*info)[module.Version{Path: v.Path, Version: ""}]; ok {
			fmt.Println(v.Path, v.Version, "=>", k.Path, k.Version)
		} else {
			fmt.Println(v.Path, v.Version)
		}

	}

}

func main() {
	// runGraph(module.Version{Path: "gorm.io/driver/mysql", Version: "v1.3.5"})
	// runGraph(module.Version{Path: "google.golang.org/protobuf", Version: "v1.26.0"})

	// runGraph(module.Version{Path: "go.mongodb.org/mongo-driver", Version: "v1.10.1"})

	if len(os.Args) != 3 {
		fmt.Println("illegal num of cmd parameters!")
		return
	}

	runGraph(module.Version{Path: os.Args[1], Version: os.Args[2]})
}
