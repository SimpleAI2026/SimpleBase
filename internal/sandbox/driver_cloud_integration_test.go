//go:build sandbox_integration

package sandbox

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/internal/config"
)

// 真实 Cloud 冒烟不进默认 CI：go test -tags=sandbox_integration ./internal/sandbox -run TestCloudSandboxSmoke -v
func TestCloudSandboxSmoke(t *testing.T) {
	key := os.Getenv("SIMPLEBASE_SANDBOX_API_KEY")
	if key == "" {
		t.Skip("SIMPLEBASE_SANDBOX_API_KEY not configured")
	}
	cfg := config.SandboxConfig{Enabled: true, Backend: config.SandboxBackendCloud, APIKey: key,
		Image: "python:3.12-slim", CPUs: 1, MemoryMiB: 256, Network: "none", Workdir: "/workspace",
		IdleTimeout: 5 * time.Minute, MaxDuration: 30 * time.Minute, ExecTimeout: 30 * time.Second,
		MaxFileBytes: 1 << 20}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	d, err := NewCloudDriver(cfg, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	name := CloudName(uuid.NewString())
	target := Target{Name: name, Spec: Spec{Image: cfg.Image, CPUs: cfg.CPUs, MemoryMiB: cfg.MemoryMiB,
		Workdir: cfg.Workdir, IdleTimeout: cfg.IdleTimeout, MaxDuration: cfg.MaxDuration,
		Network: cfg.Network, Labels: map[string]string{"simplebase": "1"}}}
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		if err := d.Remove(cleanupCtx, name); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	result, err := d.Exec(ctx, target, ExecSpec{Cmd: "python", Args: []string{"-c", "print(1)"}, Cwd: "/workspace", Timeout: 30 * time.Second})
	if err != nil || result.ExitCode != 0 || strings.TrimSpace(string(result.Stdout)) != "1" {
		t.Fatalf("exec: %+v, %v", result, err)
	}
	if err := d.WriteFile(ctx, target, "/workspace/ci.txt", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	data, err := d.ReadFile(ctx, target, "/workspace/ci.txt")
	if err != nil || string(data) != "hello" {
		t.Fatalf("read: %q, %v", data, err)
	}
	if err := d.Stop(ctx, name); err != nil {
		t.Fatal(err)
	}
	if err := d.Ensure(ctx, target); err != nil {
		t.Fatal(err)
	}
	data, err = d.ReadFile(ctx, target, "/workspace/ci.txt")
	if err != nil || string(data) != "hello" {
		t.Fatalf("restarted file: %q, %v", data, err)
	}
}
