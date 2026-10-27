// 文件说明：repl.go — gocloudagent/cli 中 Options、Run 等符号的实现
//
// 用法
//
//	Run(opts Options) int  // Run 是 CLI 的总入口，返回进程退出码
//	SetVersion(v string)  // SetVersion 允许入口设置版本号
//
// 调用架构
//
//	[上游] gocloudagent
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cli/repl.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent/agentcore, gocloudagent/tools
//	[同包] client.go, command.go, compaction.go, config.go, prompt.go 等 9 个
//
// 主要符号
//
//	type Options struct {…}
//	Run(opts Options) int
//	SetVersion(v string)
//	... 另有 19 个顶层符号

package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"github.com/linkxzhou/SimpleBase/gocloudagent/tools"
)

// Options 是一次 CLI 调用的完整配置（由 cmd/gocloudagent 组装）。
type Options struct {
	Config    Config
	Env       []string
	Cwd       string
	SessionID string // resume 目标；空表示新建
	Prompt    string
	Headless  bool
	JSON      bool
	Yolo      bool
	// FlagErr 是命令行解析阶段的错误（如非法的 --shell-isolation），Run 会按用法错误报告。
	FlagErr error
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	// Confirm 为 nil 时（非交互）破坏性操作直接拒绝（fail-closed）。
	Confirm tools.ConfirmFunc
	// MaxDuration 单次运行墙钟时限（O3-6），0 表示不限（测试时用）。
	MaxDuration time.Duration
}

// Exit codes 与 sbcli 保持同一套约定（0 成功 / 1 用法 / 2 API / 4 配置）。
const (
	exitOK     = 0
	exitUsage  = 1
	exitAPI    = 2
	exitConfig = 4
)

// Run 是 CLI 的总入口，返回进程退出码。
func Run(opts Options) int {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}
	if opts.Cwd == "" {
		opts.Cwd, _ = os.Getwd()
	}
	// 配置链路（planv6.0 §5.2）：~/.gocloudagent/config.json 是基线，调用方显式传入的
	// 字段覆盖它，最后叠环境变量（GOCLOUDAGENT_* 优先于 SIMPLEBASE_*）。
	// 之前这里只叠 env，导致 run/resume 完全不读配置文件——配置全写在文件里的用户
	// 每次都被判成「未配置」。
	fileCfg, err := LoadConfig(opts.Env)
	if err != nil {
		// 权限不合规时 fail-closed：不静默退回 env，否则同机其他用户能读到的 token
		// 会被当成「没配」，问题被掩盖。
		return fail(opts, exitConfig, "config_invalid", err.Error())
	}
	if opts.FlagErr != nil {
		return fail(opts, exitUsage, "usage", opts.FlagErr.Error())
	}
	cfg := MergeEnv(overlayConfig(fileCfg, opts.Config), opts.Env)
	if opts.Yolo {
		cfg.Yolo = true
	}
	if err := cfg.Validate(); err != nil {
		return fail(opts, exitConfig, "config_invalid", err.Error())
	}

	session, err := openOrCreateSession(opts, cfg)
	if err != nil {
		return fail(opts, exitConfig, "session_error", err.Error())
	}

	runtime, err := buildRunner(opts, cfg, session)
	if err != nil {
		return fail(opts, exitUsage, "setup_failed", err.Error())
	}

	// 交互模式：有 TTY 且未给 prompt。预算按「单轮」计，已合并的配置值经 opts 带进去。
	if !opts.Headless && strings.TrimSpace(opts.Prompt) == "" && isTTY(opts.Stdin) {
		opts.MaxDuration = runLimit(opts, cfg)
		return runREPL(opts, runtime, session)
	}

	prompt := strings.TrimSpace(opts.Prompt)
	if prompt == "" {
		// 非 TTY 且无 prompt：从 stdin 读一次（支持管道）。
		b, rerr := io.ReadAll(io.LimitReader(opts.Stdin, 1<<20))
		if rerr != nil {
			return fail(opts, exitUsage, "stdin_failed", rerr.Error())
		}
		prompt = strings.TrimSpace(string(b))
	}
	if prompt == "" {
		return fail(opts, exitUsage, "usage", "no prompt given: pass a prompt argument, -p \"...\", or pipe text on stdin")
	}
	// planv6.1 O3-6: 给 Run 加墙钟超时（防 layout-config-recreation 式黑洞）。
	// opts.MaxDuration 是调用方显式覆盖；否则取配置里的 max_run_seconds。
	baseCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ctx := baseCtx
	if limit := runLimit(opts, cfg); limit > 0 {
		var tcancel context.CancelFunc
		ctx, tcancel = context.WithTimeout(baseCtx, limit)
		defer tcancel()
	}
	result, err := runtime.Run(ctx, prompt, emitFunc(opts))
	if err != nil {
		// 只把本轮自己的墙钟截止报成 timeout。HTTP 客户端的 DeadlineExceeded
		// 也会冒泡上来；未配置 max_run_seconds 时那会被写成「run exceeded 0s」。
		if runDeadlineFired(ctx, err) {
			return fail(opts, exitAPI, "timeout", fmt.Sprintf("run exceeded %s", runLimit(opts, cfg)))
		}
		if errors.Is(err, context.Canceled) {
			return fail(opts, exitUsage, "canceled", "interrupted")
		}
		return fail(opts, exitAPI, "run_failed", err.Error())
	}
	if opts.JSON {
		usage := &CLIUsage{
			PromptTokens:     result.PromptTokens,
			CompletionTokens: result.CompletionTokens,
			TotalTokens:      result.PromptTokens + result.CompletionTokens,
			ReasoningTokens:  result.ReasoningTokens,
		}
		writeJSONEvent(opts, CLIEvent{
			Type: "end", Content: result.Content, Message: result.Reason,
			Usage: usage, Model: result.Model, DurationMS: result.DurationMS,
		})
	} else {
		fmt.Fprintln(opts.Stdout, result.Content)
	}
	return exitOK
}

// runLimit 返回单次 run 的墙钟上限；0 表示不限。
// opts.MaxDuration 是调用方显式覆盖（测试用），否则取配置的 max_run_seconds。
func runLimit(opts Options, cfg Config) time.Duration {
	if opts.MaxDuration > 0 {
		return opts.MaxDuration
	}
	if cfg.MaxRunSeconds > 0 {
		return time.Duration(cfg.MaxRunSeconds) * time.Second
	}
	return 0
}

// runDeadlineFired 区分「本轮墙钟到点」和「下层 HTTP 客户端自己的超时」。
// 两者的错误都是 context.DeadlineExceeded，但只有本轮 ctx 被取消时才是前者。
func runDeadlineFired(ctx context.Context, err error) bool {
	return errors.Is(err, context.DeadlineExceeded) && errors.Is(ctx.Err(), context.DeadlineExceeded)
}

func openOrCreateSession(opts Options, cfg Config) (*Session, error) {
	if strings.TrimSpace(opts.SessionID) != "" {
		path, err := FindSession(opts.Env, opts.SessionID)
		if err != nil {
			return nil, err
		}
		return OpenSession(path)
	}
	return NewSession(opts.Env, cfg, opts.Cwd)
}

func buildRunner(opts Options, cfg Config, session *Session) (*SessionRunner, error) {
	home, _ := os.UserHomeDir()
	if v, ok := envGet(opts.Env, "HOME"); ok && v != "" {
		home = v
	}
	// 隔离策略非法时 fail-closed：不静默退回裸执行。
	iso, err := cfg.ShellIsolation.Policy(opts.Cwd, home)
	if err != nil {
		return nil, fmt.Errorf("shell_isolation: %w", err)
	}
	ws := &tools.Workspace{Root: opts.Cwd, Confirm: opts.Confirm, Yolo: cfg.Yolo, Isolation: iso}
	localTools, err := ws.LocalTools()
	if err != nil {
		return nil, err
	}
	search := &tools.SearchClient{Provider: tools.SearchProvider(cfg.SearchProvider), APIKey: cfg.SearchAPIKey}
	if st, serr := search.SearchTool(); serr != nil {
		return nil, serr
	} else if st != nil {
		localTools = append(localTools, st)
	}
	client := &HTTPClient{Endpoint: cfg.Endpoint, Token: cfg.Token, ProjectID: cfg.ProjectID}
	runner := &SessionRunner{
		Session:              session,
		Client:               client,
		Tools:                localTools,
		Model:                cfg.Model,
		MaxIterations:        cfg.MaxIterations,
		CompactTriggerTokens: cfg.CompactTriggerTokens,
		CompactKeepTokens:    cfg.CompactKeepTokens,
		Warn:                 func(msg string, fields ...zap.Field) { fmt.Fprintln(opts.Stderr, "warn:", msg) },
	}
	return runner, nil
}

// runREPL 是交互模式：行缓冲 + ANSI 流式渲染（不引 TUI 库，§5.4）。
func runREPL(opts Options, runtime *SessionRunner, session *Session) int {
	templates, err := LoadPromptTemplates(opts.Env)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "warn: load prompt templates: %v\n", err)
		templates = map[string]PromptTemplate{}
	}
	fmt.Fprintf(opts.Stdout, "gocloudagent %s\nsession %s\n", version, session.ID)
	fmt.Fprintf(opts.Stdout, "/help 帮助  /session 会话信息  /compact 压缩上下文  /exit 退出")
	if len(templates) > 0 {
		fmt.Fprintf(opts.Stdout, "  （另有 %d 个提示词模板，/help 查看）", len(templates))
	}
	fmt.Fprint(opts.Stdout, "\n\n")

	reader := bufio.NewReader(opts.Stdin)
	for {
		fmt.Fprint(opts.Stdout, "> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Fprintln(opts.Stdout)
				return exitOK
			}
			return fail(opts, exitUsage, "read_failed", err.Error())
		}
		text := strings.TrimSpace(line)
		if text == "" {
			continue
		}
		switch text {
		case "/exit", "/quit":
			return exitOK
		case "/help", "/?":
			fmt.Fprint(opts.Stdout, replHelp(templates))
			continue
		case "/session":
			fmt.Fprintf(opts.Stdout, "session %s\npath %s\nendpoint %s\nproject %s\n",
				session.ID, session.Path, session.Meta.Endpoint, session.Meta.ProjectID)
			continue
		case "/compact":
			// 手动压缩：忽略触发阈值，直接按 keep 预算切一刀（§13.5）。
			// 摘要是一次独立的模型调用，用可取消 ctx 以便 Ctrl+C 打断。
			cctx, ccancel := context.WithCancel(context.Background())
			ok, cerr := runtime.ForceCompact(cctx)
			ccancel()
			switch {
			case cerr != nil && !errors.Is(cerr, context.Canceled):
				fmt.Fprintf(opts.Stderr, "error: compact failed: %v\n", cerr)
			case ok:
				fmt.Fprintln(opts.Stdout, "已压缩上下文：早期对话已摘要，原文仍保留在会话文件里。")
			default:
				fmt.Fprintln(opts.Stdout, "无需压缩：当前历史不足以切出一个完整回合。")
			}
			continue
		}
		// `/name args` 展开成模板文本（只做文本替换，不授予任何权限）。
		if expanded, ok := ExpandSlashCommand(templates, text); ok {
			text = expanded
			if strings.TrimSpace(text) == "" {
				continue
			}
		}
		// Ctrl+C 取消当前 run：用独立的可取消 ctx；配置了墙钟上限时按单轮套超时。
		ctx, cancel := context.WithCancel(context.Background())
		if opts.MaxDuration > 0 {
			var tcancel context.CancelFunc
			ctx, tcancel = context.WithTimeout(ctx, opts.MaxDuration)
			prev := cancel
			cancel = func() { tcancel(); prev() }
		}
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		done := make(chan struct{})
		go func() {
			select {
			case <-sigCh:
				cancel()
			case <-done:
			}
			signal.Stop(sigCh)
		}()
		result, rerr := runtime.Run(ctx, text, replEmitter(opts))
		close(done)
		cancel()
		if rerr != nil && !errors.Is(rerr, context.Canceled) {
			fmt.Fprintf(opts.Stderr, "error: %v\n", rerr)
		}
		if result.Content != "" {
			fmt.Fprintln(opts.Stdout)
		}
	}
}

// replHelp 渲染 REPL 内建命令与模板列表。
func replHelp(templates map[string]PromptTemplate) string {
	var b strings.Builder
	b.WriteString("内建命令:\n")
	b.WriteString("  /help            显示本帮助\n")
	b.WriteString("  /session         显示当前会话信息\n")
	b.WriteString("  /compact         压缩上下文（早期对话摘要化，原文仍保留）\n")
	b.WriteString("  /exit, /quit     退出\n")
	if h := promptTemplateHelp(templates); h != "" {
		b.WriteString("\n提示词模板（~/.gocloudagent/prompts/*.md）:\n")
		b.WriteString("  ")
		b.WriteString(strings.ReplaceAll(strings.TrimRight(h, "\n"), "\n", "\n  "))
		b.WriteString("\n")
	}
	b.WriteString("\n其它: @<skill> 是能力授权，与本地的 /<template> 文本展开互不影响。\n")
	return b.String()
}

// replEmitter 在 TTY 上流式渲染：token 直接写，工具调用加前缀。
func replEmitter(opts Options) func(CLIEvent) {
	return func(e CLIEvent) {
		switch e.Type {
		case "token":
			fmt.Fprint(opts.Stdout, e.Content)
		case "tool_call":
			fmt.Fprintf(opts.Stdout, "\n  ▶ %s(%s)\n", e.Name, firstLine(e.Arguments, 160))
		case "tool_result":
			mark := "✓"
			if e.IsError {
				mark = "✗"
			}
			fmt.Fprintf(opts.Stdout, "  %s %s\n", mark, firstLine(e.Content, 200))
		}
	}
}

// emitFunc 在 headless 下的输出：json 模式逐事件一行 JSON，text 模式只写 token。
func emitFunc(opts Options) func(CLIEvent) {
	if opts.JSON {
		return func(e CLIEvent) { writeJSONEvent(opts, e) }
	}
	return func(e CLIEvent) {
		if e.Type == "token" {
			fmt.Fprint(opts.Stdout, e.Content)
		}
	}
}

func writeJSONEvent(opts Options, e CLIEvent) {
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	fmt.Fprintln(opts.Stdout, string(b))
}

func fail(opts Options, code int, errCode, message string) int {
	if opts.JSON {
		b, _ := json.Marshal(map[string]any{
			"ok":   false,
			"type": "error",
			"error": map[string]any{
				"code": errCode, "message": message,
			},
		})
		fmt.Fprintln(opts.Stderr, string(b))
		return code
	}
	fmt.Fprintf(opts.Stderr, "error: %s: %s\n", errCode, message)
	return code
}

func firstLine(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

// isTTY 判断是否为字符设备（管道/重定向时为 false）。
func isTTY(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

// version 由 cmd/gocloudagent 通过 -ldflags 覆盖。
var version = "dev"

// SetVersion 允许入口设置版本号。
func SetVersion(v string) {
	if strings.TrimSpace(v) != "" {
		version = v
	}
}

var _ = tool.BaseTool(nil)
var _ = agentcore.RedactSecrets
