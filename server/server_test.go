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

// func TestServerHttp(t *testing.T) {
// 	sources := `
// 	package test

// 	func fib(i int) int {
// 		if i < 2 {
// 			return i
// 		}
// 		return fib(i - 1) + fib(i - 2)
// 	}
// 	`
// 	c := loader.NewContext(loader.EnableDumpImports)
// 	p, err := c.LoadFile("__main__.go", sources)
// 	fmt.Println("p: ", p, ", err: ", err)
// 	iv1, err1 := gointerp.NewInterp(c, p)
// 	fmt.Println("NewInterp err: ", err1, ", iv1: ", iv1)

// 	app := server.New()

// 	app.GET("/", func(c server.Context) error {
// 		log.Info("========= RealIP: ", c.RealIP())
// 		iv2, err2 := iv1.RunAny("fib", 37)
// 		log.Info("========= : ", iv2, err2)
// 		return c.String(http.StatusOK, "Hello, World!")
// 	})

// 	app.Start(":3000")
// }

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
		p, err := interpc.LoadFile("__main__.go", sources)
		if err != nil {
			return c.String(http.StatusOK, "err: "+err.Error())
		}
		fmt.Println("p: ", p, ", err: ", err)
		iv1, err1 := gointerp.NewInterp(interpc, p)
		if err1 != nil {
			return c.String(http.StatusOK, "err1: "+err1.Error())
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
		p, err := interpc.LoadFile("__main__.go", sources)
		if err != nil {
			return c.String(http.StatusOK, "err: "+err.Error())
		}
		fmt.Println("p: ", p, ", err: ", err)
		iv1, err1 := gointerp.NewInterp(interpc, p)
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
