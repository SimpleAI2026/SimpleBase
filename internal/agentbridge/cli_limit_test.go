package agentbridge

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLimitOutKeepsValidUTF8(t *testing.T) {
	if got := limitOut("短"); got != "短" {
		t.Fatalf("short string changed: %q", got)
	}
	exact := strings.Repeat("a", cliOutputLimit)
	if got := limitOut(exact); got != exact {
		t.Fatalf("exact-limit ASCII changed: len=%d", len(got))
	}
	if got := limitOut(exact + "bcd"); len(got) != cliOutputLimit {
		t.Fatalf("long ASCII len=%d, want %d", len(got), cliOutputLimit)
	}
	// 「中」3 字节，cliOutputLimit(65536) % 3 == 1：上限处切断半个字符。
	cut := limitOut(strings.Repeat("中", cliOutputLimit))
	if !utf8.ValidString(cut) {
		t.Fatal("truncated output is not valid UTF-8")
	}
	if len(cut) != cliOutputLimit-1 {
		t.Fatalf("len=%d, want %d", len(cut), cliOutputLimit-1)
	}
	// 多字节恰在边界结束时不多删：前缀 1 字节 ASCII + 若干「中」凑满上限。
	pad := strings.Repeat("a", cliOutputLimit%3) + strings.Repeat("中", cliOutputLimit/3)
	if got := limitOut(pad + "中"); got != pad {
		t.Fatalf("boundary-aligned input over-trimmed: len=%d, want %d", len(got), len(pad))
	}
}
