package catalog

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// planv6.3 阶段 0：引擎/路径纯函数的返回值与错误文本金标。
// api 层靠 errors.Is(err, ErrInvalidName) 映射 400，抽离到 godatabase 后必须保持不变。
var updateCatalogGolden = flag.Bool("update-golden", false, "rewrite testdata/project_storage.golden")

func goldenStorageLines() []string {
	var out []string
	rec := func(name string, args []any, v string, err error) {
		if err != nil {
			out = append(out, fmt.Sprintf("%s%q => ERR[invalid_name=%v] %s", name, args, errors.Is(err, ErrInvalidName), err))
			return
		}
		out = append(out, fmt.Sprintf("%s%q => %s", name, args, v))
	}
	for _, e := range []string{"", "duckdb", "DuckDB", " sqlite ", "SQLITE", "pg"} {
		v, err := NormalizeEngine(e)
		rec("NormalizeEngine", []any{e}, v, err)
	}
	for _, pid := range []string{"abcd1234", "ABCD-123", "short", "ab/cd123", "", ReservedSystemProjectID} {
		for _, e := range []string{"", "duckdb", "sqlite", "oracle"} {
			v, err := DatabasePathForProject(pid, e)
			rec("DatabasePathForProject", []any{pid, e}, v, err)
		}
	}
	paths := []string{
		"", "abcd1234/database.duckdb", "abcd1234/database.sqlite", "abcd1234/database.db",
		"/abcd1234/database.duckdb", "abcd1234//database.duckdb", "abcd1234/./database.duckdb",
		"abcd1234\\database.duckdb", "abcd1234/database.duckdb\x00", "a/b/database.duckdb",
		"abcd1234", "short/database.duckdb", "ab..d123/database.sqlite", "../database.duckdb",
	}
	for _, p := range paths {
		v, err := DatabaseEngine(p)
		rec("DatabaseEngine", []any{p}, v, err)
		rec("ValidateDatabasePath", []any{p}, "ok", ValidateDatabasePath(p))
	}
	for _, pid := range []string{"abcd1234", "wxyz9876", ReservedSystemProjectID, ""} {
		for _, p := range paths {
			rec("ValidateProjectDatabasePath", []any{pid, p}, "ok", ValidateProjectDatabasePath(pid, p))
		}
	}
	return out
}

func TestProjectStorageGolden(t *testing.T) {
	path := filepath.Join("testdata", "project_storage.golden")
	got := strings.Join(goldenStorageLines(), "\n") + "\n"
	if *updateCatalogGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v（用 -update-golden 生成）", err)
	}
	if string(want) != got {
		gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
		for i := 0; i < len(gl) && i < len(wl); i++ {
			if gl[i] != wl[i] {
				t.Fatalf("与金标不一致（第 %d 行）\n got: %s\nwant: %s", i+1, gl[i], wl[i])
			}
		}
		t.Fatalf("行数不一致: got %d want %d", len(gl), len(wl))
	}
}
