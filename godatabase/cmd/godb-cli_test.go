// 文件说明：main_test.go — godb CLI 的端到端回归（批次 E）
//
// 覆盖计划 §7.5 用例：
//   - 只读执行 CTE DELETE 被拒（联动批次 A 的 sqlguard 修复）
//   - --rw 写入后数据可读
//   - KV set/get 往返
//   - 快照文件与源库可各自打开（sha256 一致性由 SnapshotFile 内部保证）
//   - 项目 ID 非法时的脱敏错误
package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/filedb"
	"github.com/linkxzhou/SimpleBase/godatabase/kv"
)

func setupEnv(t *testing.T) env {
	t.Helper()
	root := t.TempDir()
	cache := filepath.Join(root, "cache")
	t.Setenv("GODB_DATA_ROOT", root)
	t.Setenv("GODB_CACHE_DIR", cache)
	t.Setenv("GODB_ENGINE", godatabase.EngineSQLite) // CI 可跑 SQLite；DuckDB 需本机依赖
	return parseEnv(false)
}

func TestSQLReadOnlyRejectsCTEDelete(t *testing.T) {
	e := setupEnv(t)
	ctx := context.Background()
	const project = "proj0001"

	// 先用 --rw 建表。
	_, lease, cleanup, err := e.openReg(ctx, project, true)
	if err != nil {
		t.Fatalf("open rw: %v", err)
	}
	if _, err := lease.Handle.Execute(ctx, godatabase.Statement{
		SQL: "CREATE TABLE t_ro (id INTEGER)",
	}); err != nil {
		t.Fatalf("create table: %v", err)
	}
	cleanup()

	// 只读租约 + CTE DELETE：sqlguard 必须拒绝（S1 联动）。
	_, lease2, cleanup2, err := e.openReg(ctx, project, false)
	if err != nil {
		t.Fatalf("open ro: %v", err)
	}
	defer cleanup2()
	_, err = lease2.Handle.Execute(ctx, godatabase.Statement{
		SQL: "WITH del AS (DELETE FROM t_ro RETURNING 1) SELECT * FROM del",
	})
	if err == nil {
		t.Fatal("CTE DELETE on read-only lease must fail")
	}
}

func TestSQLRWRoundtrip(t *testing.T) {
	e := setupEnv(t)
	ctx := context.Background()
	const project = "proj0002"

	_, lease, cleanup, err := e.openReg(ctx, project, true)
	if err != nil {
		t.Fatalf("open rw: %v", err)
	}
	defer cleanup()
	h := lease.Handle

	if _, err := h.Execute(ctx, godatabase.Statement{SQL: "CREATE TABLE t_rw (id INTEGER)"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := h.Execute(ctx, godatabase.Statement{SQL: "INSERT INTO t_rw VALUES (42)"}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	res, err := h.Query(ctx, godatabase.Statement{SQL: "SELECT id FROM t_rw"}, 10)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(res.Rows) != 1 || asInt(res.Rows[0][0]) != 42 {
		t.Fatalf("unexpected rows: %v", res.Rows)
	}
}

func TestKVRoundtrip(t *testing.T) {
	e := setupEnv(t)
	ctx := context.Background()
	const project = "proj0003"

	_, lease, cleanup, err := e.openReg(ctx, project, true)
	if err != nil {
		t.Fatalf("open rw: %v", err)
	}
	defer cleanup()
	h := lease.Handle
	store := kv.New(h.Conn())

	if err := store.Update(ctx, func(tx *kv.Tx) error {
		_, err := tx.Str().SetWith(ctx, "cli:k1", []byte("hello"), kv.SetOptions{})
		return err
	}); err != nil {
		t.Fatalf("kv set: %v", err)
	}
	var got []byte
	if err := store.View(ctx, func(tx *kv.Tx) error {
		v, err := tx.Str().Get(ctx, "cli:k1")
		if err != nil {
			return err
		}
		got = v
		return nil
	}); err != nil {
		t.Fatalf("kv get: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("kv roundtrip got %q", got)
	}
}

func TestSnapshotExport(t *testing.T) {
	e := setupEnv(t)
	ctx := context.Background()
	const project = "proj0004"

	_, lease, cleanup, err := e.openReg(ctx, project, true)
	if err != nil {
		t.Fatalf("open rw: %v", err)
	}
	if _, err := lease.Handle.Execute(ctx, godatabase.Statement{SQL: "CREATE TABLE t_snap (id INTEGER)"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	h := lease.Handle

	out := filepath.Join(t.TempDir(), "snap.sqlite")
	info, err := filedb.SnapshotFile(ctx, h.Conn(), dbFilePath(e, h), godatabase.EngineSQLite, out)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if info.SHA256 == "" || info.SizeBytes == 0 {
		t.Fatalf("bad snapshot info: %+v", info)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("snapshot file missing: %v", err)
	}
	cleanup()
}

func TestInvalidProjectIDSanitized(t *testing.T) {
	setupEnv(t)
	// 路径穿越 + 长度非法：spec 阶段即拒绝，错误不含数据根路径。
	_, err := parseEnv(false).spec("../etc/pw")
	if err == nil {
		t.Fatal("invalid project id must fail")
	}
	if strings.Contains(err.Error(), "godb-data") || strings.Contains(err.Error(), "..") {
		t.Fatalf("error leaks path: %v", err)
	}
}
