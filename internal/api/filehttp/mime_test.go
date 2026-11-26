package filehttp

// mime_test.go：内联可展示 MIME 的判定（自 internal/api/production_entry_test.go 随实现迁入）。

import "testing"

func TestInlineSafeMIME(t *testing.T) {
	for _, mt := range []string{"image/png", "image/jpeg", "image/gif", "image/webp", "application/pdf", "text/plain"} {
		if !inlineSafeMIME(mt) {
			t.Fatalf("%s should be inline-safe", mt)
		}
	}
	for _, mt := range []string{"image/svg+xml", "text/html", "application/javascript", "", "application/octet-stream"} {
		if inlineSafeMIME(mt) {
			t.Fatalf("%s must not be inline-safe", mt)
		}
	}
}

// WithClock 注入时钟；仅在测试里替换 loginGuard 的时间源（planv6.6 §1.3 T-3）。
