package api

import (
	"errors"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/database/sqlguard"
)

func TestPrepareInitSQL(t *testing.T) {
	if stmts, err := prepareInitSQL("  "); err != nil || stmts != nil {
		t.Fatalf("blank: %v %v", stmts, err)
	}
	stmts, err := prepareInitSQL("CREATE TABLE users (id INTEGER); INSERT INTO users VALUES (1)")
	if err != nil || len(stmts) != 2 {
		t.Fatalf("ok: %v %v", stmts, err)
	}
	if _, err := prepareInitSQL("CREATE TABLE t (id INTEGER); DROP TABLE t"); !errors.Is(err, sqlguard.ErrSQLNotAllowed) {
		t.Fatalf("drop: %v", err)
	}
	if _, err := prepareInitSQL("SELECT 1"); !errors.Is(err, sqlguard.ErrSQLNotAllowed) {
		t.Fatalf("select: %v", err)
	}
	if _, err := prepareInitSQL("ATTACH 'x'"); !errors.Is(err, sqlguard.ErrSQLNotAllowed) {
		t.Fatalf("attach: %v", err)
	}
	if _, err := prepareInitSQL(strings.Repeat("a", maxInitSQLBytes+1)); err == nil {
		t.Fatal("expected size limit")
	}
}
