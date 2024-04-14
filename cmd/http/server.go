package main

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
	"github.com/linkxzhou/TamiDB/cmd/http/handler"

	"fmt"
)

const (
	prefixVersion = "/api/v1"
	defaultListen = ":18090"
)

func withVersion(uri string) string {
	return fmt.Sprintf("%s%s", prefixVersion, uri)
}

// http://localhost:18090/api/v1/uploaddb
// http://localhost:18090/api/v1/createdb
// http://localhost:18090/api/v1/tigger/s3events
// http://localhost:18090/api/v1/:DBName/tables
// http://localhost:18090/api/v1/:DBName/tables/:tableName/rows
// http://localhost:18090/api/v1/:DBName/execute
// http://localhost:18090/api/v1/:DBName/query
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
	e.GET(withVersion("/:DBName/tables"), handler.GetTables)
	e.GET(withVersion("/:DBName/tables/:tableName/rows"), handler.GetRows)
	e.POST(withVersion("/:DBName/execute"), handler.ExecuteDB)
	e.POST(withVersion("/:DBName/query"), handler.QueryDB)
	e.Logger.Fatal(e.Start(defaultListen))
}
