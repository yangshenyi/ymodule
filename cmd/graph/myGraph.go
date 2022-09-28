package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/yangshenyi/ymodule/loadmod"
	"golang.org/x/mod/module"
)

func runGraph(target module.Version) {
	mg, ok := loadmod.LoadModGraph(target)
	if !ok {
		fmt.Println("load Graph fail[!]")

	}

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
	})

	/*
		for _, v := range mg.BuildList() {
			fmt.Println(v)
		}*/
}

func main() {
	// runGraph(module.Version{Path: "gorm.io/driver/mysql", Version: "v1.3.5"})
	// runGraph(module.Version{Path: "google.golang.org/protobuf", Version: "v1.26.0"})

	// runGraph(module.Version{Path: "go.mongodb.org/mongo-driver", Version: "v1.10.1"})

	runGraph(module.Version{Path: "golang.org/x/crypto", Version: "v0.0.0-20220722155217-630584e8d5aa"})
}
