package api

// system_test_helpers_test.go：留在 api 的路由级用例仍需要的系统库夹具（运维 handler 的测试已迁到 internal/api/opshttp）。

import (
	"context"
	"database/sql"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"

	_ "github.com/uglyer/go-sqlite3"
)

func openSystemAPI(t *testing.T) *systemdb.Store {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := systemdb.EnsureSystemSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return systemdb.NewStoreForTest(db)
}
