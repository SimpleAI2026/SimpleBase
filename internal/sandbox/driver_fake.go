package sandbox

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FakeDriver 是内存执行后端（backend=fake，仅 dev_mode / e2e / 单测，§5）。
//
// 它不启动任何进程：Shell 只解释一个极小子集，足以让 e2e 覆盖完整链路——
//
//	echo [args...] [> file | >> file]、cat file...、ls [dir]、pwd、true、false、
//	exit N、sleep S、rm [-f] file，用 ";" 或 "&&" 串联。
//
// Exec(cmd, args) 等价于把 cmd 与 args 以空格拼接后交给同一解释器。
type FakeDriver struct {
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

// NewFakeDriver 构造空的内存驱动。
func NewFakeDriver() *FakeDriver { return &FakeDriver{vms: map[string]*fakeVM{}} }

// Kind 实现 Driver。
func (d *FakeDriver) Kind() string { return DriverFake }

// EnsureCount 返回 Ensure（含隐式接回）被调用的次数。
func (d *FakeDriver) EnsureCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.ensures
}

// VMCount 返回当前存在的 VM 数（单测断言 run 必删）。
func (d *FakeDriver) VMCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.vms)
}

func (d *FakeDriver) ensureLocked(t Target) *fakeVM {
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
func (d *FakeDriver) Ensure(_ context.Context, t Target) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ensureLocked(t)
	return nil
}

// Exec 实现 Driver。
func (d *FakeDriver) Exec(ctx context.Context, t Target, req ExecSpec) (ExecResult, error) {
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

func (d *FakeDriver) run(ctx context.Context, vm *fakeVM, line string, req ExecSpec) (ExecResult, error) {
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
			return ExecResult{Stdout: []byte(out.String()), Stderr: []byte(errb.String()), ExitCode: code}, nil
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
					return ExecResult{}, ctx.Err()
				}
				return ExecResult{Stdout: []byte(out.String()), Stderr: []byte(errb.String()), ExitCode: -1, TimedOut: true}, nil
			}
			select {
			case <-time.After(time.Until(wake)):
			case <-ctx.Done():
				return ExecResult{}, ctx.Err()
			}
		default:
			fmt.Fprintf(&errb, "sh: %s: not found\n", fields[0])
			code = 127
		}
	}
	return ExecResult{Stdout: []byte(out.String()), Stderr: []byte(errb.String()), ExitCode: code}, nil
}

// ReadFile 实现 Driver。
func (d *FakeDriver) ReadFile(_ context.Context, t Target, path string) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm := d.ensureLocked(t)
	b, ok := vm.files[path]
	if !ok {
		return nil, ErrFileNotFound
	}
	return append([]byte(nil), b...), nil
}

// WriteFile 实现 Driver。
func (d *FakeDriver) WriteFile(_ context.Context, t Target, path string, data []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm := d.ensureLocked(t)
	vm.files[path] = append([]byte(nil), data...)
	return nil
}

// RemoveFile 实现 Driver。
func (d *FakeDriver) RemoveFile(_ context.Context, t Target, path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm := d.ensureLocked(t)
	delete(vm.files, path)
	return nil
}

// ListDir 实现 Driver：单层列目录，子目录由文件路径推导。
func (d *FakeDriver) ListDir(_ context.Context, t Target, dir string) ([]FileEntry, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm := d.ensureLocked(t)
	seen := map[string]FileEntry{}
	prefix := strings.TrimSuffix(dir, "/") + "/"
	for p, b := range vm.files {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		rest := strings.TrimPrefix(p, prefix)
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			name := rest[:i]
			seen[name] = FileEntry{Name: name, Path: prefix + name, Kind: "directory"}
			continue
		}
		seen[rest] = FileEntry{Name: rest, Path: p, Kind: "file", Size: int64(len(b))}
	}
	out := make([]FileEntry, 0, len(seen))
	for _, e := range seen {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Status 实现 Driver。
func (d *FakeDriver) Status(_ context.Context, name string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	vm, ok := d.vms[name]
	if !ok {
		return VMAbsent, nil
	}
	if vm.running {
		return VMRunning, nil
	}
	return VMStopped, nil
}

// Stop 实现 Driver：文件保留。
func (d *FakeDriver) Stop(_ context.Context, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if vm, ok := d.vms[name]; ok {
		vm.running = false
	}
	return nil
}

// Remove 实现 Driver。
func (d *FakeDriver) Remove(_ context.Context, name string) error {
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
