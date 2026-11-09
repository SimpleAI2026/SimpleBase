package sqlguard

import "testing"

func TestValidateRejectsReservedIdentifiers(t *testing.T) {
	cases := []string{
		`SELECT * FROM __sb_kv_keys`,
		`select * from "__SB_KV_STRINGS"`,
		"SELECT * FROM `__sb_kv_lists`",
		`SELECT * FROM sqlite_master`,
		`DROP TABLE __sb_kv_keys`,
		`SELECT /* c */ * FROM __sb_kv_sets`,
		"SELECT *\nFROM\n__sb_kv_hashes",
	}
	for _, sql := range cases {
		for _, intent := range []Intent{ReadOnly, WriteAllowed} {
			if err := Validate(sql, intent); err == nil {
				t.Fatalf("expected %q to be rejected (intent %d)", sql, intent)
			}
		}
	}
	for _, sql := range []string{`SELECT '__sb_kv_keys'`, `SELECT 1 -- __sb_kv_keys`, `SELECT * FROM users`} {
		if err := Validate(sql, ReadOnly); err != nil {
			t.Fatalf("expected %q to pass: %v", sql, err)
		}
	}
}
