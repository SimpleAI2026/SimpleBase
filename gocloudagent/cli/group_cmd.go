// 文件说明：group_cmd.go — `gocloudagent group ...` 子命令（planv6.8 §13.8 CLI 落地）
//
// 用法
//
//	gocloudagent group list                        列出群定义
//	gocloudagent group show <name>                 查看成员与策略
//	gocloudagent group new <name>                  生成单成员样例（已存在则不覆盖）
//	gocloudagent group run <name> "消息" [--yes]    headless 跑一轮：Plan 栅栏 → 执行 → 结果
//
// headless 下整份计划需要确认时：--yes 批准，否则按策略拒绝并记录；人工任务只提示，
// 到 REPL（/group <name>）里用 /reply 处理。退出码沿用 CLI 约定：0 完成 / 1 用法或拒绝 / 2 失败 / 4 配置。

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/cloudwego/eino/components/tool"
	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

func cmdGroup(env []string, args []string, stdout, stderr *os.File) int {
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "list", "ls":
		return groupList(env, stdout, stderr)
	case "show":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "usage: gocloudagent group show <name>")
			return exitUsage
		}
		return groupShow(env, args[1], stdout, stderr)
	case "new", "create":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "usage: gocloudagent group new <name>")
			return exitUsage
		}
		return groupNew(env, args[1], stdout, stderr)
	case "run":
		return groupRunCommand(env, args[1:], stdout, stderr)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, `gocloudagent group <subcommand>

  list                        show defined groups
  show <name>                 show members and policies
  new <name>                  scaffold a one-member group (never overwrites)
  run <name> "message" [--yes] [--json] [--cwd dir]
                              run one turn headless: plan fence → dispatch → result
  help                        show this message

Groups live in ~/.gocloudagent/groups/<name>.json:
  {"name":"review","fallback_member":"explorer",
   "members":[{"id":"explorer","instruction":"You map the code...","tools":["read","glob","grep"]},
              {"id":"writer","instruction":"You implement...","tools":["read","write","edit","bash"]}]}
Each member = model + instruction + tool subset. Every message is planned first
(gocloudagent/group fence); plan files and turn state are kept under groups/<name>/.
In the REPL, "/group <name>" enters a group; "/reply" answers human tasks.
`)
		return exitOK
	default:
		fmt.Fprintf(stderr, "unknown group subcommand %q (try `gocloudagent group help`)\n", sub)
		return exitUsage
	}
}

func groupList(env []string, stdout, stderr *os.File) int {
	names, err := ListGroups(env)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitConfig
	}
	if len(names) == 0 {
		fmt.Fprintf(stdout, "no groups defined (try `gocloudagent group new <name>`; files live in %s)\n", GroupsDir(env))
		return exitOK
	}
	for _, name := range names {
		def, err := LoadGroup(env, name)
		if err != nil {
			fmt.Fprintf(stdout, "%s\t(invalid: %v)\n", name, err)
			continue
		}
		ids := make([]string, 0, len(def.Members))
		for _, m := range def.Members {
			ids = append(ids, m.ID)
		}
		fmt.Fprintf(stdout, "%s\t%d members\t%s\n", name, len(def.Members), strings.Join(ids, ","))
	}
	return exitOK
}

func groupShow(env []string, name string, stdout, stderr *os.File) int {
	def, err := LoadGroup(env, name)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return groupConfigExit(err)
	}
	fmt.Fprintf(stdout, "group %s\n", def.Name)
	if def.RoutingPrompt != "" {
		fmt.Fprintf(stdout, "  routing prompt : %s\n", def.RoutingPrompt)
	}
	policy := strings.ToLower(strings.TrimSpace(def.ConfirmPolicy))
	if policy == "" {
		policy = "auto"
	}
	fmt.Fprintf(stdout, "  confirm policy : %s\n", policy)
	if def.FallbackMember != "" {
		fmt.Fprintf(stdout, "  fallback       : @%s\n", def.FallbackMember)
	}
	fmt.Fprintf(stdout, "  human tasks    : %v\n", def.HumanEnabled())
	for _, m := range def.Members {
		model := m.Model
		if model == "" {
			model = "(default)"
		}
		tools := strings.Join(m.Tools, ",")
		if tools == "" {
			tools = "(all)"
		}
		fmt.Fprintf(stdout, "  @%-12s model=%s tools=%s\n    %s\n", m.ID, model, tools, firstLine(m.Instruction, 160))
	}
	fmt.Fprintf(stdout, "plan files & turn state: %s\n", GroupStateDir(env, def.Name))
	return exitOK
}

func groupNew(env []string, name string, stdout, stderr *os.File) int {
	path, created, err := ScaffoldGroup(env, name)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return groupConfigExit(err)
	}
	if created {
		fmt.Fprintf(stdout, "created %s\nedit members (id / instruction / tools), then `/group %s` in the REPL or `gocloudagent group run %s \"...\"`\n", path, strings.TrimSpace(name), strings.TrimSpace(name))
	} else {
		fmt.Fprintf(stdout, "exists %s (not overwritten)\n", path)
	}
	return exitOK
}

// groupRunCommand 处理 `group run <name> <message> [--yes] [--json] [--cwd dir]`。
func groupRunCommand(env []string, args []string, stdout, stderr *os.File) int {
	opts := Options{Env: env, Stdout: stdout, Stderr: stderr, Headless: true}
	yes := false
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--yes", "-y":
			yes = true
		case "--json":
			opts.JSON = true
		case "--cwd":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "usage: gocloudagent group run <name> \"message\" [--yes] [--json] [--cwd dir]")
				return exitUsage
			}
			i++
			opts.Cwd = args[i]
		case "--shell-isolation":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "usage: gocloudagent group run <name> \"message\" [--yes] [--json] [--cwd dir]")
				return exitUsage
			}
			i++
			iso, err := ShellIsolationFromFlag(args[i])
			if err != nil {
				fmt.Fprintf(stderr, "error: %v\n", err)
				return exitUsage
			}
			opts.Config.ShellIsolation = overlayShellIsolation(opts.Config.ShellIsolation, iso)
		default:
			positional = append(positional, args[i])
		}
	}
	if len(positional) < 2 {
		fmt.Fprintln(stderr, "usage: gocloudagent group run <name> \"message\" [--yes] [--json] [--cwd dir]")
		return exitUsage
	}
	return groupRun(opts, positional[0], strings.Join(positional[1:], " "), yes)
}

// groupRun 读取配置、装配模型客户端与工具集，再 headless 跑一轮。
func groupRun(opts Options, name, message string, yes bool) int {
	if opts.Cwd == "" {
		opts.Cwd, _ = os.Getwd()
	}
	// 先认群再认配置：群名写错时直接给出用法提示，而不是先报模型未配置。
	def, err := LoadGroup(opts.Env, name)
	if err != nil {
		return fail(opts, groupConfigExit(err), "group_invalid", err.Error())
	}
	fileCfg, err := LoadConfig(opts.Env)
	if err != nil {
		return fail(opts, exitConfig, "config_invalid", err.Error())
	}
	cfg := MergeEnv(overlayConfig(fileCfg, opts.Config), opts.Env)
	if err := cfg.Validate(); err != nil {
		return fail(opts, exitConfig, "config_invalid", err.Error())
	}
	client, localTools, release, err := buildLocalTools(opts, cfg)
	if err != nil {
		return fail(opts, exitUsage, "setup_failed", err.Error())
	}
	defer release()
	return runGroupHeadless(opts, cfg, def, client, localTools, message, yes)
}

// runGroupHeadless 用给定的模型客户端与工具集 headless 跑一轮（测试可直接注入假客户端）。
func runGroupHeadless(opts Options, cfg Config, def GroupConfig, client agentcore.ChatClient, localTools []tool.BaseTool, message string, yes bool) int {
	rt, err := newGroupRuntime(opts.Env, cfg, def, client, localTools, groupRuntimeOptions{
		Out: opts.Stdout, MaxIterations: cfg.MaxIterations,
		Warn: func(msg string, _ ...zap.Field) { fmt.Fprintln(opts.Stderr, "warn:", msg) },
	})
	if err != nil {
		return fail(opts, exitUsage, "setup_failed", err.Error())
	}
	baseCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ctx := baseCtx
	if limit := runLimit(opts, cfg); limit > 0 {
		var tcancel context.CancelFunc
		ctx, tcancel = context.WithTimeout(baseCtx, limit)
		defer tcancel()
	}
	code := rt.submit(ctx, message, false, yes)
	if errors.Is(ctx.Err(), context.Canceled) && code != exitOK {
		return fail(opts, exitUsage, "canceled", "interrupted")
	}
	return code
}

// groupConfigExit 把群定义错误映射为退出码。
func groupConfigExit(err error) int {
	if errors.Is(err, ErrGroupNotFound) {
		return exitUsage
	}
	return exitConfig
}
