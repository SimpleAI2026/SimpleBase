package database

import (
	"fmt"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

func TestExampleMysql(t *testing.T) {
	xml, err := NewXmlBatis(DbConfig{
		Driver:             "mysql",
		Dsn:                "root:pYDtRb@3852kd@tcp(9.135.105.86:3306)/mytest1?charset=utf8",
		MaxOpenConnections: 5,
		MaxIdleConnections: 1,
		MaxLifeTime:        3600,
		MaxIdleTime:        1800,
	})
	if err != nil {
		panic("err: " + err.Error())
	}
	err = xml.LoadFromFile("sample", "./tests/sample.xml")
	fmt.Println("LoadFromFile err: ", err)
	preexecErr := xml.PreExec("sample.preexec", map[string]interface{}{
		"status":   1,
		"username": "xxxx",
		"limit":    10,
	})
	fmt.Println("preexec err: ", preexecErr)
	n, err := xml.Update("sample.List", map[string]interface{}{
		"status":   1,
		"username": "xxxx",
		"limit":    10,
	})
	fmt.Println("n: ", n, ", err: ", err)
}

func TestExampleSqlite(t *testing.T) {
	xml, err := NewXmlBatis(DbConfig{
		Driver:             "sqlite3",
		Dsn:                "./tests/mytest1.sqlite",
		MaxOpenConnections: 5,
		MaxIdleConnections: 1,
		MaxLifeTime:        3600,
		MaxIdleTime:        1800,
	})
	if err != nil {
		panic("err: " + err.Error())
	}
	err = xml.LoadFromFile("sample", "./tests/sample.xml")
	fmt.Println("LoadFromFile err: ", err)
	preexecErr := xml.PreExec("sample.preexec", map[string]interface{}{
		"status":   1,
		"username": "xxxx",
		"limit":    10,
	})
	fmt.Println("preexec err: ", preexecErr)
	n, err := xml.Update("sample.List", map[string]interface{}{
		"status":   1,
		"username": "xxxx",
		"limit":    10,
	})
	fmt.Println("n: ", n, ", err: ", err)
}
