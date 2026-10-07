package sandbox

import (
	"strings"
	"sync"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/config"
)

func TestSandboxName(t *testing.T) {
	got := SandboxName("550e8400-e29b-41d4-a716-446655440000")
	want := "sb-550e8400e29b41d4a716446655440000"
	if got != want {
		t.Fatalf("SandboxName = %q, want %q", got, want)
	}
	if len(got) > 128 {
		t.Fatalf("name exceeds 128 bytes: %d", len(got))
	}
	if SandboxName("ABC-DEF") != "sb-abcdef" {
		t.Fatalf("SandboxName should lowercase and strip dashes, got %q", SandboxName("ABC-DEF"))
	}
}

func newTestClient(t *testing.T) *Client {
	t.Helper()
	return &Client{cfg: normalizeConfig(config.SandboxConfig{Enabled: true, APIKey: "k"}), locks: map[string]*sync.Mutex{}}
}

func TestValidPath(t *testing.T) {
	c := newTestClient(t)
	ok := []string{
		"/workspace/a.txt",
		"/workspace/sub/b.py",
		"/workspace//nested//f.txt", // Clean 后合法
	}
	for _, p := range ok {
		if _, err := c.validPath(p); err != nil {
			t.Errorf("validPath(%q) = %v, want nil", p, err)
		}
	}
	bad := map[string]string{
		"":                "empty",
		"relative.txt":    "relative",
		"/etc/passwd":     "outside",
		"/workspace/../etc/passwd": "escape",
		"/workspacex/f":   "prefix trick",
		"/workspace":      "dir itself",
	}
	for p := range bad {
		if _, err := c.validPath(p); err == nil {
			t.Errorf("validPath(%q) = nil, want error", p)
		}
	}
	// Clean 后的 /workspace/../etc 应被拒绝。
	if _, err := c.validPath("/workspace/../etc/passwd"); err == nil {
		t.Fatal("path with .. must be rejected")
	}
}

func TestValidPathCleanPrefix(t *testing.T) {
	c := newTestClient(t)
	got, err := c.validPath("/workspace//sub/../a.txt")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "/workspace/a.txt" {
		t.Fatalf("cleaned path = %q, want /workspace/a.txt", got)
	}
}

func TestAvailableDisabled(t *testing.T) {
	var nilClient *Client
	if nilClient.Available() {
		t.Fatal("nil client must not be available")
	}
	c := &Client{cfg: config.SandboxConfig{}, locks: map[string]*sync.Mutex{}}
	if c.Available() {
		t.Fatal("disabled config must not be available")
	}
}

func TestNewDisabledWithoutKey(t *testing.T) {
	c := New(config.SandboxConfig{Enabled: true, APIKey: ""}, "", nil)
	if c.Available() {
		t.Fatal("enabled without key must stay unavailable")
	}
}

func TestNormalizeConfigDefaults(t *testing.T) {
	cfg := normalizeConfig(config.SandboxConfig{Enabled: true})
	if cfg.Image != "python:3.12-slim" || cfg.CPUs != 1 || cfg.MemoryMiB != 256 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.Network != "none" || cfg.Workdir != "/workspace" {
		t.Fatalf("unexpected network/workdir: %+v", cfg)
	}
	if cfg.MaxOutputBytes != 65536 || cfg.MaxFileBytes != 1<<20 {
		t.Fatalf("unexpected byte limits: %+v", cfg)
	}
}

func TestTruncateBytes(t *testing.T) {
	s := strings.Repeat("a", 100)
	if got := truncateBytes(s, 50); len(got) != 50 {
		t.Fatalf("truncateBytes len = %d, want 50", len(got))
	}
	if got := truncateBytes(s, 200); got != s {
		t.Fatal("truncateBytes must not alter short strings")
	}
	if got := truncateBytes(s, 0); got != s {
		t.Fatal("max<=0 means no truncation")
	}
}

func TestWrapExecErrPassthrough(t *testing.T) {
	err := wrapExecErr(nil)
	if err != nil {
		t.Fatalf("wrapExecErr(nil) = %v, want nil", err)
	}
	msg := wrapExecErr(errTestSentinel)
	if msg == nil || !strings.Contains(msg.Error(), "sentinel") {
		t.Fatalf("wrapExecErr should wrap: %v", msg)
	}
}

var errTestSentinel = errorString("sandbox: sentinel")

type errorString string

func (e errorString) Error() string { return string(e) }
