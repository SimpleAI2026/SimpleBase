package vfsextend

import (
	"fmt"

	_ "github.com/mattn/go-sqlite3"

	"database/sql"
	"testing"
)

type FooRow struct {
	ID    string
	Title string
}

func TestSqlite3vfsCreateDB100Tables(t *testing.T) {
	db, err := sql.Open("sqlite3", "test.db")
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 100; i++ {
		tableName := fmt.Sprintf("foo%v", i)
		_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS ` + tableName + ` (
	id text NOT NULL PRIMARY KEY,
	title text
)`)
		if err != nil {
			t.Fatal(err)
		}

		rows := []FooRow{
			{
				ID:    "415",
				Title: "romantic-swell",
			},
			{
				ID:    "610",
				Title: "ironically-gnarl",
			},
			{
				ID:    "768",
				Title: "biophysicist-straddled",
			},
		}

		for _, row := range rows {
			_, err = db.Exec(`INSERT INTO `+tableName+` (id, title) values (?, ?)`, row.ID, row.Title)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	err = db.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestSqlite3vfsCreateDBRow1W(t *testing.T) {
	db, err := sql.Open("sqlite3", "test100w.db")
	if err != nil {
		t.Fatal(err)
	}

	tableName := "rows1W_foo"
		_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS ` + tableName + ` (
	id text NOT NULL PRIMARY KEY,
	title text
)`)

	for i := 0; i < 10000; i++ {
		if err != nil {
			t.Fatal(err)
		}

		rows := []FooRow{
			{
				ID:    fmt.Sprintf("%v", i),
				Title: fmt.Sprintf("romantic-swell-%v", i),
			},
		}

		for _, row := range rows {
			_, err = db.Exec(`INSERT INTO `+tableName+` (id, title) values (?, ?)`, row.ID, row.Title)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	err = db.Close()
	if err != nil {
		t.Fatal(err)
	}
}

