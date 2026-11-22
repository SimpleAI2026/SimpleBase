package host

import (
	"testing"

	"github.com/linkxzhou/SimpleBase/gosandbox/host/spec"
)

// 显式配置 shm 时挂一个带 --size 的 tmpfs；未配置时不挂。
func TestBwrapArgsShmSize(t *testing.T) {
	p := basePolicy()
	if idx(mustArgs(t, BwrapInput{Policy: p, Exists: fakeExists()}), "--tmpfs", "/dev/shm") >= 0 {
		t.Fatal("shm must not be mounted without an explicit size")
	}
	p.ShmBytes = 1 << 30
	// bwrap 支持 --size 时带上大小。
	args := mustArgs(t, BwrapInput{Policy: p, Exists: fakeExists(), SizesSupported: true})
	i := idx(args, "--size", "1073741824", "--tmpfs", "/dev/shm")
	if i < 0 {
		t.Fatalf("missing sized /dev/shm tmpfs: %v", args)
	}
	// bwrap < 0.9（不认 --size）时退化为不限大小的 tmpfs，而不是让 bwrap 失败。
	args = mustArgs(t, BwrapInput{Policy: p, Exists: fakeExists()})
	if idx(args, "--size") >= 0 || idx(args, "--tmpfs", "/dev/shm") < 0 {
		t.Fatalf("unsupported --size must degrade to a plain tmpfs: %v", args)
	}
}

// masked / readonly 路径只在存在时挂载。
func TestBwrapArgsMaskedPaths(t *testing.T) {
	p := basePolicy()
	present := append(append([]string{}, MaskedPaths...), ReadonlyPaths...)
	args := mustArgs(t, BwrapInput{Policy: p, Exists: fakeExists(present...)})
	if idx(args, "--ro-bind", "/dev/null", "/proc/kcore") < 0 {
		t.Fatalf("missing masked /proc/kcore: %v", args)
	}
	if idx(args, "--ro-bind", "/proc/sys", "/proc/sys") < 0 {
		t.Fatalf("missing readonly /proc/sys: %v", args)
	}
}

// masked 路径必须排在其他绑定之后（后挂的覆盖先挂的），且在命名空间参数之前。
func TestBwrapArgsMaskedOrder(t *testing.T) {
	p := basePolicy()
	p.ShmBytes = 64 << 20
	args := mustArgs(t, BwrapInput{Policy: p, Exists: fakeExists("/proc/kcore", "/proc/sys"), SizesSupported: true})
	dev := idx(args, "--dev", "/dev")
	shm := idx(args, "--size", "67108864", "--tmpfs", "/dev/shm")
	kcore := idx(args, "--ro-bind", "/dev/null", "/proc/kcore")
	unshare := idx(args, "--unshare-user")
	if !(dev < shm && shm < kcore && kcore < unshare) {
		t.Fatalf("order dev(%d) shm(%d) kcore(%d) unshare(%d): %v", dev, shm, kcore, unshare, args)
	}
}

// 未知的 seccomp 档位按 host 处理（保守）；default 档含 Docker 默认清单。
//
// 过滤器本体在 host/inner 包里测；这里只保证档位名与「两个档位不同」不变。
func TestFilterForSelection(t *testing.T) {
	// 档位常量拼错会让 helper 静默落回 host 档，这里钉住取值。
	if spec.SeccompHost != "host" || spec.SeccompDefault != "default" {
		t.Fatalf("seccomp kinds changed: %q %q", spec.SeccompHost, spec.SeccompDefault)
	}
}
