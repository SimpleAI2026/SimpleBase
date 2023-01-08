package server

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/linkxzhou/gongx/goscript"
	"github.com/linkxzhou/gongx/interp/go/loader"
	"github.com/linkxzhou/gongx/packages/log"
	"github.com/linkxzhou/gongx/packages/server"
)

var initFileMap = make(map[string]*goscript.Interp, 0)
var interpc = loader.NewContext(loader.EnableDumpImports)

func initFileList() {
	rootPath := "gofiles/"
	files, err := ioutil.ReadDir(rootPath)
	if err != nil {
		log.Debug("==== err: ", err)
	}
	for _, file := range files {
		fileName := rootPath + file.Name()
		if s, err := ioutil.ReadFile(rootPath + file.Name()); err == nil {
			initFileMap["/"+fileName], err = goscript.LoadFileWithCache(interpc, fileName, string(s), "")
			fmt.Println("fileName: ", fileName, ", err: ", err)
		}
	}
}

func TestServerGofiles(t *testing.T) {
	initFileList()
	app := server.New()
	for path, _ := range initFileMap {
		app.Any(path, func(c server.Context) error {
			pathName := c.Path()
			fmt.Println("pathName: ", pathName)
			if iv, ok := initFileMap[pathName]; ok && iv != nil {
				result, err2 := iv.RunAny("Handle2")
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
