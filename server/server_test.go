package server

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/linkxzhou/gongx/interp"
	"github.com/linkxzhou/gongx/interp/loader"
	"github.com/linkxzhou/gongx/log"
	"github.com/linkxzhou/gongx/server"
)

func TestServerHttp(t *testing.T) {
	sources := `
	package test

	func fib(i int) int {
		if i < 2 {
			return i
		}
		return fib(i - 1) + fib(i - 2)
	}	
	`
	c := loader.NewContext(loader.EnableDumpImports)
	p, err := c.LoadFile("__main__.go", sources)
	fmt.Println("p: ", p, ", err: ", err)
	iv1, err1 := interp.NewInterp(c, p)
	fmt.Println("NewInterp err: ", err1, ", iv1: ", iv1)

	app := server.New()

	app.GET("/", func(c server.Context) error {
		log.Info("========= RealIP: ", c.RealIP())
		iv2, err2 := iv1.RunAny("fib", 37)
		log.Info("========= : ", iv2, err2)
		return c.String(http.StatusOK, "Hello, World!")
	})

	app.Start(":3000")
}
