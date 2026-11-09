package godatabase

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestSpecIsSystem(t *testing.T) {
	if !(Spec{Kind: KindSystem}).IsSystem() {
		t.Fatal("kind=system must be system")
	}
	for _, k := range []string{"", KindUser, "kv", "SYSTEM"} {
		if (Spec{Kind: k}).IsSystem() {
			t.Fatalf("kind %q must not be system", k)
		}
	}
}

func TestNormalizeEngine(t *testing.T) {
	ok := map[string]string{"": EngineDuckDB, "duckdb": EngineDuckDB, " DuckDB ": EngineDuckDB, "sqlite": EngineSQLite, "SQLITE": EngineSQLite}
	for in, want := range ok {
		got, err := NormalizeEngine(in)
		if err != nil || got != want {
			t.Fatalf("NormalizeEngine(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	_, err := NormalizeEngine("pg")
	if !errors.Is(err, ErrInvalidPath) || !strings.Contains(err.Error(), `"pg"`) {
		t.Fatalf("pg: %v", err)
	}
}

func TestPathForProjectAndEngineFromPath(t *testing.T) {
	p, err := PathForProject("abcd1234", "")
	if err != nil || p != "abcd1234/database.duckdb" {
		t.Fatalf("duckdb path = %q, %v", p, err)
	}
	p, err = PathForProject("abcd1234", "sqlite")
	if err != nil || p != "abcd1234/database.sqlite" {
		t.Fatalf("sqlite path = %q, %v", p, err)
	}
	if _, err := PathForProject("short", "duckdb"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("bad project id: %v", err)
	}
	if _, err := PathForProject("abcd1234", "oracle"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("bad engine: %v", err)
	}

	for path, want := range map[string]string{"abcd1234/database.duckdb": EngineDuckDB, "abcd1234/database.sqlite": EngineSQLite} {
		got, err := EngineFromPath(path)
		if err != nil || got != want {
			t.Fatalf("EngineFromPath(%q) = %q, %v", path, got, err)
		}
	}
	if _, err := EngineFromPath("abcd1234/database.db"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("bad ext: %v", err)
	}
}

func TestValidatePathRejectsUnsafe(t *testing.T) {
	bad := []string{
		"", "/abcd1234/database.duckdb", "abcd1234//database.duckdb", "abcd1234/./database.duckdb",
		`abcd1234\database.duckdb`, "abcd1234/database.duckdb\x00", "a/b/database.duckdb", "abcd1234",
		"short/database.duckdb", "ab..d123/database.sqlite", "../database.duckdb", "abcd1234/other.duckdb",
	}
	for _, p := range bad {
		if err := ValidatePath(p); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("ValidatePath(%q) = %v, want ErrInvalidPath", p, err)
		}
	}
	if err := ValidatePath("abcd1234/database.duckdb"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateProjectPath(t *testing.T) {
	const sys = "sb-admin"
	if err := ValidateProjectPath(sys, sys, ""); err != nil {
		t.Fatalf("system project with empty path: %v", err)
	}
	if err := ValidateProjectPath(sys, sys, "sb-admin/database.duckdb"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("system project must reject a path: %v", err)
	}
	if err := ValidateProjectPath(sys, "abcd1234", "abcd1234/database.sqlite"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateProjectPath(sys, "wxyz9876", "abcd1234/database.sqlite"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("path of another project must be rejected: %v", err)
	}
	// 空的 systemProjectID 表示"无系统项目概念"，空 projectID 不得因此被当成系统项目放行。
	if err := ValidateProjectPath("", "", ""); err == nil {
		t.Fatal("empty system id must not turn an empty project into a system project")
	}
}

func TestRecordingLoggerAndObserver(t *testing.T) {
	l := &RecordingLogger{}
	l.Debug("d")
	l.Info("registry: opened", zap.String("k", "v"))
	l.Warn("w")
	l.Error("registry: failed")
	if len(l.Entries()) != 4 {
		t.Fatalf("entries = %d", len(l.Entries()))
	}
	if l.Count("", "registry:") != 2 || l.Count("error", "registry:") != 1 || l.Count("warn", "registry:") != 0 {
		t.Fatalf("count mismatch: %+v", l.Entries())
	}

	o := &RecordingObserver{}
	o.OpenHandleDelta(1)
	o.OpenHandleDelta(1)
	o.OpenHandleDelta(-1)
	o.CacheBytes(42)
	o.CacheEvicted()
	o.CatalogLocalAhead()
	o.CatalogSync("ok", time.Second, false)
	o.CatalogSync("error", time.Millisecond, true)
	o.CatalogSyncLag("db", 3)
	o.CatalogSyncConflict("db")
	o.CatalogSize("db", 99)
	if o.Handles.Load() != 1 || o.Bytes.Load() != 42 || o.Evictions.Load() != 1 || o.LocalAhead.Load() != 1 {
		t.Fatalf("counters: %+v", o)
	}
	if o.SyncOK.Load() != 1 || o.SyncFailed.Load() != 1 || o.LastOutcome() != "error" || o.Conflicts.Load() != 1 {
		t.Fatalf("sync counters: ok=%d failed=%d outcome=%s", o.SyncOK.Load(), o.SyncFailed.Load(), o.LastOutcome())
	}
	if v, ok := o.Lag("db"); !ok || v != 3 {
		t.Fatalf("lag = %d %v", v, ok)
	}
	if v, ok := o.CatalogBytes("db"); !ok || v != 99 {
		t.Fatalf("catalog bytes = %d %v", v, ok)
	}
	if _, ok := o.Lag("none"); ok {
		t.Fatal("unknown db must report not found")
	}

	var n NopObserver
	n.OpenHandleDelta(1)
	n.CacheBytes(1)
	n.CacheEvicted()
	n.CatalogSync("ok", 0, false)
	n.CatalogSyncLag("d", 0)
	n.CatalogSyncConflict("d")
	n.CatalogSize("d", 0)
	n.CatalogLocalAhead()
}
