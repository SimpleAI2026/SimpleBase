package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/gosandbox/host"
)

// brokenPolicy 在任何平台都无法建立隔离（工作区不是绝对路径，Validate 即拒绝），
// 用来验证 fail-closed，而不依赖本机是否装了 bwrap。
func brokenPolicy() host.Policy {
	return host.Policy{Enabled: true, Mode: host.ModeWorkspaceWrite, Workspace: "relative/ws"}
}

// 隔离开着但不可用：返回 isolation_unavailable，命令没有被执行。
func TestShellIsolationFailsClosed(t *testing.T) {
	ws, root := testWorkspace(t)
	ws.Isolation = brokenPolicy()
	tools, err := ws.LocalTools()
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "ran")
	out := invoke(t, findTool(t, tools, "shell"), `{"command":"touch ran"}`)
	if !strings.Contains(out, "isolation_unavailable") {
		t.Fatalf("want isolation_unavailable, got: %s", out)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("command ran although isolation was unavailable")
	}
}

// Yolo 只跳过确认，不跳过隔离：破坏性命令在 Yolo 下也不会因为隔离不可用而裸跑。
func TestShellYoloStillIsolated(t *testing.T) {
	ws, root := testWorkspace(t)
	ws.Yolo = true
	ws.Isolation = brokenPolicy()
	sentinel := filepath.Join(root, "sentinel")
	if err := os.WriteFile(sentinel, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	tools, err := ws.LocalTools()
	if err != nil {
		t.Fatal(err)
	}
	out := invoke(t, findTool(t, tools, "shell"), `{"command":"rm -rf sentinel"}`)
	if strings.Contains(out, "confirmation_denied") {
		t.Fatalf("yolo must skip confirmation: %s", out)
	}
	if !strings.Contains(out, "isolation_unavailable") {
		t.Fatalf("yolo must not skip isolation: %s", out)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("sentinel must survive: %v", err)
	}
}

// danger-full-access 与未启用都走原来的裸执行。
func TestShellBypassModesRunBare(t *testing.T) {
	cases := map[string]host.Policy{
		"disabled": {},
		"danger":   {Enabled: true, Mode: host.ModeDangerFullAccess, Workspace: "relative/ws"},
	}
	for name, pol := range cases {
		t.Run(name, func(t *testing.T) {
			ws, _ := testWorkspace(t)
			ws.Isolation = pol
			tools, err := ws.LocalTools()
			if err != nil {
				t.Fatal(err)
			}
			out := invoke(t, findTool(t, tools, "shell"), `{"command":"echo hello-bare"}`)
			if !strings.Contains(out, "hello-bare") {
				t.Fatalf("bare run expected: %s", out)
			}
		})
	}
}

func denyWorkspace(t *testing.T) (*Workspace, string) {
	t.Helper()
	ws, root := testWorkspace(t)
	token := filepath.Join(root, "token.txt")
	if err := os.WriteFile(token, []byte("TOPSECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "open.txt"), []byte("TOPSECRET-open"), 0o600); err != nil {
		t.Fatal(err)
	}
	ws.Isolation = host.Policy{Enabled: true, Mode: host.ModeWorkspaceWrite, Workspace: root, DenyRead: []string{token}}
	return ws, token
}

// deny_read 在 shell 里被盖住，文件工具也必须拒绝。
func TestDenyReadFileTools(t *testing.T) {
	ws, token := denyWorkspace(t)
	tools, err := ws.LocalTools()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ tool, args string }{
		{"read_file", `{"path":"token.txt"}`},
		{"read_file", `{"path":"` + token + `"}`},
		{"write_file", `{"path":"token.txt","content":"x"}`},
		{"edit_file", `{"path":"token.txt","old_text":"a","new_text":"b"}`},
	} {
		out := invoke(t, findTool(t, tools, tc.tool), tc.args)
		if !strings.Contains(out, "path_not_allowed") || strings.Contains(out, "TOPSECRET") {
			t.Fatalf("%s must be denied: %s", tc.tool, out)
		}
	}
	if out := invoke(t, findTool(t, tools, "read_file"), `{"path":"open.txt"}`); !strings.Contains(out, "TOPSECRET-open") {
		t.Fatalf("non-denied file must stay readable: %s", out)
	}
}

// glob 不列出、grep 不读取 deny_read 路径。
func TestDenyReadGlobGrepSkip(t *testing.T) {
	ws, _ := denyWorkspace(t)
	tools, err := ws.LocalTools()
	if err != nil {
		t.Fatal(err)
	}
	out := invoke(t, findTool(t, tools, "glob"), `{"pattern":"*.txt"}`)
	if strings.Contains(out, "token.txt") || !strings.Contains(out, "open.txt") {
		t.Fatalf("glob must hide denied path only: %s", out)
	}
	out = invoke(t, findTool(t, tools, "grep"), `{"pattern":"TOPSECRET"}`)
	if strings.Contains(out, "token.txt") || !strings.Contains(out, "open.txt") {
		t.Fatalf("grep must skip denied path only: %s", out)
	}
}

// 隔离关闭时 deny_read 不生效（保持现状）。
func TestDenyReadIgnoredWhenDisabled(t *testing.T) {
	ws, _ := denyWorkspace(t)
	ws.Isolation.Enabled = false
	if _, err := ws.resolvePath("token.txt"); err != nil {
		t.Fatalf("disabled isolation must not deny: %v", err)
	}
}

// 隔离层的安全提示（强制 deny 表被关闭等）原样进入信封，不被吞掉；没有提示时不出现该字段。
func TestShellResultReportsWarnings(t *testing.T) {
	res := host.Result{Output: []byte("ok"), Warnings: []string{"force-deny table disabled: x"}}
	out := shellResult(res, 0)
	if !strings.Contains(out, `"warnings":["force-deny table disabled: x"]`) {
		t.Fatalf("warnings must be reported: %s", out)
	}
	if clean := shellResult(host.Result{Output: []byte("ok")}, 0); strings.Contains(clean, "warnings") {
		t.Fatalf("no warnings field expected when there are none: %s", clean)
	}
}
