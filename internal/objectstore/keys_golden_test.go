package objectstore

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// planv6.3 阶段 0：数据库相关 key 布局金标。
// key 布局是 S3 上已落盘数据的磁盘契约，抽离 godatabase 前后必须字节级一致。
// 错误只记录"是否报错"，不记录文本：key 构造的报错文本属于库内部，不是磁盘契约。
// 用 -update-golden 重新生成；正常运行只比对。
var updateGolden = flag.Bool("update-golden", false, "rewrite testdata/keys.golden")

func goldenKeyLines() []string {
	prefixes := []string{"", "tenant-root", "/a/b/"}
	projects := []string{"abcd1234", "ABCD-123", "short", "ab/cd123", ""}
	const dbID = "0123456789abcdef0123456789abcdef"
	const dbUUID = "01234567-89ab-cdef-0123-456789abcdef"

	var out []string
	line := func(name string, args ...any) func(string, error) {
		return func(v string, err error) {
			if err != nil {
				out = append(out, fmt.Sprintf("%s%v => ERR", name, args))
				return
			}
			out = append(out, fmt.Sprintf("%s%v => %s", name, args, v))
		}
	}
	for _, rp := range prefixes {
		kb := KeyBuilder{RootPrefix: rp}
		out = append(out, fmt.Sprintf("## root=%q", rp))
		out = append(out, fmt.Sprintf("RootListingPrefix => %q", kb.RootListingPrefix()))
		out = append(out, fmt.Sprintf("InstanceFormatKey => %s", kb.InstanceFormatKey()))
		out = append(out, fmt.Sprintf("ResetLockKey => %s", kb.ResetLockKey()))
		out = append(out, fmt.Sprintf("SystemPrefix => %s", kb.SystemPrefix()))
		out = append(out, fmt.Sprintf("CatalogPrefix => %s", kb.CatalogPrefix()))
		out = append(out, fmt.Sprintf("SystemKey => %s", kb.SystemKey("/x/y")))
		for _, pid := range projects {
			line("ProjectDatabasePrefix", pid)(kb.ProjectDatabasePrefix(pid))
			line("ProjectDatabaseSnapshotKey", pid, 1, 1, ".duckdb")(kb.ProjectDatabaseSnapshotKey(pid, 1, 1, ".duckdb"))
			line("ProjectDatabaseSnapshotKey", pid, 12, 3, ".sqlite")(kb.ProjectDatabaseSnapshotKey(pid, 12, 3, ".sqlite"))
			line("ProjectDatabaseSnapshotKey", pid, 0, 1, ".duckdb")(kb.ProjectDatabaseSnapshotKey(pid, 0, 1, ".duckdb"))
			line("ProjectDatabaseSnapshotKey", pid, 1, 1, ".bin")(kb.ProjectDatabaseSnapshotKey(pid, 1, 1, ".bin"))
			line("ProjectDatabaseManifestKey", pid, 7)(kb.ProjectDatabaseManifestKey(pid, 7))
			line("ProjectDatabaseManifestKey", pid, 0)(kb.ProjectDatabaseManifestKey(pid, 0))
			line("ProjectDatabaseLeaseKey", pid, 9)(kb.ProjectDatabaseLeaseKey(pid, 9))
			line("ProjectDatabaseLeaseKey", pid, 0)(kb.ProjectDatabaseLeaseKey(pid, 0))
			for _, did := range []string{dbUUID, dbID, ""} {
				for _, eng := range []string{"duckdb", "sqlite", "", "oracle"} {
					line("DuckLakeSnapshotKey", pid, did, 5, 2, eng)(kb.DuckLakeSnapshotKey(pid, did, 5, 2, eng))
				}
				line("DuckLakeSnapshotKey", pid, did, 0, 2, "duckdb")(kb.DuckLakeSnapshotKey(pid, did, 0, 2, "duckdb"))
				line("DuckLakeSnapshotKey", pid, did, 5, -1, "duckdb")(kb.DuckLakeSnapshotKey(pid, did, 5, -1, "duckdb"))
				line("DuckLakeManifestKey", pid, did, 4)(kb.DuckLakeManifestKey(pid, did, 4))
				line("DuckLakeManifestKey", pid, did, 0)(kb.DuckLakeManifestKey(pid, did, 0))
				line("DuckLakeLeaseKey", pid, did, 3)(kb.DuckLakeLeaseKey(pid, did, 3))
				line("DuckLakeLeaseKey", pid, did, 0)(kb.DuckLakeLeaseKey(pid, did, 0))
				line("DuckLakeOrphanLedgerKey", pid, did, "2026-10-26", "u1")(kb.DuckLakeOrphanLedgerKey(pid, did, "2026-10-26", "u1"))
				line("DuckLakeOrphanLedgerKey", pid, did, "", "u1")(kb.DuckLakeOrphanLedgerKey(pid, did, "", "u1"))
				line("DuckLakeDataURI", "bkt", pid, did)(kb.DuckLakeDataURI("bkt", pid, did))
				line("DuckLakeDataURI", "", pid, did)(kb.DuckLakeDataURI("", pid, did))
			}
		}
	}
	for _, eng := range []string{"duckdb", "DuckDB", " sqlite ", "", "pg"} {
		line("CatalogFileExt", eng)(CatalogFileExt(eng))
	}
	out = append(out, fmt.Sprintf("SystemPrefix const => %s", SystemPrefix))
	out = append(out, fmt.Sprintf("ProjectIDLen const => %d", ProjectIDLen))
	for _, id := range []string{"abcd1234", "ABCD-123", "abcd123", "abcd12345", "abcd..12", "abcd/123", "abcd_123", ""} {
		verdict := "ok"
		if ValidateProjectID(id) != nil {
			verdict = "ERR"
		}
		out = append(out, fmt.Sprintf("ValidateProjectID(%q) => %s", id, verdict))
	}
	return out
}

func TestKeysGolden(t *testing.T) {
	path := filepath.Join("testdata", "keys.golden")
	got := strings.Join(goldenKeyLines(), "\n") + "\n"
	if *updateGolden {
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
				t.Fatalf("key 布局与金标不一致（第 %d 行）\n got: %s\nwant: %s", i+1, gl[i], wl[i])
			}
		}
		t.Fatalf("key 布局与金标行数不一致: got %d want %d", len(gl), len(wl))
	}
}
