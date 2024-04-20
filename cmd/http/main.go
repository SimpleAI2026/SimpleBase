package main

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
	"github.com/linkxzhou/LessDB/cmd/http/handler"
	"github.com/linkxzhou/LessDB/internal/utils"
	"github.com/prometheus/client_golang/prometheus/promhttp" 

	"fmt"
)

const (
	prefixVersion = "/api/v1"
	defaultListen = ":9000"
)

func withVersion(uri string) string {
	return fmt.Sprintf("%s%s", prefixVersion, uri)
}

// http://localhost:9000/api/v1/uploaddb
// http://localhost:9000/api/v1/createdb
// http://localhost:9000/api/v1/tigger/s3events
// http://localhost:9000/api/v1/:ReadKey/tables
// http://localhost:9000/api/v1/:ReadKey/tables/:tableName/rows
// http://localhost:9000/api/v1/:ReadKey/execute
// http://localhost:9000/api/v1/:ReadKey/query
// http://localhost:9000/api/v1/:ReadKey/executelog
func main() {
	e := echo.New()
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept},
	}))
	e.Logger.SetLevel(log.DEBUG)
	e.POST(withVersion("/uploaddb"), handler.UploadDB)
	e.POST(withVersion("/createdb"), handler.CreateDB)
	e.POST(withVersion("/tigger/s3events"), handler.TiggerS3Events)
	e.GET(withVersion("/:ReadKey/tables"), handler.GetTables)
	e.GET(withVersion("/:ReadKey/tables/:tableName/rows"), handler.GetRows)
	e.POST(withVersion("/:ReadKey/execute"), handler.ExecuteDB)
	e.POST(withVersion("/:ReadKey/executelog"), handler.ExecuteLog)
	e.POST(withVersion("/:ReadKey/query"), handler.QueryDB)
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))  

	listenSvr := utils.GetEnviron("LESSDB_LISTEN")
	if listenSvr == utils.EmptyNil {
		listenSvr = defaultListen
	}
	e.Logger.Fatal(e.Start(listenSvr))
}
