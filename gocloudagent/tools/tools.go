// 文件说明：tools.go — gocloudagent/tools 中 ShellOutputLimit、Workspace 等符号的实现
//
// 用法
//
//	ErrEnvelope(code, msg string) string  // toolErr 是工具失败结果（统一信封，与云端 skillFail 同形）
//	IsDangerousShell(command string) bool  // IsDangerousShell 报告一条命令是否需要确认
//
// 调用架构
//
//	[上游] gocloudagent/cli
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/tools/tools.go
//	         │ 依赖
//	         ▼
//	[下游] gosandbox/host
//	[同包] fsutil.go, search.go
//
// 主要符号
//
//	const ShellOutputLimit
//	type Workspace struct {…}
//	type ConfirmFunc func(question string) bool
//	ErrEnvelope(code, msg string) string
//	IsDangerousShell(command string) bool
//	(w *Workspace) LocalTools() ([]tool.BaseTool, error)
//	... 另有 26 个顶层符号

package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"

	"github.com/linkxzhou/SimpleBase/gosandbox/host"
)

// ToolLimits 是本机工具的统一上限（planv6.0 §5.3）。
const (
	// shellTimeout 是单条命令的默认超时。
	shellTimeout = 120 * time.Second
	// ShellOutputLimit 是回给模型的 stdout+stderr 字符上限。
	ShellOutputLimit = 32 << 10
	// fileReadLimit 是单次 read_file 的字节上限。
	fileReadLimit = 256 << 10
)

// Workspace 限定本机工具的路径边界与确认策略。
type Workspace struct {
	// Root 是允许访问的目录（默认启动目录）。所有路径工具限制在其子树内。
	Root string
	// Confirm 在破坏性命令执行前询问；nil 表示非交互（直接拒绝）。
	Confirm ConfirmFunc
	// Yolo 为真时跳过确认（显式承担风险，§5.3）。
	Yolo bool
	// Isolation 是本机 shell 的隔离策略（planv6.2）。零值（Enabled=false）保持裸执行。
	// Yolo 只跳过确认，不跳过隔离；关闭隔离必须显式设为 danger-full-access 或 Enabled=false。
	Isolation host.Policy
	// Record 记录一次工具调用（用于 REPL 展示与 jsonl 落盘）。
	Record func(name, args, result string, isErr bool)
}

// ConfirmFunc 返回是否批准；question 面向人阅读。
type ConfirmFunc func(question string) bool

// toolErr 是工具失败结果（统一信封，与云端 skillFail 同形）。
func ErrEnvelope(code, msg string) string {
	b, _ := json.Marshal(map[string]any{"ok": false, "error": map[string]any{"code": code, "message": msg}})
	return string(b)
}

// resolvePath 把模型给的路径解析到 workspace 内；越界返回错误。
//
// 相对路径相对 Root 解析；绝对路径必须落在 Root 子树内。
// 用 filepath.Rel 判定，避免 strings.HasPrefix 的 /root-evil 绕过。
func (w *Workspace) resolvePath(p string) (string, error) {
	if w == nil || w.Root == "" {
		return "", errors.New("workspace root is not set")
	}
	root, err := filepath.Abs(w.Root)
	if err != nil {
		return "", err
	}
	target := p
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	target = filepath.Clean(target)
	if target == root {
		return target, nil
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace: %s", p)
	}
	// shell 里被盖住的 deny_read 路径，文件工具也必须拒绝，
	// 否则模型可以用 read_file 读到 shell 已经看不到的 token。
	if host.DeniesRead(w.Isolation, target) {
		return "", fmt.Errorf("path is denied by shell isolation policy: %s", p)
	}
	return target, nil
}

// denyReadSkipper 返回 glob/grep 遍历时用的跳过函数；隔离未开启时为 nil。
func (w *Workspace) denyReadSkipper() pathSkipper {
	if w == nil || !w.Isolation.Isolates() || len(w.Isolation.DenyRead) == 0 {
		return nil
	}
	pol := w.Isolation
	return func(abs string) bool { return host.DeniesRead(pol, abs) }
}

// dangerousShellPatterns 是 shell 工具的破坏性命令规则表。
//
// 与 simplebase CLI 的 destructiveArgv 是两套规则：命令行空间完全不同，
// 不能复用（planv6.0 §5.3）。
var dangerousShellPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\brm\s+(-[a-z]*[rf][a-z]*\s+)+`),            // rm -rf / rm -fr
	regexp.MustCompile(`(?i)\bgit\s+push\b.*(--force|-f)\b`),            // git push --force
	regexp.MustCompile(`(?i)\bgit\s+(reset\s+--hard|clean\s+-[a-z]*f)`), // git reset --hard / clean -f
	regexp.MustCompile(`(?i)\b(curl|wget)\b[^|;]*\|\s*(ba)?sh\b`),       // curl ... | sh
	regexp.MustCompile(`(?i)\b(dd\s+if|mkfs|shutdown|reboot|halt)\b`),
	regexp.MustCompile(`(?i)\b(chmod|chown)\s+-R\b`),
	regexp.MustCompile(`(?i)(^|[;&|]\s*)sudo\b`),
	regexp.MustCompile(`(?i)>\s*/dev/(sd|disk|nvme)`),
	regexp.MustCompile(`(?i)\btruncate\s+-s\s*0\b`),
	regexp.MustCompile(`(?i)\b(kill|pkill|killall)\s+-9\b`),
}

// IsDangerousShell 报告一条命令是否需要确认。
func IsDangerousShell(command string) bool {
	for _, re := range dangerousShellPatterns {
		if re.MatchString(command) {
			return true
		}
	}
	return false
}

// shellArgs 是 shell 工具的参数。
type shellArgs struct {
	Command string `json:"command" jsonschema:"description=shell command to run with /bin/sh -c, in the workspace directory"`
	// Cwd 相对 workspace 解析；空表示 workspace 根。
	Cwd string `json:"cwd,omitempty" jsonschema:"description=working directory relative to the workspace root"`
}

func (w *Workspace) shellTool() (tool.BaseTool, error) {
	return utils.InferTool("shell",
		"Run a shell command on the local machine in the workspace directory. Destructive commands require user confirmation. Prefer read-only inspection commands. The command environment has SIMPLEBASE_*, GOCLOUDAGENT_* and credential-like variables removed, so a config_missing error from the simplebase CLI means its config file is not set up, not that the command is wrong; do not try to read or print credentials to fix it.",
		func(ctx context.Context, in shellArgs) (string, error) {
			if strings.TrimSpace(in.Command) == "" {
				return ErrEnvelope("usage", "command is required"), nil
			}
			cwd := w.Root
			if in.Cwd != "" {
				resolved, err := w.resolvePath(in.Cwd)
				if err != nil {
					return ErrEnvelope("path_not_allowed", err.Error()), nil
				}
				cwd = resolved
			}
			if IsDangerousShell(in.Command) {
				if !w.approve("将执行破坏性命令：" + in.Command) {
					return ErrEnvelope("confirmation_denied", "用户未批准该命令"), nil
				}
			}
			return w.runShell(ctx, in.Command, cwd), nil
		})
}

// runShell 在确认通过之后执行命令：隔离开启时经 host.Run，否则保持原来的裸执行。
func (w *Workspace) runShell(ctx context.Context, command, cwd string) string {
	env := sanitizedEnv(os.Environ())
	timeout := shellTimeout
	pol := w.Isolation
	if pol.Isolates() {
		if pol.Workspace == "" {
			pol.Workspace = w.Root
		}
		if pol.Timeout <= 0 {
			pol.Timeout = shellTimeout
		}
		timeout = pol.Timeout
		res, err := host.Run(ctx, pol, host.Command{Argv: []string{"/bin/sh", "-c", command}, Dir: cwd, Env: env})
		switch {
		case err == nil:
			return shellResult(res, timeout)
		case !errors.Is(err, host.ErrNotIsolated):
			// 隔离开着却没跑成：一律 fail-closed，命令没有被执行，不退回裸执行。
			code := "isolation_unavailable"
			if errors.Is(err, context.Canceled) {
				code = "canceled"
			}
			return ErrEnvelope(code, truncateText(err.Error(), 300))
		}
		// ErrNotIsolated：落到下面的裸执行。
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "/bin/sh", "-c", command)
	cmd.Dir = cwd
	// 不注入任何 SimpleBase 凭据到子进程（与云端「不向沙盒注入凭据」同原则）。
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return ErrEnvelope("timeout", fmt.Sprintf("命令超过 %s 未结束", timeout))
	}
	code := 0
	if err != nil {
		code = exitCode(err)
	}
	var cmdErr error
	if err != nil {
		cmdErr = err
	}
	return formatShell(truncateText(string(out), ShellOutputLimit), code, cmdErr, host.Limits{})
}

// shellResult 把隔离执行的结果转成工具返回值。
func shellResult(res host.Result, timeout time.Duration) string {
	if res.TimedOut {
		return ErrEnvelope("timeout", fmt.Sprintf("命令超过 %s 未结束", timeout))
	}
	var cmdErr error
	if res.ExitCode != 0 {
		cmdErr = fmt.Errorf("exit status %d", res.ExitCode)
	}
	return formatShell(truncateText(string(res.Output), ShellOutputLimit), res.ExitCode, cmdErr, res.Limits, res.Warnings...)
}

// formatShell 生成统一信封。配置了资源限额却没有生效时，如实写 limits=unenforced；
// 隔离层给出的安全提示（如强制 deny 表被关闭）原样放进 warnings，不吞掉。
func formatShell(text string, code int, cmdErr error, limits host.Limits, warnings ...string) string {
	body := map[string]any{"ok": cmdErr == nil, "output": text}
	if limits.Configured && !limits.Enforced {
		body["limits"] = "unenforced"
	}
	if len(warnings) > 0 {
		body["warnings"] = warnings
	}
	if cmdErr != nil {
		name := "command_failed"
		var exitErr *exec.ExitError
		if errors.As(cmdErr, &exitErr) || code > 0 {
			name = fmt.Sprintf("exit_%d", code)
		}
		body["exit_code"] = code
		body["error"] = map[string]any{"code": name, "message": truncateText(cmdErr.Error(), 300)}
	}
	b, _ := json.Marshal(body)
	return string(b)
}

// approve 询问用户；Yolo 或已配置的信任直接通过；无确认通道时拒绝（fail-closed）。
func (w *Workspace) approve(question string) bool {
	if w == nil {
		return false
	}
	if w.Yolo {
		return true
	}
	if w.Confirm == nil {
		return false
	}
	return w.Confirm(question)
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// sensitiveEnvPattern 匹配不应传给子进程的环境变量名。
var sensitiveEnvPattern = regexp.MustCompile(`(?i)(TOKEN|SECRET|PASSWORD|CREDENTIAL|API_KEY|ACCESS_KEY|SESSION)`)

// sanitizedEnv 过滤凭据类环境变量：CLI 跑的是用户自己的命令，
// 但不应把 SimpleBase 令牌顺手交给任意子进程。
func sanitizedEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "SIMPLEBASE_") || strings.HasPrefix(name, "GOCLOUDAGENT_") {
			continue
		}
		if sensitiveEnvPattern.MatchString(name) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// readFileArgs 是 read_file 的参数。
type readFileArgs struct {
	Path string `json:"path" jsonschema:"description=file path, relative to the workspace root or an absolute path inside it"`
	// Offset / Limit 支持大文件分段读。
	Offset int `json:"offset,omitempty" jsonschema:"description=1-based starting line number"`
	Limit  int `json:"limit,omitempty" jsonschema:"description=max lines to return, default 2000"`
}

func (w *Workspace) readFileTool() (tool.BaseTool, error) {
	return utils.InferTool("read_file", "Read a UTF-8 text file inside the workspace. Returns numbered lines.",
		func(_ context.Context, in readFileArgs) (string, error) {
			path, err := w.resolvePath(in.Path)
			if err != nil {
				return ErrEnvelope("path_not_allowed", err.Error()), nil
			}
			st, err := os.Stat(path)
			if err != nil {
				return ErrEnvelope("not_found", err.Error()), nil
			}
			if st.IsDir() {
				return ErrEnvelope("is_directory", "path is a directory; use glob or grep instead"), nil
			}
			if st.Size() > fileReadLimit {
				return ErrEnvelope("too_large", fmt.Sprintf("file is %d bytes, limit is %d; use offset/limit", st.Size(), fileReadLimit)), nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return ErrEnvelope("read_failed", err.Error()), nil
			}
			lines := strings.Split(string(b), "\n")
			start := 0
			if in.Offset > 1 {
				start = in.Offset - 1
			}
			if start > len(lines) {
				start = len(lines)
			}
			limit := in.Limit
			if limit <= 0 {
				limit = 2000
			}
			end := start + limit
			if end > len(lines) {
				end = len(lines)
			}
			var sb strings.Builder
			for i := start; i < end; i++ {
				fmt.Fprintf(&sb, "%d:%s\n", i+1, lines[i])
			}
			payload := map[string]any{"ok": true, "content": sb.String(), "total_lines": len(lines), "truncated": end < len(lines)}
			out, _ := json.Marshal(payload)
			return string(out), nil
		})
}

// writeFileArgs 是 write_file 的参数。
type writeFileArgs struct {
	Path    string `json:"path" jsonschema:"description=file path inside the workspace"`
	Content string `json:"content" jsonschema:"description=full file content to write"`
}

func (w *Workspace) writeFileTool() (tool.BaseTool, error) {
	return utils.InferTool("write_file", "Create or overwrite a file inside the workspace. Parent directories are created. Overwriting an existing file requires confirmation.",
		func(_ context.Context, in writeFileArgs) (string, error) {
			path, err := w.resolvePath(in.Path)
			if err != nil {
				return ErrEnvelope("path_not_allowed", err.Error()), nil
			}
			if _, err := os.Stat(path); err == nil {
				if !w.approve("将覆盖已有文件：" + in.Path) {
					return ErrEnvelope("confirmation_denied", "用户未批准覆盖"), nil
				}
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return ErrEnvelope("mkdir_failed", err.Error()), nil
			}
			if err := os.WriteFile(path, []byte(in.Content), 0o644); err != nil {
				return ErrEnvelope("write_failed", err.Error()), nil
			}
			b, _ := json.Marshal(map[string]any{"ok": true, "path": in.Path, "bytes": len(in.Content)})
			return string(b), nil
		})
}

// editFileArgs 是 edit_file 的参数（精确替换，不引入 diff 格式）。
type editFileArgs struct {
	Path    string `json:"path" jsonschema:"description=file path inside the workspace"`
	OldText string `json:"old_text" jsonschema:"description=exact text to replace; must be unique in the file"`
	NewText string `json:"new_text" jsonschema:"description=replacement text"`
}

func (w *Workspace) editFileTool() (tool.BaseTool, error) {
	return utils.InferTool("edit_file", "Replace an exact, unique snippet in a file inside the workspace.",
		func(_ context.Context, in editFileArgs) (string, error) {
			path, err := w.resolvePath(in.Path)
			if err != nil {
				return ErrEnvelope("path_not_allowed", err.Error()), nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return ErrEnvelope("read_failed", err.Error()), nil
			}
			content := string(b)
			n := strings.Count(content, in.OldText)
			if n == 0 {
				return ErrEnvelope("not_found", "old_text not found"), nil
			}
			if n > 1 {
				return ErrEnvelope("ambiguous", fmt.Sprintf("old_text appears %d times; include more context", n)), nil
			}
			if !w.approve("将修改文件：" + in.Path) {
				return ErrEnvelope("confirmation_denied", "用户未批准修改"), nil
			}
			if err := os.WriteFile(path, []byte(strings.Replace(content, in.OldText, in.NewText, 1)), 0o644); err != nil {
				return ErrEnvelope("write_failed", err.Error()), nil
			}
			b2, _ := json.Marshal(map[string]any{"ok": true, "path": in.Path})
			return string(b2), nil
		})
}

// globArgs 是 glob 的参数。
type globArgs struct {
	Pattern string `json:"pattern" jsonschema:"description=glob pattern, e.g. **/*.go or internal/*.md"`
}

func (w *Workspace) globTool() (tool.BaseTool, error) {
	return utils.InferTool("glob", "List files inside the workspace matching a glob pattern (** and * supported).",
		func(_ context.Context, in globArgs) (string, error) {
			if strings.TrimSpace(in.Pattern) == "" {
				return ErrEnvelope("usage", "pattern is required"), nil
			}
			// 拒绝绝对路径或带 .. 的模式，防止越界遍历。
			if filepath.IsAbs(in.Pattern) || strings.Contains(in.Pattern, "..") {
				return ErrEnvelope("path_not_allowed", "pattern must be relative and must not contain .."), nil
			}
			matches, err := globWalkSkip(w.Root, in.Pattern, w.denyReadSkipper())
			if err != nil {
				return ErrEnvelope("glob_failed", err.Error()), nil
			}
			const maxMatches = 500
			truncated := len(matches) > maxMatches
			if truncated {
				matches = matches[:maxMatches]
			}
			b, _ := json.Marshal(map[string]any{"ok": true, "matches": matches, "truncated": truncated})
			return string(b), nil
		})
}

// grepArgs 是 grep 的参数。
type grepArgs struct {
	Pattern string `json:"pattern" jsonschema:"description=RE2 regular expression to search for"`
	Glob    string `json:"glob,omitempty" jsonschema:"description=only search files matching this glob, e.g. **/*.go"`
	MaxHits int    `json:"max_hits,omitempty" jsonschema:"description=max matches to return, default 100"`
}

func (w *Workspace) grepTool() (tool.BaseTool, error) {
	return utils.InferTool("grep", "Search file contents inside the workspace with a regular expression. Returns file:line hits.",
		func(_ context.Context, in grepArgs) (string, error) {
			if strings.TrimSpace(in.Pattern) == "" {
				return ErrEnvelope("usage", "pattern is required"), nil
			}
			re, err := regexp.Compile(in.Pattern)
			if err != nil {
				return ErrEnvelope("bad_pattern", err.Error()), nil
			}
			limit := in.MaxHits
			if limit <= 0 {
				limit = 100
			}
			hits, truncated, err := grepWalkSkip(w.Root, re, in.Glob, limit, w.denyReadSkipper())
			if err != nil {
				return ErrEnvelope("grep_failed", err.Error()), nil
			}
			b, _ := json.Marshal(map[string]any{"ok": true, "hits": hits, "truncated": truncated})
			return string(b), nil
		})
}

// LocalTools 返回全部本机工具（不含 web 搜索，那个由 search.go 提供）。
func (w *Workspace) LocalTools() ([]tool.BaseTool, error) {
	builders := []func() (tool.BaseTool, error){
		w.shellTool, w.readFileTool, w.writeFileTool, w.editFileTool, w.globTool, w.grepTool,
	}
	out := make([]tool.BaseTool, 0, len(builders))
	for _, b := range builders {
		t, err := b()
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func truncateText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n…(truncated)"
}

var _ = schema.AssistantMessage
