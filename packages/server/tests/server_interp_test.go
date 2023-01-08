package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/linkxzhou/gongx/goscript"
	"github.com/linkxzhou/gongx/interp/go/loader"
	"github.com/linkxzhou/gongx/packages/log"
	"github.com/linkxzhou/gongx/packages/server"
)

const defaultSources = `
package test

import (
    "encoding/json"
	"net/http"
)

func Handle() interface{} {
    coronaVirusJSON := ` + "`" + `{
        "name" : "covid-11",
        "country" : "China",
        "city" : "Wuhan",
        "reason" : "Non vedge Food"
    }` + "`" + `
    var result map[string]interface{}
    json.Unmarshal([]byte(coronaVirusJSON), &result)
    return result
}
`

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
		iv1, err1 := goscript.LoadFileWithCache(interpc, "__main__.go", sources, "")
		if err1 != nil {
			return c.String(http.StatusOK, "err: "+err1.Error())
		}
		iv2, err2 := iv1.RunMain()
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
		iv1, err1 := goscript.LoadWithCache(interpc, "__main__.go", sources)
		if err1 != nil {
			return c.String(http.StatusOK, "err1: "+err1.Error())
		}
		iv2, err2 := iv1.RunAny("Handle")
		log.Info("========= : ", iv2, err2)
		if err2 != nil {
			return c.String(http.StatusOK, "err2: "+err2.Error())
		}
		return c.String(http.StatusOK, fmt.Sprintf("result: %v", iv2))
	})

	app.Any("/default", func(c server.Context) error {
		iv1, err1 := goscript.LoadWithCache(interpc, "__main__.go", defaultSources)
		if err1 != nil {
			return c.String(http.StatusOK, "err1: "+err1.Error())
		}
		iv2, err2 := iv1.RunAny("Handle")
		log.Info("========= : ", iv2, err2)
		if err2 != nil {
			return c.String(http.StatusOK, "err2: "+err2.Error())
		}
		return c.String(http.StatusOK, fmt.Sprintf("result: %v", iv2))
	})

	app.Any("/origincall", func(c server.Context) error {
		coronaVirusJSON := `{
			"name" : "covid-11",
			"country" : "China",
			"city" : "Wuhan",
			"reason" : "Non vedge Food"
		}`
		var result map[string]interface{}
		json.Unmarshal([]byte(coronaVirusJSON), &result)
		log.Info("========= : ", result)
		return c.String(http.StatusOK, fmt.Sprintf("result: %v", result))
	})

	app.Start(":3001")
}
