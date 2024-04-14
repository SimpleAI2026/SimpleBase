package handler

import (
	"os"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/TamiDB/internal/sqlite3vfs"
	"github.com/linkxzhou/TamiDB/internal/utils"
	"github.com/linkxzhou/TamiDB/internal/vfsextend"

	"database/sql"
	"sync"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

var cacheVFS sync.Map

func getVFSDB(dbName string) (*sql.DB, string, error) {
	uri, err := s3Client.GetFileLink(dbName)
	if err != nil {
		return nil, uri, err
	}

	var vfs *vfsextend.HttpVFS
	if newVFS, ok := cacheVFS.Load(dbName); !ok || newVFS == nil {
		vfs = &vfsextend.HttpVFS{URL: uri}
		if cacheFile, err := os.Create("vfscache_" + dbName); err == nil {
			vfs.CacheHandler = vfsextend.NewDiskCache(cacheFile, -1)
		}
		err = sqlite3vfs.RegisterVFS("httpvfs", vfs)
		if err != nil {
			return nil, uri, err
		}
		cacheVFS.Store(dbName, vfs)
	} else {
		vfs = newVFS.(*vfsextend.HttpVFS)
		err = sqlite3vfs.RegisterVFS("httpvfs", vfs)
		if err != nil {
			return nil, uri, err
		}
	}

	db, err := sql.Open("sqlite3", "vfs.db?vfs=httpvfs")
	return db, uri, err
}

func querySQLWithHTTPVFS(c echo.Context, db *sql.DB, cmd SQLExecuteCommandArgs) (interface{}, 
	interface{}, interface{}, int, error) {
	rows, err := db.Query(cmd.CMD, cmd.Args...)
	if err != nil {
		c.Logger().Error("Query err: ", err)
		return nil, nil, nil, 0, err
	}

	var columns []string
	var values []interface{}

	cols, err := rows.Columns()
	if err != nil {
		c.Logger().Error("Columns err: ", err)
		return nil, nil, nil, 0, err
	}

	for _, column := range cols {
		columns = append(columns, column)
	}

	types, err := rows.ColumnTypes()
	if err != nil {
		c.Logger().Error("ColumnTypes err: ", err)
		return nil, nil, nil, 0, err
	}
	xTypes := make([]string, len(types))
	for i := range types {
		xTypes[i] = strings.ToLower(types[i].DatabaseTypeName())
	}

	for rows.Next() {
		rows.Columns()
		columns := make([]*string, len(cols))
		columnPointers := make([]interface{}, len(cols))
		for i := range columns {
			columnPointers[i] = &columns[i]
		}

		err = rows.Scan(columnPointers...)
		if err != nil {
			c.Logger().Error("Scan err: ", err)
			return columns, nil, xTypes, 0, err
		}

		names := make([]string, 0, len(columns))
		for _, col := range columns {
			if col == nil {
				names = append(names, "NULL")
			} else {
				names = append(names, *col)
			}
		}
		values = append(values, names)
	}

	if err = rows.Close(); err != nil {
		c.Logger().Error("Close err: ", err)
		return nil, nil, nil, 0, err
	}

	return columns, values, xTypes, len(values), nil
}

func execSQLWithHTTPVFS(c echo.Context, db *sql.DB, cmd SQLExecuteCommandArgs) error {
	result, err := db.Exec(cmd.CMD, cmd.Args...)
	if err != nil {
		c.Logger().Error("Execute err: ", err)
		return err
	}

	lastInsertId, _ := result.LastInsertId()
	rowsAffected, _ := result.RowsAffected()
	c.Logger().Info("lastInsertId: ", lastInsertId, ", rowsAffected: ", rowsAffected)
	return nil
}

func GetFileDB(dbName string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbName)
	return db, err
}

type SQLExecuteCommandArgs struct {
	CMD  string        `json:"cmd"`
	Args []interface{} `json:"args"`
}

func ExecuteSQLWithFile(c echo.Context, db *sql.DB, sqlList []SQLExecuteCommandArgs) error {
	for _, sqlv := range sqlList {
		c.Logger().Info("CMD: ", sqlv.CMD, ", sqlv.Args: ", sqlv.Args)
		_, err := db.Exec(sqlv.CMD, sqlv.Args...)
		if err != nil {
			c.Logger().Error("Execute err: ", err, ", sqlv: ", sqlv)
			return err
		}
	}
	return nil
}

// Create system table sql template
const systemInitSQLTemplate = `CREATE TABLE __TAMISYSTEM__ (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT UNIQUE NOT NULL,
	value TEXT NOT NULL,
	create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO __TAMISYSTEM__ (name, value)
VALUES ('__version', '1'),
	   ('__tamiversion', ?),
       ('__readkey', ?),
	   ('__writekey', ?);
`

func initSQLWithSystem(c echo.Context, db *sql.DB, readKey, writeKey string) error {
	_, err := db.Exec(systemInitSQLTemplate, utils.VERSION, readKey, writeKey)
	return err
}
