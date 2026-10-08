package sqlguard

import "testing"

func TestSplitStatements(t *testing.T) {
	parts, err := SplitStatements("CREATE TABLE t (id INTEGER); INSERT INTO t VALUES (1)")
	if err != nil || len(parts) != 2 {
		t.Fatalf("parts=%v err=%v", parts, err)
	}
	parts, err = SplitStatements("INSERT INTO t VALUES('a;b')")
	if err != nil || len(parts) != 1 {
		t.Fatalf("string semicolon: %v %v", parts, err)
	}
	parts, err = SplitStatements("SELECT 1; -- tail")
	if err != nil || len(parts) != 2 {
		t.Fatalf("comment tail: %#v %v", parts, err)
	}
	if FirstKeyword(parts[1]) != "" {
		t.Fatalf("comment keyword %q", FirstKeyword(parts[1]))
	}
	if _, err := SplitStatements("SELECT \x00 1"); err != ErrNulChar {
		t.Fatalf("nul: %v", err)
	}
	parts, err = SplitStatements("  ")
	if err != nil || len(parts) != 0 {
		t.Fatalf("blank: %#v %v", parts, err)
	}
}
