//go:build sandbox_integration

package cloud

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// 真实 Cloud 冒烟不进默认 CI：
// cd gosandbox && go test -tags=sandbox_integration ./vm/cloud -run TestCloudSandboxSmoke -v
func TestCloudSandboxSmoke(t *testing.T) {
	key := os.Getenv("SIMPLEBASE_SANDBOX_API_KEY")
	if key == "" {
		t.Skip("SIMPLEBASE_SANDBOX_API_KEY not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	d, err := New(Options{APIKey: key, Home: t.TempDir(), MaxFileBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	name := "sbx-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	target := vm.Target{Name: name, Spec: vm.Spec{Image: "python:3.12-slim", CPUs: 1, MemoryMiB: 256,
		Workdir: "/workspace", IdleTimeout: 5 * time.Minute, MaxDuration: 30 * time.Minute,
		Network: "none", Labels: map[string]string{"simplebase": "1"}}}
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		if err := d.Remove(cleanupCtx, name); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	result, err := d.Exec(ctx, target, vm.ExecSpec{Cmd: "python", Args: []string{"-c", "print(1)"}, Cwd: "/workspace", Timeout: 30 * time.Second})
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
