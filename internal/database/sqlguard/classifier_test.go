package sqlguard

import (
	"testing"
)

func TestFirstKeyword(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"SELECT * FROM t", "SELECT"},
		{"  select 1", "SELECT"},
		{"-- comment\nINSERT INTO t VALUES(1)", "INSERT"},
		{"/* block */ WITH x AS (SELECT 1) SELECT * FROM x", "WITH"},
		{"  \n\t  EXPLAIN SELECT 1", "EXPLAIN"},
		{"pragma integrity_check", "PRAGMA"},
		{"", ""},
		{"  -- only comment\n", ""},
	}
	for _, c := range cases {
		got := FirstKeyword(c.in)
		if got != c.want {
			t.Errorf("FirstKeyword(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidate_EmptyAndNul(t *testing.T) {
	if err := Validate("", ReadOnly); err != ErrEmptySQL {
		t.Errorf("empty sql: got %v, want ErrEmptySQL", err)
	}
	if err := Validate("SELECT\x00 1", ReadOnly); err != ErrNulChar {
		t.Errorf("nul char: got %v, want ErrNulChar", err)
	}
}

func TestValidate_MultipleStatements(t *testing.T) {
	if err := Validate("SELECT 1; SELECT 2", ReadOnly); err != ErrMultipleStatements {
		t.Errorf("multi-stmt: got %v, want ErrMultipleStatements", err)
	}
	// 末尾单个分号不算多语句
	if err := Validate("SELECT 1;", ReadOnly); err != nil {
		t.Errorf("trailing semicolon: got %v, want nil", err)
	}
	// 分号后只有注释不算多语句
	if err := Validate("SELECT 1; -- comment", ReadOnly); err != nil {
		t.Errorf("semicolon then comment: got %v, want nil", err)
	}
}

func TestValidate_ReadOnly(t *testing.T) {
	allowed := []string{
		"SELECT * FROM t",
		"WITH x AS (SELECT 1) SELECT * FROM x",
		"EXPLAIN SELECT 1",
		"PRAGMA integrity_check",
		"PRAGMA table_info(t)",
	}
	for _, sql := range allowed {
		if err := Validate(sql, ReadOnly); err != nil {
			t.Errorf("ReadOnly allow %q: got %v", sql, err)
		}
	}

	denied := []string{
		"INSERT INTO t VALUES(1)",
		"UPDATE t SET x=1",
		"DELETE FROM t",
		"CREATE TABLE t (x INT)",
		"DROP TABLE t",
		"ALTER TABLE t ADD COLUMN x",
	}
	for _, sql := range denied {
		err := Validate(sql, ReadOnly)
		if err != ErrWriteInReadOnly {
			t.Errorf("ReadOnly deny %q: got %v, want ErrWriteInReadOnly", sql, err)
		}
	}
}

func TestValidate_WriteAllowed(t *testing.T) {
	allowed := []string{
		"INSERT INTO t VALUES(1)",
		"UPDATE t SET x=1",
		"DELETE FROM t",
		"CREATE TABLE t (x INT)",
		"DROP TABLE t",
		"SELECT * FROM t",
	}
	for _, sql := range allowed {
		if err := Validate(sql, WriteAllowed); err != nil {
			t.Errorf("WriteAllowed allow %q: got %v", sql, err)
		}
	}
}

func TestValidate_AlwaysDenied(t *testing.T) {
	denied := []string{
		"ATTACH DATABASE 'foo.db' AS foo",
		"DETACH foo",
		"LOAD_EXTENSION('evil.so')",
	}
	for _, sql := range denied {
		if err := Validate(sql, WriteAllowed); err != ErrSQLNotAllowed {
			t.Errorf("always-deny %q: got %v, want ErrSQLNotAllowed", sql, err)
		}
	}
}

func TestValidate_DangerousPragma(t *testing.T) {
	denied := []string{
		"PRAGMA writable_schema=1",
		"PRAGMA load_extension('evil.so')",
	}
	for _, sql := range denied {
		if err := Validate(sql, ReadOnly); err != ErrSQLNotAllowed {
			t.Errorf("dangerous pragma %q: got %v, want ErrSQLNotAllowed", sql, err)
		}
	}
}

func TestValidate_OnlyComment(t *testing.T) {
	if err := Validate("-- just a comment", ReadOnly); err != ErrEmptySQL {
		t.Errorf("comment-only: got %v, want ErrEmptySQL", err)
	}
}

func TestValidate_StringLiteralWithSemicolon(t *testing.T) {
	// 字符串内的分号不应触发多语句检测
	sql := "INSERT INTO t VALUES('a;b')"
	if err := Validate(sql, WriteAllowed); err != nil {
		t.Errorf("string with semicolon: got %v, want nil", err)
	}
}

func TestValidate_BlockCommentWithSemicolon(t *testing.T) {
	sql := "SELECT /* ; ; */ 1"
	if err := Validate(sql, ReadOnly); err != nil {
		t.Errorf("block comment with semicolon: got %v, want nil", err)
	}
}
