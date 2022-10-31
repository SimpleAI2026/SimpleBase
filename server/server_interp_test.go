package server

import (
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/linkxzhou/gongx/gointerp"
	"github.com/linkxzhou/gongx/gointerp/loader"
	"github.com/linkxzhou/gongx/log"
	"github.com/linkxzhou/gongx/server"
)

func TestServerInterp(t *testing.T) {
	interpc := loader.NewContext(loader.EnableDumpImports)
	app := server.New()

	app.Any("/", func(c server.Context) error {
		defer c.Request().Body.Close()
		buf, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return c.String(http.StatusOK, "buf err: "+err.Error())
		}
		sources := string(buf)
		fmt.Println("== sources: ", sources)
		iv1, err1 := gointerp.LoadWithCache(interpc, "__main__.go", sources)
		if err1 != nil {
			return c.String(http.StatusOK, "err: "+err1.Error())
		}
		fmt.Println("NewInterp err: ", err1, ", iv1: ", iv1)
		iv2, err2 := iv1.RunMain()
		log.Info("========= : ", iv2, err2)
		if err2 != nil {
			return c.String(http.StatusOK, "err2: "+err2.Error())
		}
		return c.String(http.StatusOK, fmt.Sprintf("result: %v", iv2))
	})

	app.Any("/handle", func(c server.Context) error {
		defer c.Request().Body.Close()
		buf, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return c.String(http.StatusOK, "buf err: "+err.Error())
		}
		sources := string(buf)
		fmt.Println("== sources: ", sources)
		iv1, err1 := gointerp.LoadWithCache(interpc, "__main__.go", sources)
		if err1 != nil {
			return c.String(http.StatusOK, "err1: "+err1.Error())
		}
		fmt.Println("NewInterp err: ", err1, ", iv1: ", iv1)
		iv2, err2 := iv1.RunAny("Handle")
		log.Info("========= : ", iv2, err2)
		if err2 != nil {
			return c.String(http.StatusOK, "err2: "+err2.Error())
		}
		return c.String(http.StatusOK, fmt.Sprintf("result: %v", iv2))
	})

	app.Start(":3001")
}
