package sqlx

import "testing"

func TestIdent(t *testing.T) {
	cases := map[string]string{
		"t":      `"t"`,
		`a"b`:    `"a""b"`,
		`"`:      `""""`,
		"a'b":    `"a'b"`,
		"":       `""`,
		"a\x00b": "\"a\x00b\"",
	}
	for in, want := range cases {
		if got := Ident(in); got != want {
			t.Errorf("Ident(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLiteral(t *testing.T) {
	cases := map[string]string{
		"a":     "'a'",
		"a'b":   "'a''b'",
		"'":     "''''",
		"":      "''",
		`a"b`:   `'a"b'`,
		"a\\'b": `'a\''b'`,
	}
	for in, want := range cases {
		if got := Literal(in); got != want {
			t.Errorf("Literal(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsIdentByte(t *testing.T) {
	for _, c := range []byte("azAZ09_") {
		if !IsIdentByte(c) {
			t.Errorf("%q should be ident byte", c)
		}
	}
	for _, c := range []byte(` -."'$`) {
		if IsIdentByte(c) {
			t.Errorf("%q should not be ident byte", c)
		}
	}
}

func TestIsSafeIdent(t *testing.T) {
	for _, ok := range []string{"lake", "_x1", "表名", "a_b"} {
		if !IsSafeIdent(ok) {
			t.Errorf("%q should be safe", ok)
		}
	}
	for _, bad := range []string{"", "1x", "la-ke", "a b", `a"b`} {
		if IsSafeIdent(bad) {
			t.Errorf("%q should not be safe", bad)
		}
	}
}
