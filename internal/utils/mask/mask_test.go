package mask

import "testing"

func TestIsSensitiveKey(t *testing.T) {
	sensitive := []string{
		"password", "user_password", "secret", "api_key", "access_key",
		"token", "credential", "dsn", "authorization", "API_KEY", "Access_Token",
	}
	for _, k := range sensitive {
		if !IsSensitiveKey(k) {
			t.Errorf("IsSensitiveKey(%q) = false, want true", k)
		}
	}
	notSensitive := []string{"name", "id", "created_at", "tokens", "secretary", "tokenizer"}
	for _, k := range notSensitive {
		if IsSensitiveKey(k) {
			t.Errorf("IsSensitiveKey(%q) = true, want false", k)
		}
	}
	// 正则以 `_` 为边界，`password_hint_x` 中间的 password 仍视为敏感字段。
	if !IsSensitiveKey("password_hint_x") {
		t.Error("password_hint_x should match: the pattern anchors on underscore boundaries")
	}
}

func TestSecret(t *testing.T) {
	if Secret("") != "" {
		t.Error("empty string should stay empty")
	}
	// 长度 <= 8 全部遮蔽。
	if got := Secret("short"); got != "••••••••" {
		t.Errorf("Secret(short) = %q", got)
	}
	if got := Secret("12345678"); got != "••••••••" {
		t.Errorf("Secret(8 chars) = %q", got)
	}
	// 保留前 3 后 4。
	if got := Secret("sk-live-1234567890"); got != "sk-...7890" {
		t.Errorf("Secret(long) = %q, want %q", got, "sk-...7890")
	}
}
