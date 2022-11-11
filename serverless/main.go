package main

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/linkxzhou/gongx/interp_go"
	"github.com/linkxzhou/gongx/interp_go/loader"
	"github.com/linkxzhou/gongx/packages/log"
	"github.com/linkxzhou/gongx/packages/server"
)

const handleName = "Handle"

var initApiMap = make(map[string]*interp_go.Interp, 0)
var interpc = loader.NewContext(loader.EnableDumpImports)

func init() {
	rootPath := "api/"
	files, err := ioutil.ReadDir(rootPath)
	if err != nil {
		log.Fatal("==== err: ", err)
	}
	for _, file := range files {
		fileName := rootPath + file.Name()
		if s, err := ioutil.ReadFile(rootPath + file.Name()); err == nil {
			initApiMap["/"+fileName], err = interp_go.LoadWithCache(interpc, fileName, string(s))
			log.Error("fileName: ", fileName, ", err: ", err)
		}
	}
}

func main() {
	app := server.New()
	for path, _ := range initApiMap {
		app.Any(path, func(c server.Context) error {
			pathName := c.Path()
			if iv, ok := initApiMap[pathName]; ok && iv != nil {
				result, err2 := iv.RunAny(handleName)
				if err2 != nil {
					return c.String(http.StatusInternalServerError, "err2: "+err2.Error())
				} else {
					return c.String(http.StatusOK, fmt.Sprintf("%v", result))
				}
			}
			return c.String(http.StatusInternalServerError, "error")
		})
	}

	app.Start(":3001")
}
