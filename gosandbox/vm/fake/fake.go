// 文件说明：fake.go — gosandbox/vm/fake 中 Driver、New 等符号的实现
//
// 用法
//
//	New() *Driver
//	(d *Driver) EnsureCount() int  // EnsureCount 返回 Ensure（含隐式接回）被调用的次数
//	(d *Driver) VMCount() int  // VMCount 返回当前存在的 VM 数（单测断言 run 必删）
//	同包调用：Driver → interactive.go；fakeVM → interactive.go；ensureLocked() → interactive.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/fake/fake.go
//	         │ 依赖
//	         ▼
//	[下游] gosandbox/vm
//	[同包] interactive.go
//	[关键引用] Driver ← interactive.go
//
// 主要符号
//
//	type Driver struct {…}
//	New() *Driver
//	(d *Driver) EnsureCount() int
//	(d *Driver) VMCount() int
//	(d *Driver) Ensure(_ context.Context, t sb.Target) error
//	(d *Driver) Exec(ctx context.Context, t sb.Target, req sb.ExecSpec) (sb.ExecResult ...
//	... 另有 18 个顶层符号

package fake

import (
	"context"
	"fmt"
	sb "github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// fakeEpoch 是 fake 驱动报告的固定修改时间，保证测试结果可复现。
var fakeEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// Driver 是内存执行后端（仅 e2e / 单测；装配层要求 SIMPLEBASE_SANDBOX_ALLOW_FAKE=1）。
//
// 它不启动任何进程：Shell 只解释一个极小子集，足以让 e2e 覆盖完整链路——
//
//	echo [args...] [> file | >> file]、cat file...、ls [dir]、pwd、true、false、
//	exit N、sleep S、rm [-f] file，用 ";" 或 "&&" 串联。
//
// Exec(cmd, args) 等价于把 cmd 与 args 以空格拼接后交给同一解释器。
type Driver struct {
	mu  sync.Mutex
	vms map[string]*fakeVM
	// Calls 记录 Ensure 次数（单测断言懒创建）。
	ensures int
}

type fakeVM struct {
	running bool
	workdir string
	files   map[string][]byte
}

// New 构造空的内存驱动。
func New() *Driver { return &Driver{vms: map[string]*fakeVM{}} }

// EnsureCount 返回 Ensure（含隐式接回）被调用的次数。
func (d *Driver) EnsureCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.ensures
}

// VMCount 返回当前存在的 VM 数（单测断言 run 必删）。
func (d *Driver) VMCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.vms)
}

func (d *Driver) ensureLocked(t sb.Target) *fakeVM {
	d.ensures++
	vm, ok := d.vms[t.Name]
	if !ok {
		wd := t.Spec.Workdir
		if wd == "" {
			wd = "/workspace"
		}
		vm = &fakeVM{workdir: wd, files: map[string][]byte{}}
		d.vms[t.Name] = vm
	}
	vm.running = true
	return vm
}

// Ensure 实现 Driver。
func (d *Driver) Ensure(_ context.Context, t sb.Target) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ensureLocked(t)
	return nil
}

// Exec 实现 Driver。
func (d *Driver) Exec(ctx context.Context, t sb.Target, req sb.ExecSpec) (sb.ExecResult, error) {
	line := req.Shell
	if line == "" {
		parts := append([]string{req.Cmd}, req.Args...)
		line = strings.Join(parts, " ")
	}
	d.mu.Lock()
	vm := d.ensureLocked(t)
	d.mu.Unlock()
	return d.run(ctx, vm, line, req)
}

func (d *Driver) run(ctx context.Context, vm *fakeVM, line string, req sb.ExecSpec) (sb.ExecResult, error) {
	var out, errb strings.Builder
	cwd := req.Cwd
	if cwd == "" {
		cwd = vm.workdir
	}
	deadline := time.Now().Add(req.Timeout)
	code := 0
	for _, seg := range splitCommands(line) {
		if seg.op == "&&" && code != 0 {
			break
		}
		fields := strings.Fields(seg.cmd)
		if len(fields) == 0 {
			continue
		}
		code = 0
		switch fields[0] {
		case "echo":
			text, target, appendMode := parseRedirect(fields[1:])
			text = expandEnv(text, req.Env)
			if target == "" {
				out.WriteString(text + "\n")
				continue
			}
			p := resolve(cwd, target)
			d.mu.Lock()
			if appendMode {
				vm.files[p] = append(vm.files[p], []byte(text+"\n")...)
			} else {
				vm.files[p] = []byte(text + "\n")
			}
			d.mu.Unlock()
		case "cat":
			for _, f := range fields[1:] {
				d.mu.Lock()
				b, ok := vm.files[resolve(cwd, f)]
				d.mu.Unlock()
				if !ok {
					fmt.Fprintf(&errb, "cat: %s: No such file or directory\n", f)
					code = 1
					continue
				}
				out.Write(b)
			}
		case "ls":
			dir := cwd
			if len(fields) > 1 {
				dir = resolve(cwd, fields[1])
			}
			d.mu.Lock()
			names := listNames(vm.files, dir)
			d.mu.Unlock()
			for _, n := range names {
				out.WriteString(n + "\n")
			}
		case "pwd":
			out.WriteString(cwd + "\n")
		case "true":
		case "false":
			code = 1
		case "rm":
			for _, f := range fields[1:] {
				if strings.HasPrefix(f, "-") {
					continue
				}
				d.mu.Lock()
				delete(vm.files, resolve(cwd, f))
				d.mu.Unlock()
			}
		case "exit":
			if len(fields) > 1 {
				code, _ = strconv.Atoi(fields[1])
			}
			return sb.ExecResult{Stdout: []byte(out.String()), Stderr: []byte(errb.String()), ExitCode: code}, nil
		case "sleep":
			secs := 0.0
			if len(fields) > 1 {
				secs, _ = strconv.ParseFloat(fields[1], 64)
			}
			wake := time.Now().Add(time.Duration(secs * float64(time.Second)))
			if req.Timeout > 0 && wake.After(deadline) {
				wait := time.Until(deadline)
				select {
				case <-time.After(wait):
				case <-ctx.Done():
					return sb.ExecResult{}, ctx.Err()
				}
				return sb.ExecResult{Stdout: []byte(out.String()), Stderr: []byte(errb.String()), ExitCode: -1, TimedOut: true}, nil
			}
			select {
			case <-time.After(time.Until(wake)):
			case <-ctx.Done():
				return sb.ExecResult{}, ctx.Err()
			}
		default:
			fmt.Fprintf(&errb, "sh: %s: not found\n", fields[0])
			code = 127
		}
	}
	return sb.ExecResult{Stdout: []byte(out.String()), Stderr: []byte(errb.String()), ExitCode: code}, nil
}

// ReadFile 实现 Driver。
func (d *Driver) ReadFile(_ context.Context, t sb.Target, path string) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm := d.ensureLocked(t)
	b, ok := vm.files[path]
	if !ok {
		return nil, sb.ErrFileNotFound
	}
	return append([]byte(nil), b...), nil
}

// WriteFile 实现 Driver。
func (d *Driver) WriteFile(_ context.Context, t sb.Target, path string, data []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm := d.ensureLocked(t)
	vm.files[path] = append([]byte(nil), data...)
	return nil
}

// RemoveFile 实现 Driver。
func (d *Driver) RemoveFile(_ context.Context, t sb.Target, path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm := d.ensureLocked(t)
	delete(vm.files, path)
	return nil
}

// ListDir 实现 Driver：单层列目录，子目录由文件路径推导。
func (d *Driver) ListDir(_ context.Context, t sb.Target, dir string) ([]sb.FileEntry, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm := d.ensureLocked(t)
	seen := map[string]sb.FileEntry{}
	prefix := strings.TrimSuffix(dir, "/") + "/"
	for p, b := range vm.files {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		rest := strings.TrimPrefix(p, prefix)
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			name := rest[:i]
			seen[name] = sb.FileEntry{Name: name, Path: prefix + name, Kind: "directory", Mode: 0o755, ModTime: fakeEpoch}
			continue
		}
		seen[rest] = sb.FileEntry{Name: rest, Path: p, Kind: "file", Size: int64(len(b)), Mode: 0o644, ModTime: fakeEpoch}
	}
	out := make([]sb.FileEntry, 0, len(seen))
	for _, e := range seen {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Status 实现 Driver。
func (d *Driver) Status(_ context.Context, name string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm, ok := d.vms[name]
	if !ok {
		return sb.VMAbsent, nil
	}
	if vm.running {
		return sb.VMRunning, nil
	}
	return sb.VMStopped, nil
}

// Stop 实现 Driver：文件保留。
func (d *Driver) Stop(_ context.Context, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if vm, ok := d.vms[name]; ok {
		vm.running = false
	}
	return nil
}

// Remove 实现 Driver。
func (d *Driver) Remove(_ context.Context, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.vms, name)
	return nil
}

type cmdSeg struct {
	op  string // 与前一段的连接符：""、";"、"&&"
	cmd string
}

func splitCommands(line string) []cmdSeg {
	var segs []cmdSeg
	op := ""
	for {
		iSemi := strings.Index(line, ";")
		iAnd := strings.Index(line, "&&")
		if iSemi < 0 && iAnd < 0 {
			segs = append(segs, cmdSeg{op: op, cmd: line})
			return segs
		}
		if iAnd >= 0 && (iSemi < 0 || iAnd < iSemi) {
			segs = append(segs, cmdSeg{op: op, cmd: line[:iAnd]})
			line, op = line[iAnd+2:], "&&"
			continue
		}
		segs = append(segs, cmdSeg{op: op, cmd: line[:iSemi]})
		line, op = line[iSemi+1:], ";"
	}
}

func parseRedirect(args []string) (text, target string, appendMode bool) {
	var words []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if (a == ">" || a == ">>") && i+1 < len(args) {
			target, appendMode = args[i+1], a == ">>"
			i++
			continue
		}
		words = append(words, strings.Trim(a, `'"`))
	}
	return strings.Join(words, " "), target, appendMode
}

func expandEnv(s string, env map[string]string) string {
	for k, v := range env {
		s = strings.ReplaceAll(s, "$"+k, v)
	}
	return s
}

func resolve(cwd, p string) string {
	if strings.HasPrefix(p, "/") {
		return p
	}
	return strings.TrimSuffix(cwd, "/") + "/" + p
}

func listNames(files map[string][]byte, dir string) []string {
	prefix := strings.TrimSuffix(dir, "/") + "/"
	set := map[string]struct{}{}
	for p := range files {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		rest := strings.TrimPrefix(p, prefix)
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			rest = rest[:i]
		}
		set[rest] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

var _ sb.Driver = (*Driver)(nil)
