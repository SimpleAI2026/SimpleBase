package main

import (
	sqle "github.com/dolthub/go-mysql-server"
	"github.com/dolthub/go-mysql-server/server"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/information_schema"
	"github.com/linkxzhou/LessDB/internal/utils"
)

func main() {
	engine := sqle.NewDefault(
		sql.NewDatabaseProvider(
			information_schema.NewInformationSchemaDatabase(),
		))

	listenSvr := utils.GetEnviron("LESSDB_LISTEN")
	if listenSvr == utils.EmptyNil {
		listenSvr = "localhost:3306"
	}

	config := server.Config{
		Protocol: "tcp",
		Address:  listenSvr,
	}

	svr, err := server.NewServer(config, engine, nil, nil)
	if err != nil {
		panic(err)
	}

	svr.Start()
}
