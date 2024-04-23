package main

import (
	"errors"

	"github.com/dolthub/go-mysql-server/sql"
)

var (
	ErrNoSQLiteConn         = errors.New("could not retrieve SQLite connection")
	ErrNoInsertsAllowed     = errors.New("table does not permit INSERTs")
	ErrNoCreateTableAllowed = errors.New("database does not permit creating tables")
	ErrCouldNotFindDatabase = errors.New("could not find database")
)

func (db *Database) Name() string {
	return db.name
}

func (db *Database) GetTableInsensitive(ctx *sql.Context, tblName string) (sql.Table, bool, error) {
	return nil, false, errors.New("Not support")
}

func (db *Database) GetTableNames(ctx *sql.Context) ([]string, error) {
	return nil, ErrNoSQLiteConn
}

func (db *Database) CreateTable(ctx *sql.Context, name string, schema sql.Schema) error {
	return errors.New("Not support")
}
