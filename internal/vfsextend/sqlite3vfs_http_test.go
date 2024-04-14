package vfsextend

import (
	"github.com/google/go-cmp/cmp"
	"github.com/linkxzhou/LessDB/internal/sqlite3vfs"
	_ "github.com/mattn/go-sqlite3"

	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type FooRow struct {
	ID    string
	Title string
}

func TestSqlite3vfsHTTP(t *testing.T) {
	absTestDir, _ := os.Getwd()
	dir, err := os.MkdirTemp(absTestDir, "sqlite3vfs_httpdir_")
	if err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite3", filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS foo (
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
		_, err = db.Exec(`INSERT INTO foo (id, title) values (?, ?)`, row.ID, row.Title)
		if err != nil {
			t.Fatal(err)
		}
	}

	err = db.Close()
	if err != nil {
		t.Fatal(err)
	}

	s := httptest.NewServer(http.FileServer(http.Dir(dir)))

	cacheFile, err := os.Create(filepath.Join(dir, "cache"))
	if err != nil {
		t.Fatal(err)
	}

	vfs := HttpVFS{
		URL:          s.URL + "/test.db",
		CacheHandler: NewDiskCache(cacheFile, -1),
	}

	err = sqlite3vfs.RegisterVFS("httpvfs", &vfs)
	if err != nil {
		t.Fatal(err)
	}

	db, err = sql.Open("sqlite3", "test.db?vfs=httpvfs")
	if err != nil {
		t.Fatal(err)
	}

	rowIter, err := db.Query(`SELECT id, title from foo order by id`)
	if err != nil {
		t.Fatal(err)
	}

	var gotRows []FooRow

	for rowIter.Next() {
		var row FooRow
		err = rowIter.Scan(&row.ID, &row.Title)
		if err != nil {
			t.Fatal(err)
		}
		gotRows = append(gotRows, row)
	}

	err = rowIter.Close()
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(rows, gotRows) {
		t.Fatal(cmp.Diff(rows, gotRows))
	}

	err = db.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestSqlite3vfsDelete(t *testing.T) {
	absTestDir, _ := os.Getwd()
	dir, err := os.MkdirTemp(absTestDir, "sqlite3vfs_httpdir_")
	if err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite3", filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS foo (
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
		_, err = db.Exec(`INSERT INTO foo (id, title) values (?, ?)`, row.ID, row.Title)
		if err != nil {
			t.Fatal(err)
		}
	}

	err = db.Close()
	if err != nil {
		t.Fatal(err)
	}

	s := httptest.NewServer(http.FileServer(http.Dir(dir)))
	vfs := HttpVFS{
		URL: s.URL + "/test.db",
	}

	err = sqlite3vfs.RegisterVFS("httpvfs", &vfs)
	if err != nil {
		t.Fatal(err)
	}

	db, err = sql.Open("sqlite3", "test.db?vfs=httpvfs")
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`DELETE from foo where id = 415`)
	if err != nil {
		t.Fatal(err)
	}

	err = db.Close()
	if err != nil {
		t.Fatal(err)
	}
}
