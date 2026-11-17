// 文件说明：skilltool.go — simplebase CLI 工具：命令校验、权限降权与执行

package cloudagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"go.uber.org/zap"

	agentskills "github.com/linkxzhou/SimpleBase/gocloudagent/skills"
)

type simplebaseArgs struct {
	Argv []string `json:"argv" jsonschema:"description=simplebase CLI arguments, without the binary name"`
	// Stdin 是内联传给 CLI 的标准输入（planv6.0 D3）：--file - 场景的文件内容。
	// 上限 256KiB，与 handler 一致；超过上限返回 usage，由模型分段处理。
	Stdin string `json:"stdin,omitempty" jsonschema:"description=inline stdin content for --file - (max 256KiB), e.g. function source code or batch JSON"`
}

// stdinMaxBytes 是工具参数 stdin 的上限（planv6.0 §3.2）。
const stdinMaxBytes = 256 << 10

// commandPolicyUnavailable 是命令策略缺失/出错时的稳定错误码。
// 这是装配错误，不是权限问题：任何情况下都不执行命令。
const commandPolicyUnavailable = "command_policy_unavailable"

func (r *Runtime) buildSkillTools(req RunRequest) ([]tool.BaseTool, error) {
	sb, err := utils.InferTool("simplebase", "Run the simplebase CLI for resources loaded in this turn. Pass argv as a string array. Do not include the binary name, a shell pipeline, or any token. File content goes in the stdin field with --file -.",
		func(ctx context.Context, in simplebaseArgs) (string, error) {
			return r.execSimplebase(ctx, req, in.Argv, in.Stdin)
		})
	if err != nil {
		return nil, err
	}
	sandboxIDs := make([]string, 0, 4)
	for _, id := range req.Agent.ToolIDs {
		if IsSandboxTool(id) {
			sandboxIDs = append(sandboxIDs, id)
		}
	}
	out := []tool.BaseTool{sb}
	if len(sandboxIDs) > 0 {
		extra, err := buildTools(sandboxIDs, toolDeps{DB: r.DB, Obj: r.Obj, Logs: r.Logs, Sandbox: r.Sandbox})
		if err != nil {
			return nil, err
		}
		out = append(out, extra...)
	}
	return wrapToolErrors(out), nil
}

func (r *Runtime) execSimplebase(ctx context.Context, req RunRequest, argv []string, stdin string) (string, error) {
	rc, _ := runContextFrom(ctx)
	callID := compose.GetToolCallID(ctx)
	tr := r.trace()
	// rejected 记录被拒命令并返回统一的失败信封（planv6.0 v2 §5.3 打点）。
	rejected := func(code, msg string) (string, error) {
		tr.Meta("gocloudagent: cli rejected",
			zap.String("run_id", req.RunID),
			zap.String("cmd", argvSummary(argv)),
			zap.String("code", code),
		)
		return skillFail(code, msg), nil
	}
	// planv6.0 §3.4：只在增量协议下等待 tool_call 收束（最多 1s）；
	// text 协议参数一次到位，直接跳过 gate，避免无谓延迟。
	if callID != "" && r.ToolProtocol != "text" {
		r.ensureGate().wait(req.RunID, callID, time.Second)
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if code, msg := validateSkillArgv(argv, stdin); code != "" {
		return rejected(code, msg)
	}
	token := r.tokenFor(req.RunID)
	if token != "" {
		for _, a := range argv {
			if strings.Contains(a, token) {
				return rejected("token_in_argv", "argv 不能携带令牌")
			}
		}
	}
	skills := req.Skills
	if len(rc.Skills) > 0 {
		skills = rc.Skills
	}
	if !agentskills.AllowArgv(skills, argv) {
		return rejected("skill_not_loaded", "当前回合没有加载该命令")
	}
	// 命令语义唯一事实来源是宿主的 CommandPolicy（sbcli + sqlguard）。
	// 策略缺失或分类失败时拒绝执行：不允许 writable / 确认 / headless 绕过。
	if r.Policy == nil {
		return rejected(commandPolicyUnavailable, "命令安全策略未配置")
	}
	decision, err := r.Policy.Classify(argv, stdin)
	if err != nil {
		return rejected(commandPolicyUnavailable, "命令安全策略不可用")
	}
	// planv6.0 v2 §3.4 第二层（命令拦截）：不可写助手只能执行只读命令。
	// 必须在确认之前拦截，否则用户会先看到一个对不可写助手弹出的确认框。
	if !writableFor(req, rc) && !decision.ReadOnly {
		return rejected("write_not_allowed", "该助手未开启写权限，只能执行只读命令。请在「云助手 → 编辑助手 → 允许写操作」中开启后重试")
	}
	// 搭建会话的阶段限权（home-starter-prompts-plan §3.3）：阶段内被标为只读的技能
	// （如 func 阶段的 database）只允许只读命令，写命令不属于当前阶段，同样在确认前拦截。
	readonly := req.ReadOnlySkills
	if len(rc.ReadOnlySkills) > 0 {
		readonly = rc.ReadOnlySkills
	}
	if len(readonly) > 0 && !decision.ReadOnly && !agentskills.AllowWriteArgv(skills, readonly, argv) {
		return rejected("write_not_allowed", "当前阶段不允许该写命令，请只执行本阶段的操作")
	}
	emit := rc.Emit
	if decision.Destructive {
		if req.Headless || rc.Headless {
			if emit != nil {
				emit(Event{Type: "confirmation_resolved", CallID: callID, Name: "simplebase", Approve: boolPtr(false), Message: "定时任务不能确认破坏性操作"})
			}
			return rejected("confirmation_unavailable", "定时任务不能确认破坏性操作")
		}
		if emit != nil {
			// planv6.0 §3.4：发出事件前先登记确认项，消除「确认请求先于
			// Wait 登记」的竞态（Resolve 会 404）。
			if p, ok := r.Confirm.(ConfirmationPreparer); ok && callID != "" {
				p.Prepare(req.RunID, callID, principalKey(req.Principal))
			}
			emit(Event{Type: "confirmation_required", CallID: callID, Name: "simplebase", Argv: r.Policy.RedactArgv(argv), Message: confirmMessage(argv)})
		}
		tr.Meta("gocloudagent: confirmation requested",
			zap.String("run_id", req.RunID),
			zap.String("call_id", callID),
			zap.String("cmd", argvSummary(argv)),
		)
		tr.Payload("gocloudagent: confirmation argv", "argv", argvRedacted(r.Policy, argv))
		approved := false
		if r.Confirm != nil && callID != "" {
			ok, err := r.Confirm.Wait(ctx, req.RunID, callID, principalKey(req.Principal), r.confirmTimeout())
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				return "", ctx.Err()
			}
			approved = err == nil && ok
		}
		if emit != nil {
			emit(Event{Type: "confirmation_resolved", CallID: callID, Name: "simplebase", Approve: boolPtr(approved)})
		}
		tr.Meta("gocloudagent: confirmation resolved",
			zap.String("run_id", req.RunID),
			zap.String("call_id", callID),
			zap.Bool("approved", approved),
		)
		if !approved {
			return skillFail("confirmation_denied", "已取消"), nil
		}
	}
	return r.runSkillCLI(ctx, req, argv, stdin, token, callID, emit, tr, rejected)
}

// validateSkillArgv 做纯参数校验：长度、令牌字面量、stdin 上限、--file 必须是 -。
// 返回空 code 表示通过。
func validateSkillArgv(argv []string, stdin string) (code, msg string) {
	if len(argv) == 0 || len(argv) > 64 {
		return "usage", "argv 必须是 1 到 64 段"
	}
	var total int
	for _, a := range argv {
		if len(a) > 32<<10 {
			return "usage", "单个参数过长"
		}
		total += len(a)
		if strings.Contains(a, "SIMPLEBASE_TOKEN") {
			return "token_in_argv", "argv 不能携带令牌"
		}
	}
	if total > 256<<10 {
		return "usage", "参数合计过长"
	}
	// planv6.0 D3：stdin 上限 256KiB；文件内容一律内联，宿主机文件不可读。
	if len(stdin) > stdinMaxBytes {
		return "usage", "stdin 超过 256KiB 上限"
	}
	// --file / --init-sql 的值必须是 "-"（stdin）；其他路径一律拒绝。
	for _, name := range []string{"file", "init-sql"} {
		if v, ok := argvFlag(argv, name); ok && v != "-" {
			return "file_not_allowed", "助手模式不能读取宿主机文件，请用 --file - 并把内容放入 stdin"
		}
	}
	return "", ""
}

// runSkillCLI 执行已通过校验与确认的命令，并把退出码翻成给模型的文本。
func (r *Runtime) runSkillCLI(ctx context.Context, req RunRequest, argv []string, stdin, token, callID string,
	emit func(Event), tr tracer, rejected func(code, msg string) (string, error)) (string, error) {
	if emit != nil {
		emit(Event{Type: "tool_start", CallID: callID, Name: "simplebase"})
	}
	if r.APIBaseURL == "" || r.Issuer == nil {
		return rejected("cli_not_configured", "CLI 未配置")
	}
	if token == "" {
		return rejected("cli_not_configured", "委托令牌未签发")
	}
	runner := r.CLI
	if runner == nil {
		runner = SubprocessCLI{Path: r.CLIPath}
	}
	timeout := 60 * time.Second
	joined := strings.Join(argv, " ")
	if strings.Contains(joined, "function test") || (len(argv) >= 2 && argv[0] == "function" && argv[1] == "test") {
		timeout = 120 * time.Second
	}
	started := time.Now()
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stdout, stderr, code, err := runner.Run(runCtx, argv, []string{
		"SIMPLEBASE_URL=" + strings.TrimRight(r.APIBaseURL, "/"),
		"SIMPLEBASE_TOKEN=" + token,
		"SIMPLEBASE_PROJECT_ID=" + req.ProjectID,
		"SIMPLEBASE_OUTPUT=json",
		"SIMPLEBASE_AGENT_RUN=1",
		"PATH=/usr/bin:/bin",
	}, stdin)
	// 只记命令摘要、耗时与退出码；不记环境变量、stdin 与令牌。
	tr.Meta("gocloudagent: cli executed",
		zap.String("run_id", req.RunID),
		zap.String("cmd", argvSummary(argv)),
		zap.Int64("duration_ms", time.Since(started).Milliseconds()),
		zap.Int("exit_code", code),
		zap.String("stdout", hashSummary(stdout)),
		zap.String("stderr", hashSummary(stderr)),
	)
	tr.Payload("gocloudagent: cli stdout", "stdout", stdout)
	tr.Payload("gocloudagent: cli stderr", "stderr", stderr)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return skillFail("cli_timeout", "命令超时"), nil
	}
	if err != nil {
		return skillFail("cli_failed", "无法启动 CLI"), nil
	}
	switch code {
	case 0:
		return strings.TrimSpace(stdout), nil
	case 2:
		if strings.TrimSpace(stderr) != "" {
			return strings.TrimSpace(stderr), nil
		}
		return skillFail("api_error", "API 调用失败"), nil
	case 4:
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = skillFail("config_missing", "CLI 配置无效")
		}
		return msg, nil
	default:
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = skillFail("usage", "命令用法错误")
		}
		return msg, nil
	}
}

func skillFail(code, message string) string {
	b, _ := json.Marshal(map[string]any{"ok": false, "error": map[string]any{"code": code, "message": message}})
	return string(b)
}

// writableFor 报告本轮助手是否允许写（planv6.0 v2 §3.4）。
// RunContext 里的值优先（它由 Runtime 按 req.Agent.Writable 注入）；
// 两者都为假时按只读处理（fail-closed）。
func writableFor(req RunRequest, rc RunContext) bool {
	return rc.Writable || req.Agent.Writable
}

// stripWritePermissions 移除写类权限，供不可写助手的委托令牌使用。
//
// 必须复制权限集合：宿主侧的委托签发会遍历它写进 claims，直接改原 map
// 会污染调用方身份（例如同一 principal 的后续可写请求）。
func stripWritePermissions(p Principal) Principal {
	if len(p.Permissions) == 0 {
		return p
	}
	out := make(map[Permission]struct{}, len(p.Permissions))
	for perm := range p.Permissions {
		switch perm {
		case DatabaseWrite, DatabaseAdmin:
			continue
		default:
			out[perm] = struct{}{}
		}
	}
	p.Permissions = out
	return p
}

// principalKey 生成发起人标识（planv6.0 D5）：登录态为 UserID，
// API Key 为 "apikey:<id>"。与 api 包的判定保持一致。
func principalKey(p Principal) string {
	if p.UserID != "" {
		return p.UserID
	}
	if p.APIKeyID != "" {
		return "apikey:" + p.APIKeyID
	}
	return ""
}

func boolPtr(v bool) *bool { return &v }

func confirmMessage(argv []string) string {
	return "将执行破坏性命令：" + strings.Join(argv, " ")
}

// argvFlag 读取 --name value / --name=value。
//
// 这里只用于本模块的参数检查（--file / --init-sql 必须为 "-"），
// 命令安全分类与展示脱敏由宿主 CommandPolicy 负责。
func argvFlag(argv []string, name string) (string, bool) {
	flag := "--" + name
	for i := 0; i < len(argv); i++ {
		if argv[i] == flag && i+1 < len(argv) {
			return argv[i+1], true
		}
		if strings.HasPrefix(argv[i], flag+"=") {
			return strings.TrimPrefix(argv[i], flag+"="), true
		}
	}
	return "", false
}

func toolResultIsError(content string) bool {
	var env struct {
		OK      *bool `json:"ok"`
		IsError bool  `json:"is_error"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(content)), &env) != nil {
		return false
	}
	if env.IsError {
		return true
	}
	return env.OK != nil && !*env.OK
}
