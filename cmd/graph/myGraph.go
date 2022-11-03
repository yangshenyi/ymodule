package main

import (
	//"bufio"
	"fmt"
	"os"

	"github.com/yangshenyi/ymodule/loadmod"
	"golang.org/x/mod/module"
)

func runGraph(target module.Version) {
	mg, ok, info := loadmod.LoadModGraph(target)
	if !ok {
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
	
		for _, v := range mg.BuildList() {
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
