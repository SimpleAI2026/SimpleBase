package server

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/linkxzhou/gongx/interp_go"
	"github.com/linkxzhou/gongx/interp_go/loader"
	"github.com/linkxzhou/gongx/packages/log"
	"github.com/linkxzhou/gongx/packages/server"
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
	interpc := loader.NewContext(loader.DisableRecover)
	p, err := interpc.LoadFile("__main__.go", sources)
	fmt.Println("p: ", p, ", err: ", err)

	app := server.New()

	app.GET("/", func(c server.Context) error {
		log.Info("========= RealIP: ", c.RealIP())
		iv1, err1 := interp_go.LoadFileWithCache(interpc, "__main__.go", sources)
		fmt.Println("NewInterp err: ", err1, ", iv1: ", iv1)
		if err1 != nil {
			return c.String(http.StatusBadRequest, "Hello, World![1]")
		}
		iv2, err2 := iv1.RunAny("fib", 37)
		log.Info("========= : ", iv2, err2)
		return c.String(http.StatusOK, "Hello, World!")
	})

	app.Start(":3000")
}
