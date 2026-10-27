// 文件说明：seatbelt.go — gosandbox/host 中 SeatbeltExecutable、SeatbeltProfile 等符号的实现
//
// 用法
//
//	SeatbeltProfile(p Policy) (profile string, params []string, err error)  // SeatbeltProfile 生成 SBPL 与 -D 参数
//	SeatbeltArgs(p Policy, argv []string) ([]string, error)  // SeatbeltArgs 返回 sandbox-exec 的参数：-p <profile> -D k=v ... -- argv...
//	同包调用：SeatbeltExecutable → run_darwin.go, start_darwin.go；SeatbeltArgs() → run_darwin.go, start_darwin.go；canonical() → glob.go
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/seatbelt.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] bwrap_args.go, cgroup.go, glob.go, mandatory.go, policy.go 等 13 个
//	[关键引用] SeatbeltExecutable ← run_darwin.go, start_darwin.go
//
// 主要符号
//
//	const SeatbeltExecutable
//	SeatbeltProfile(p Policy) (profile string, params []string, err error)
//	SeatbeltArgs(p Policy, argv []string) ([]string, error)
//	... 另有 3 个顶层符号

package host

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SeatbeltExecutable 只认系统自带路径，防止 PATH 上的同名假二进制。
// sandbox-exec 的 man page 标注为 deprecated，但 Codex 与 sandbox-runtime 仍在使用。
const SeatbeltExecutable = "/usr/bin/sandbox-exec"

// seatbeltBase 以 (deny default) 开头，只放行执行命令所需的最小集合。
const seatbeltBase = `(version 1)
(deny default)
(allow process-exec)
(allow process-fork)
(allow signal (target same-sandbox))
(allow sysctl-read)
(allow mach-lookup)
(allow ipc-posix-shm)
(allow pseudo-tty)
(allow file-read*)
(allow file-write* (literal "/dev/null") (literal "/dev/tty") (literal "/dev/dtracehelper"))
(allow file-ioctl (literal "/dev/null") (literal "/dev/tty"))
; 交互终端：PTY 从端是 /dev/ttysN，stty / 作业控制需要对它 ioctl。
(allow file-ioctl (literal "/dev/ptmx") (regex #"^/dev/ttys[0-9]+$"))
(allow file-read* file-write* (literal "/dev/ptmx") (regex #"^/dev/ttys[0-9]+$"))
`

// SeatbeltProfile 生成 SBPL 与 -D 参数。路径一律经 -D 传入，不拼进策略文本，
// 避免路径里的引号或括号改变策略语义。返回的 params 形如 {"W0=/a","D0=/b"}。
//
// 网络恒为拒绝：策略里没有 network 规则，(deny default) 即拒绝。
func SeatbeltProfile(p Policy) (profile string, params []string, err error) {
	return seatbeltProfile(p, userHome(), nil)
}

// seatbeltProfile 是 SeatbeltProfile 的可注入版本：home 与 exists 供测试使用。
//
// 只读路径（用户配置的 ReadOnlySubpaths 加强制 deny 表）只对生成时已存在的路径出规则，
// 与 Linux 一致：对不存在的 .git/hooks 加 deny 会让沙盒内 git init 直接失败（D7）。
func seatbeltProfile(p Policy, home string, exists func(string) bool) (profile string, params []string, err error) {
	if !p.Isolates() {
		return "", nil, ErrNotIsolated
	}
	if err := Validate(p); err != nil {
		return "", nil, err
	}
	var b strings.Builder
	b.WriteString(seatbeltBase)

	var writable []string
	if p.EffectiveMode() == ModeWorkspaceWrite {
		writable = append(writable, filepath.Clean(p.Workspace))
	}
	for _, r := range p.WritableRoots {
		writable = append(writable, filepath.Clean(r))
	}
	// /tmp 与 TMPDIR 的 tmp 在 macOS 上是 /private/tmp、/private/var/folders；
	// 编译器需要写临时文件，因此放开这两处。
	writable = append(writable, "/private/tmp", "/private/var/folders")
	for i, w := range writable {
		fmt.Fprintf(&b, "(allow file-write* (subpath (param \"W%d\")))\n", i)
		params = append(params, fmt.Sprintf("W%d=%s", i, canonical(w)))
	}

	// 后写的 deny 覆盖前面的 allow：只读路径、deny_read。
	if p.EffectiveMode() == ModeWorkspaceWrite {
		for n, abs := range readOnlyPaths(p, home, exists) {
			fmt.Fprintf(&b, "(deny file-write* (subpath (param \"R%d\")))\n", n)
			params = append(params, fmt.Sprintf("R%d=%s", n, canonical(abs)))
		}
	}
	for i, d := range p.DenyRead {
		if isGlobEntry(d) {
			path, _ := splitDenyEntry(d)
			expr, err := denyGlobRegex(path, canonical)
			if err != nil {
				return "", nil, err
			}
			fmt.Fprintf(&b, "(deny file-read* (regex (param \"D%d\")))\n", i)
			params = append(params, fmt.Sprintf("D%d=%s", i, expr))
			continue
		}
		path, _ := splitDenyEntry(d)
		fmt.Fprintf(&b, "(deny file-read* (subpath (param \"D%d\")))\n", i)
		params = append(params, fmt.Sprintf("D%d=%s", i, canonical(path)))
	}
	return b.String(), params, nil
}

// canonical 把路径解析成真实路径（macOS 的 /tmp、/var 是符号链接，
// Seatbelt 按真实路径匹配）；路径不存在时保留原值。
func canonical(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return filepath.Clean(path)
}

// SeatbeltArgs 返回 sandbox-exec 的参数：-p <profile> -D k=v ... -- argv...
func SeatbeltArgs(p Policy, argv []string) ([]string, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("host: empty argv")
	}
	profile, params, err := SeatbeltProfile(p)
	if err != nil {
		return nil, err
	}
	args := []string{"-p", profile}
	for _, kv := range params {
		args = append(args, "-D", kv)
	}
	args = append(args, "--")
	return append(args, argv...), nil
}
