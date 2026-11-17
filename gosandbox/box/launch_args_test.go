//go:build linux

package box

// launch_args_test.go：沙盒根参数的失败语义。
//
// 用户请求了镜像（Lower 非空）却构造不出镜像根时，必须报错，
// 不能悄悄把宿主根只读挂进去。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// overlay 的 upper/work 建不出来：返回错误，而不是宿主根。
func TestRootArgsOverlayDirFailureIsAnError(t *testing.T) {
	overlayMu.Lock()
	old := overlayOK
	yes := true
	overlayOK = &yes // 假装 bwrap 支持 --overlay，避免依赖真 bwrap
	overlayMu.Unlock()
	t.Cleanup(func() { overlayMu.Lock(); overlayOK = old; overlayMu.Unlock() })

	root := t.TempDir()
	// upper 已经是一个普通文件：MkdirAll 必然失败。
	if err := os.WriteFile(filepath.Join(root, "upper"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	args, err := rootArgs(Config{Bwrap: "/usr/bin/bwrap", Root: root, Lower: t.TempDir()})
	if err == nil {
		t.Fatalf("must fail instead of falling back to the host root: %v", args)
	}
	if args != nil {
		t.Fatalf("no mount args on error: %v", args)
	}
}

// 拷贝退路失败：同样返回错误。
func TestRootArgsCopyFailureIsAnError(t *testing.T) {
	overlayMu.Lock()
	old := overlayOK
	no := false
	overlayOK = &no // 强制走拷贝路径
	overlayMu.Unlock()
	t.Cleanup(func() { overlayMu.Lock(); overlayOK = old; overlayMu.Unlock() })

	root := t.TempDir()
	// lower 不存在：copyTree 必然失败。
	_, err := rootArgs(Config{Bwrap: "/usr/bin/bwrap", Root: root, Lower: filepath.Join(root, "missing")})
	if err == nil {
		t.Fatal("must fail instead of falling back to the host root")
	}
	if !strings.Contains(err.Error(), "image root") {
		t.Fatalf("error should name the cause: %v", err)
	}
}

// 没有镜像（Lower 为空）：宿主根只读，这是 image=host 的正常语义。
func TestRootArgsHostRootWhenNoImage(t *testing.T) {
	args, err := rootArgs(Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 3 || args[0] != "--ro-bind" {
		t.Fatalf("args = %v", args)
	}
}
