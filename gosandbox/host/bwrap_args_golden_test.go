// 文件说明：bwrap_args_golden_test.go — gosandbox/host 的 BwrapArgs 完整 argv 快照测试
//
// 用法
//
//	go test ./host -run TestBwrapArgsGolden
//
// 调用架构
//
//	[上游] 重构前的参数顺序特征测试
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/bwrap_args_golden_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] bwrap_args.go, bwrap_args_extra_test.go, policy.go, policy_test.go
//
// 主要符号
//
//	TestBwrapArgsGolden(t *testing.T)

package host

import (
	"slices"
	"testing"
)

func TestBwrapArgsGolden(t *testing.T) {
	workspace := basePolicy()
	workspace.ReadOnlySubpaths = []string{".protected"}
	workspace.WritableRoots = []string{"/data/out"}
	workspace.DenyRead = []string{"/home/u/.ssh", "/home/u/token"}
	workspace.ShmBytes = 64 << 20
	workspace.TmpBytes = 32 << 20

	readOnly := basePolicy()
	readOnly.Mode = ModeReadOnly

	writableTmp := basePolicy()
	writableTmp.Workspace = "/tmp"
	writableTmp.ShmBytes = 64 << 20
	writableTmp.TmpBytes = 32 << 20

	cases := []struct {
		name string
		in   BwrapInput
		want []string
	}{
		{
			name: "read only",
			in: BwrapInput{
				Policy: readOnly, Helper: "/opt/gosandbox-inner", Argv: []string{"/bin/true"},
				Home: "/home/u", Exists: fakeExists(),
			},
			want: []string{
				"--new-session", "--die-with-parent", "--ro-bind", "/", "/",
				"--tmpfs", "/tmp",
				"--dev", "/dev", "--proc", "/proc",
				"--unshare-user", "--unshare-pid", "--unshare-ipc", "--unshare-net",
				"--cap-drop", "ALL", "--dir", "/tmp/gosandbox",
				"--perms", "0400", "--file", "3", "/tmp/gosandbox/policy.json",
				"--", "/opt/gosandbox-inner", "--policy", "/tmp/gosandbox/policy.json",
				"--", "/bin/true",
			},
		},
		{
			name: "workspace write with protections",
			in: BwrapInput{
				Policy: workspace, Dir: "/work/ws/sub", Helper: "/opt/gosandbox-inner",
				Argv: []string{"/bin/sh", "-c", "echo hi"}, PolicyFD: 7,
				DisableUserNS: true, SizesSupported: true, CTTY: true, Home: "/home/u",
				Exists: fakeExists("/work/ws/.protected", "/work/ws/.git/hooks", "/home/u/.ssh", "/home/u/token", "/proc/kcore", "/proc/sys"),
				IsDir:  func(path string) bool { return path == "/home/u/.ssh" },
			},
			want: []string{
				"--new-session", "--die-with-parent", "--ro-bind", "/", "/",
				"--size", "33554432", "--tmpfs", "/tmp",
				"--bind", "/work/ws", "/work/ws", "--bind", "/data/out", "/data/out",
				"--ro-bind", "/work/ws/.protected", "/work/ws/.protected",
				"--ro-bind", "/work/ws/.git/hooks", "/work/ws/.git/hooks",
				"--tmpfs", "/home/u/.ssh", "--ro-bind", "/dev/null", "/home/u/token",
				"--dev", "/dev", "--proc", "/proc",
				"--size", "67108864", "--tmpfs", "/dev/shm",
				"--ro-bind", "/dev/null", "/proc/kcore", "--ro-bind", "/proc/sys", "/proc/sys",
				"--unshare-user", "--unshare-pid", "--unshare-ipc", "--unshare-net", "--disable-userns",
				"--cap-drop", "ALL", "--dir", "/tmp/gosandbox",
				"--perms", "0400", "--file", "7", "/tmp/gosandbox/policy.json",
				"--chdir", "/work/ws/sub",
				"--", "/opt/gosandbox-inner", "--policy", "/tmp/gosandbox/policy.json", "--ctty",
				"--", "/bin/sh", "-c", "echo hi",
			},
		},
		{
			name: "writable tmp and old bwrap",
			in: BwrapInput{
				Policy: writableTmp, Helper: "/opt/gosandbox-inner", Argv: []string{"/bin/true"},
				Home: "/home/u", Exists: fakeExists(),
			},
			want: []string{
				"--new-session", "--die-with-parent", "--ro-bind", "/", "/",
				"--bind", "/tmp", "/tmp",
				"--dev", "/dev", "--proc", "/proc", "--tmpfs", "/dev/shm",
				"--unshare-user", "--unshare-pid", "--unshare-ipc", "--unshare-net",
				"--cap-drop", "ALL", "--dir", "/tmp/gosandbox",
				"--perms", "0400", "--file", "3", "/tmp/gosandbox/policy.json",
				"--", "/opt/gosandbox-inner", "--policy", "/tmp/gosandbox/policy.json",
				"--", "/bin/true",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BwrapArgs(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("argv:\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}
