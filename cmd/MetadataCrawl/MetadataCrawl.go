package main

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type module struct {
	Path, Version, Timestamp, publishTime    string
	IsPackageOnDev, IsVersionOnDev, ValidMod bool
}

/*
type modInfo struct {
}
*/
type publishTime struct {
	Version, Time string
}

func trans(src string) string {
	var ret string
	for _, val := range src {
		if val >= 'A' && val <= 'Z' {
			ret += "!" + string(val)
		} else {
			ret += string(val)
		}
	}
	return ret
}

func Parse(mod []module, client *http.Client, quit chan bool, db *sql.DB) {
	stmt, err := db.Prepare("INSERT INTO goversion(path, version, timestamp, publishtime, isGopackage, isVersionOnDev, validgomod) VALUES(?,?,?,?,?,?,?)")
	if err != nil {
		log.Fatal(err, "!!!")
	}
	defer stmt.Close()

	for _, val := range mod {
		//package on pkg.go.dev？
		resp, err := client.Head("https://pkg.go.dev/" + val.Path)
		if err != nil {
			log.Fatal("[Error]", err, 1, val)
		}
		if resp.StatusCode == 200 {
			val.IsPackageOnDev = true
			respMod, errMod := client.Get("https://proxy.golang.org/" + trans(val.Path) + "/@v/" + trans(val.Version) + ".mod")
			if errMod != nil {
				log.Fatal("[Error]", err, 2, val)
			}

			if respMod.StatusCode == 200 {
				bytesmod, _ := ioutil.ReadAll(respMod.Body)
				s := string(bytesmod)
				if len(strings.Split(s, " ")) > 2 {
					val.ValidMod = true
				}
			}
			respMod.Body.Close()

			//Publish Time
			//$base/$module/@v/$version.info
			respPubTime, errPubTime := client.Get("https://proxy.golang.org/" + trans(val.Path) + "/@v/" + trans(val.Version) + ".info")
			if errPubTime != nil {
				log.Fatal("[Error]", err, 3, val)
			}

			if respPubTime.StatusCode == 200 {
				dec := json.NewDecoder(respPubTime.Body)
				var temp publishTime
				dec.Decode(&temp)
				val.publishTime = temp.Time
			}
			respPubTime.Body.Close()

			//version on pkg.go.dev?
			respVersion, errVersion := client.Head("https://pkg.go.dev/" + val.Path + "@" + val.Version)
			if errVersion != nil {
				log.Fatal("[Error]", err, 4, val)
			}
			if respVersion.StatusCode == 200 {
				val.IsVersionOnDev = true
				/*
					//valid mod file?
					doc, err := goquery.NewDocumentFromReader(respVersion.Body)
					if err != nil {
						log.Fatal(err)
					}
					doc.Find("ul.UnitMeta-details").Each(func(i int, s *goquery.Selection) {
						if _, ok := s.Find("li").Eq(0).Find("a").Attr("href"); ok {
							val.ValidMod = true
						}
					})*/
			}
			respVersion.Body.Close()
		}
		resp.Body.Close()

		_, err1 := stmt.Exec(val.Path, val.Version, val.Timestamp, val.publishTime, val.IsPackageOnDev, val.IsVersionOnDev, val.ValidMod)
		if err1 != nil {
			log.Fatal(err, 6, val)
		}

		//fmt.Println(val)
	}
	quit <- true
}

func main() {
	//set core
	runtime.GOMAXPROCS(runtime.NumCPU()) // 12 cores on my PC
	core := runtime.NumCPU()
	chunk := 2000/core + 1

	/*
		create table if not exists `goversion`(
			`id` int unsigned auto_increment,
			`path` varchar(100) not null,
			`version` varchar(100) not null,
			`timestamp` varchar(40) not null,
			`publishtime` varchar(40) not null,
			`isGopackage`    boolean default false,
			`isVersionOnDev` boolean default false,
			`validgomod` boolean default false,
			primary key(id)
			)ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=UTF8MB4 ;
	*/
	//mysql
	db, err := sql.Open("mysql",
		"root:ysy013323@tcp(127.0.0.1:3306)/godep")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	//set proxy
	proxyUrl := "http://127.0.0.1:7890"
	proxy, _ := url.Parse(proxyUrl)
	tr := &http.Transport{
		Proxy:           http.ProxyURL(proxy),
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   time.Second * 120,
	}

	var lastMod module
	lastMod.Timestamp = "2019-07-23T05:25:19.821912Z"
	//a signal indicating that a round is over
	quitC := make(chan bool)

	var mods []module
	for {
		t1 := time.Now()
		mods = nil
		resp, err := client.Get("https://index.golang.org/index?since=" + lastMod.Timestamp)
		if err != nil {
			log.Fatal("[Error:]", lastMod, err)
		}
		dec := json.NewDecoder(resp.Body)
		for dec.More() {
			var mod module
			if err := dec.Decode(&mod); err != nil {
				log.Fatal("[Error:]", lastMod)
			}
			mods = append(mods, mod)
		}

		if len(mods) == 1 {
			resp.Body.Close()
			fmt.Println("[Done]")
			break
		} else if len(mods) == 2000 {
			//fmt.Println(mods)
			for i := 1; i < 2000; i += chunk {
				if i+chunk < 2000 {
					go Parse(mods[i:i+chunk], client, quitC, db)
				} else {
					go Parse(mods[i:2000], client, quitC, db)
				}
			}
			for i := 1; i < 2000; i += chunk {
				<-quitC
			}
		} else { // last page
			go Parse(mods[1:], client, quitC, db)
			<-quitC
		}

		resp.Body.Close()
		lastMod = mods[len(mods)-1]
		fmt.Print("\n\n\n", lastMod, time.Since(t1).Milliseconds()/1000, "\n\n\n")
		time.Sleep(time.Second)
	}
}
