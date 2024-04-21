package client

import (
	"errors"
	"fmt"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/LessDB/internal/utils"

	"database/sql"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

var vfsCache sync.Map

// GetVFSDB get httpvfs sqlite3 file
func GetVFSDB(dbName string) (*sql.DB, string, error) {
	uri, err := s3client.GetFileLink(dbName)
	if err != nil {
		return nil, uri, err
	}
	db, err := sql.Open("sqlite3", fmt.Sprintf("%v?vfs=httpvfs&mode=rw", dbName))
	return db, uri, err
}

// QuerySQLWithHTTPVFS query sql on httpvfs
func QuerySQLWithHTTPVFS(c echo.Context, db *sql.DB, cmd SQLExecuteCommandArgs) (interface{},
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

// ExecuteSQLWithHTTPVFS execute sql on httpvfs
func ExecuteSQLWithHTTPVFS(c echo.Context, db *sql.DB, cmd SQLExecuteCommandArgs) error {
	if cmd.CMD == "" {
		return errors.New("invalid sql")
	}

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

// GetFileDB get local sqlite3 file
func GetFileDB(dbName string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbName)
	return db, err
}

type SQLExecuteCommandArgs struct {
	CMD  string        `json:"cmd"`
	Args []interface{} `json:"args"`
}

// ExecuteSQLWithFile execute sql on local sqlite3
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
const systemDBName = "__LESSDBSYSTEM__"

var systemInitSQLTemplate = fmt.Sprintf(`CREATE TABLE %v (
	name TEXT PRIMARY KEY UNIQUE NOT NULL,
	value TEXT NOT NULL,
	value_int INTEGER DEFAULT 0,
	create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO %v (name, value)
VALUES ('__version', '1'),
	   ('__lessdbversion', ?),
       ('__readkey', ?),
	   ('__writekey', ?);
`, systemDBName, systemDBName)

// SysTableInit system table init
func SysTableInit(c echo.Context, db *sql.DB, readKey, writeKey string) error {
	_, err := db.Exec(systemInitSQLTemplate, utils.VERSION, readKey, writeKey)
	return err
}

// SysTableQuerySQL get system table sql
func SysTableQuerySQL() string {
	return fmt.Sprintf(`SELECT name, value, value_int FROM %v where name = ?`, systemDBName)
}

func SysTableInsertStatus(c echo.Context, db *sql.DB, status int, name, value string) error {
	return ExecuteSQLWithFile(c, db, []SQLExecuteCommandArgs{
		SQLExecuteCommandArgs{
			CMD:  fmt.Sprintf(`INSERT INTO %v(value_int, value, name) values(?, ?, ?)`, systemDBName),
			Args: []interface{}{status, value, name},
		},
	})
}

func SysTableUpdateStatus(c echo.Context, db *sql.DB, status int, name, value string) error {
	return ExecuteSQLWithFile(c, db, []SQLExecuteCommandArgs{
		SQLExecuteCommandArgs{
			CMD:  fmt.Sprintf(`UPDATE %v SET value_int = ?, value = ? where name = ?`, systemDBName),
			Args: []interface{}{status, value, name},
		},
	})
}
