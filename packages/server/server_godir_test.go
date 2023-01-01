package server

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/linkxzhou/gongx/interp/go/loader"
	"github.com/linkxzhou/gongx/interp_go"
	"github.com/linkxzhou/gongx/packages/server"
)

var initDirInterp *interp_go.Interp
var interpcDir = loader.NewContext(loader.EnableDumpImports)
var rootPath = "tests/"

func initDir() {
	initDirInterp, _ = interp_go.LoadDirWithCache(interpcDir, rootPath)
}

func TestServerGodir(t *testing.T) {
	initDir()
	app := server.New()
	app.Any(rootPath, func(c server.Context) error {
		fn := c.QueryParam("fn")
		fmt.Println("fn: ", fn)
		result, err2 := initDirInterp.RunAny(fn)
		if err2 != nil {
			return c.String(http.StatusInternalServerError, "err2: "+err2.Error())
		} else {
			return c.String(http.StatusOK, fmt.Sprintf("%v", result))
		}
	})

	app.Start(":3001")
}
