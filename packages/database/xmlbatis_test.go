package database

import (
	"fmt"
	"testing"
)

func TestExample(t *testing.T) {
	xml := NewXmlBatis()
	err := xml.LoadFromFile("sample", "./tests/sample.xml")
	fmt.Println("LoadFromFile err: ", err)
	initErr := xml.Init("sample.init", map[string]interface{}{
		"status":   1,
		"username": "xxxx",
		"limit":    10,
	}, func(stmt *BatisStmt, queryer *Queryer) error {
		fmt.Println("Init queryer.Sql: ", queryer.Sql)
		return nil
	})
	fmt.Println("Init err: ", initErr)
	n, err := xml.Update("sample.List", map[string]interface{}{
		"status":   1,
		"username": "xxxx",
		"limit":    10,
	})
	fmt.Println("n: ", n, ", err: ", err)
}
