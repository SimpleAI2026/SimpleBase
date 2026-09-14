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
		"DESCRIBE t",
		"SHOW TABLES",
		"FROM t SELECT *",
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
		"INSTALL ducklake",
		"LOAD httpfs",
		"COPY t TO 'out.parquet'",
		"SET memory_limit = '1GB'",
		"CALL lake.set_option('x', 'y')",
		"CHECKPOINT lake",
		"CREATE SECRET s3 (TYPE s3)",
		"CREATE MACRO add(a, b) AS a + b",
		"USE lake",
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
		"PRAGMA table_info(users)",
		"PRAGMA database_list",
	}
	for _, sql := range denied {
		if err := Validate(sql, ReadOnly); err != ErrSQLNotAllowed {
			t.Errorf("pragma %q: got %v, want ErrSQLNotAllowed", sql, err)
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

func TestValidate_NestedBlockComments(t *testing.T) {
	// SQLite 不支持嵌套块注释，但 /* /* */  应正确识别第一个 */ 结束
	// 剩余内容应被视为关键字后续
	sql := "SELECT /* outer /* inner */ 1"
	err := Validate(sql, ReadOnly)
	if err != nil {
		t.Errorf("nested block comment: got %v, want nil", err)
	}
}

func TestValidate_StringEscapeSingleQuote(t *testing.T) {
	// 字符串内转义的单引号不应中断字符串扫描
	sql := "INSERT INTO t VALUES('it''s a test;')"
	if err := Validate(sql, WriteAllowed); err != nil {
		t.Errorf("escaped quote in string: got %v, want nil", err)
	}
}

func TestValidate_IdentifierEscapeDoubleQuote(t *testing.T) {
	// 标识符内转义的双引号
	sql := `INSERT INTO "my ""table""" VALUES(1)`
	if err := Validate(sql, WriteAllowed); err != nil {
		t.Errorf("escaped quote in identifier: got %v, want nil", err)
	}
}

func TestValidate_MultipleStatementsWithInsert(t *testing.T) {
	sql := "INSERT INTO t VALUES(1); DROP TABLE t"
	if err := Validate(sql, WriteAllowed); err != ErrMultipleStatements {
		t.Errorf("multi-stmt insert+drop: got %v, want ErrMultipleStatements", err)
	}
}

func TestValidate_SemicolonInBlockComment(t *testing.T) {
	sql := "INSERT INTO t VALUES(/* ; */ 1)"
	if err := Validate(sql, WriteAllowed); err != nil {
		t.Errorf("semicolon in block comment: got %v, want nil", err)
	}
}

func TestValidate_LineCommentAfterSemicolon(t *testing.T) {
	sql := "SELECT 1 -- comment after\n"
	// 这不是多语句，末尾无分号
	if err := Validate(sql, ReadOnly); err != nil {
		t.Errorf("single with trailing line comment: got %v, want nil", err)
	}
}

func TestValidate_WriteAllowedMergeAndCreate(t *testing.T) {
	allowed := []string{
		"MERGE INTO t USING s ON t.id = s.id WHEN MATCHED THEN UPDATE SET x = s.x WHEN NOT MATCHED THEN INSERT VALUES (s.id)",
		"CREATE TABLE t (x INT)",
		"CREATE SCHEMA analytics",
		"CREATE VIEW v AS SELECT 1",
	}
	for _, sql := range allowed {
		if err := Validate(sql, WriteAllowed); err != nil {
			t.Errorf("WriteAllowed allow %q: got %v", sql, err)
		}
	}
}

func TestValidate_FirstKeywordWithParen(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"SELECT(1)", "SELECT"},
		{"WITH(x AS (SELECT 1)) SELECT * FROM x", "WITH"},
	}
	for _, c := range cases {
		got := FirstKeyword(c.in)
		if got != c.want {
			t.Errorf("FirstKeyword(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidate_WriteAllowedAllowsCreateAndDrop(t *testing.T) {
	allowed := []string{
		"CREATE INDEX idx ON t(x)",
		"DROP INDEX idx",
		"ALTER TABLE t RENAME TO t2",
		"REPLACE INTO t VALUES(1)",
	}
	for _, sql := range allowed {
		if err := Validate(sql, WriteAllowed); err != nil {
			t.Errorf("WriteAllowed %q: got %v", sql, err)
		}
	}
}

func TestValidate_WhitespaceOnlyReturnsEmptyKeyword(t *testing.T) {
	if kw := FirstKeyword("   \t\n  "); kw != "" {
		t.Errorf("whitespace-only should return empty keyword, got %q", kw)
	}
}

func TestValidate_MixedCaseKeywords(t *testing.T) {
	// 关键字大小写无关
	cases := []string{"select 1", "Select 1", "SELECT 1"}
	for _, sql := range cases {
		if kw := FirstKeyword(sql); kw != "SELECT" {
			t.Errorf("FirstKeyword(%q) = %q, want SELECT", sql, kw)
		}
	}
}
