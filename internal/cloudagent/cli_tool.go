package cloudagent

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/database/sqlguard"
	"github.com/linkxzhou/SimpleBase/internal/sbcli"
)

// DelegationIssuer 签发本轮 CLI 使用的短时令牌。
type DelegationIssuer interface {
	Issue(p auth.Principal, projectID, runID string, ttl time.Duration) (string, error)
	RevokeRun(runID string)
}

// ConfirmationWaiter 在破坏性命令执行前等待界面确认。
// initiator 是发起人标识（planv6.0 D5），由确认端做比对。
type ConfirmationWaiter interface {
	Wait(ctx context.Context, runID, callID, initiator string, timeout time.Duration) (approved bool, err error)
}

// ConfirmationPreparer 是可选的预登记接口（planv6.0 §3.4）：
// 在 confirmation_required 事件发出前登记等待项，消除早到确认的竞态。
type ConfirmationPreparer interface {
	Prepare(runID, callID, initiator string)
}

type simplebaseArgs struct {
	Argv []string `json:"argv" jsonschema:"description=simplebase CLI arguments, without the binary name"`
	// Stdin 是内联传给 CLI 的标准输入（planv6.0 D3）：--file - 场景的文件内容。
	// 上限 256KiB，与 handler 一致；超过上限返回 usage，由模型分段处理。
	Stdin string `json:"stdin,omitempty" jsonschema:"description=inline stdin content for --file - (max 256KiB), e.g. function source code or batch JSON"`
}

// stdinMaxBytes 是工具参数 stdin 的上限（planv6.0 §3.2）。
const stdinMaxBytes = 256 << 10

type callGate struct {
	mu sync.Mutex
	ch map[string]chan struct{}
}

// wait 阻塞到该 call_id 的 tool_call 收束或超时（planv6.0 §3.4）。
// 无论 release 还是超时，结束后删除登记，防止残留累积。
func (g *callGate) wait(id string, d time.Duration) {
	if g == nil || id == "" {
		return
	}
	g.mu.Lock()
	if g.ch == nil {
		g.ch = map[string]chan struct{}{}
	}
	ch, ok := g.ch[id]
	if !ok {
		ch = make(chan struct{})
		g.ch[id] = ch
	}
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		delete(g.ch, id)
		g.mu.Unlock()
	}()
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ch:
	case <-timer.C:
	}
}

func (g *callGate) release(id string) {
	if g == nil || id == "" {
		return
	}
	g.mu.Lock()
	if g.ch == nil {
		g.ch = map[string]chan struct{}{}
	}
	ch, ok := g.ch[id]
	if !ok {
		ch = make(chan struct{})
		g.ch[id] = ch
	}
	g.mu.Unlock()
	select {
	case <-ch:
	default:
		close(ch)
	}
}

// dropRun 在 run 结束时清理残留登记（planv6.0 §3.4）。
// callID 无 run 维度映射，超时路径的删除已由 wait 的 defer 保证；
// 此处兜底清空全部残留（调用点持有 Runtime 锁语义外，仅发生在 run 收尾）。
func (g *callGate) dropRun(runID string) {
	_ = runID
	if g == nil {
		return
	}
	g.mu.Lock()
	g.ch = nil
	g.mu.Unlock()
}

func (r *Runtime) ensureGate() *callGate {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.gate == nil {
		r.gate = &callGate{}
	}
	return r.gate
}

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
		tr.Meta("cloudagent: cli rejected",
			zap.String("run_id", req.RunID),
			zap.String("cmd", argvSummary(argv)),
			zap.String("code", code),
		)
		return skillFail(code, msg), nil
	}
	// planv6.0 §3.4：只在增量协议下等待 tool_call 收束（最多 1s）；
	// text 协议参数一次到位，直接跳过 gate，避免无谓延迟。
	if callID != "" && r.ToolProtocol != "text" {
		r.ensureGate().wait(callID, time.Second)
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if len(argv) == 0 || len(argv) > 64 {
		return rejected("usage", "argv 必须是 1 到 64 段")
	}
	var total int
	for _, a := range argv {
		if len(a) > 32<<10 {
			return rejected("usage", "单个参数过长")
		}
		total += len(a)
		if strings.Contains(a, "SIMPLEBASE_TOKEN") {
			return rejected("token_in_argv", "argv 不能携带令牌")
		}
	}
	if total > 256<<10 {
		return rejected("usage", "参数合计过长")
	}
	// planv6.0 D3：stdin 上限 256KiB；文件内容一律内联，宿主机文件不可读。
	if len(stdin) > stdinMaxBytes {
		return rejected("usage", "stdin 超过 256KiB 上限")
	}
	// --file / --init-sql 的值必须是 "-"（stdin）；其他路径一律拒绝。
	for _, name := range []string{"file", "init-sql"} {
		if v, ok := argvFlag(argv, name); ok && v != "-" {
			return rejected("file_not_allowed", "助手模式不能读取宿主机文件，请用 --file - 并把内容放入 stdin")
		}
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
	if !AllowArgv(skills, argv) {
		return rejected("skill_not_loaded", "当前回合没有加载该命令")
	}
	// planv6.0 v2 §3.4 第二层（命令拦截）：不可写助手只能执行只读命令。
	// 必须在 destructiveArgv/Confirm.Wait 之前拦截，否则用户会先看到一个
	// 对不可写助手弹出的确认框。判定事实来源是 sbcli.CommandReadOnly。
	if !writableFor(req, rc) && !sbcli.CommandReadOnlyWithArgv(argv) {
		return rejected("write_not_allowed", "该助手未开启写权限，只能执行只读命令。请在「云助手 → 编辑助手 → 允许写操作」中开启后重试")
	}
	emit := rc.Emit
	if destructiveArgv(argv, stdin) {
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
			emit(Event{Type: "confirmation_required", CallID: callID, Name: "simplebase", Argv: redactArgv(argv), Message: confirmMessage(argv)})
		}
		tr.Meta("cloudagent: confirmation requested",
			zap.String("run_id", req.RunID),
			zap.String("call_id", callID),
			zap.String("cmd", argvSummary(argv)),
		)
		tr.Payload("cloudagent: confirmation argv", "argv", argvRedacted(argv))
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
		tr.Meta("cloudagent: confirmation resolved",
			zap.String("run_id", req.RunID),
			zap.String("call_id", callID),
			zap.Bool("approved", approved),
		)
		if !approved {
			return skillFail("confirmation_denied", "已取消"), nil
		}
	}
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
	if strings.Contains(joined, "function test") || strings.HasPrefix(joined, "sandbox run") || (len(argv) >= 2 && argv[0] == "sandbox" && argv[1] == "run") || (len(argv) >= 2 && argv[0] == "function" && argv[1] == "test") {
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
	tr.Meta("cloudagent: cli executed",
		zap.String("run_id", req.RunID),
		zap.String("cmd", argvSummary(argv)),
		zap.Int64("duration_ms", time.Since(started).Milliseconds()),
		zap.Int("exit_code", code),
		zap.String("stdout", hashSummary(stdout)),
		zap.String("stderr", hashSummary(stderr)),
	)
	tr.Payload("cloudagent: cli stdout", "stdout", stdout)
	tr.Payload("cloudagent: cli stderr", "stderr", stderr)
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
func stripWritePermissions(p auth.Principal) auth.Principal {
	if len(p.Permissions) == 0 {
		return p
	}
	out := make(map[auth.Permission]struct{}, len(p.Permissions))
	for perm := range p.Permissions {
		switch perm {
		case auth.DatabaseWrite, auth.DatabaseAdmin:
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
func principalKey(p auth.Principal) string {
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

// sensitiveKeyPattern 匹配敏感 flag 名（planv6.0 §3.5：值替换 ***）。
var sensitiveKeyPattern = regexp.MustCompile(`(?i)(^|_)(password|passwd|secret|api_key|access_key|token|credential|dsn|authorization)($|_)`)

// redactArgv 生成确认事件里的脱敏 argv（planv6.0 §3.5）：
//   - --statement 只保留首关键字 + 前 200 字符（防确认框泄漏大段 SQL/数据）；
//   - kv exec 的 --arg 除第一个（key）以外全部替换 ***；
//   - 匹配敏感词的 flag 值替换 ***。
//
// 同时支持 "--k v" 与 "--k=v" 两种形式。
func redactArgv(argv []string) []string {
	out := make([]string, len(argv))
	copy(out, argv)
	kvArgSeen := 0
	for i := 0; i < len(out); i++ {
		name, inlineValue, isFlag := splitFlag(out[i])
		if !isFlag {
			continue
		}
		// 取值：内联（--k=v）或下一个非 flag 元素（--k v）。
		valueIdx := -1
		value := ""
		if inlineValue != nil {
			value = *inlineValue
		} else if i+1 < len(out) && !strings.HasPrefix(out[i+1], "--") {
			valueIdx = i + 1
			value = out[valueIdx]
		}
		if value == "" && inlineValue == nil {
			continue
		}
		switch {
		case name == "statement":
			if inlineValue != nil {
				out[i] = "--statement " + redactStatement(*inlineValue)
			} else {
				out[valueIdx] = redactStatement(value)
			}
		case name == "arg" && isKVExec(argv):
			kvArgSeen++
			// 第一个 --arg 是 key，保留；其余替换。
			if kvArgSeen > 1 {
				if inlineValue != nil {
					out[i] = "--arg=***"
				} else {
					out[valueIdx] = "***"
				}
			}
		case sensitiveKeyPattern.MatchString(name):
			if inlineValue != nil {
				out[i] = "--" + name + "=***"
			} else if valueIdx >= 0 {
				out[valueIdx] = "***"
			}
		}
		if valueIdx >= 0 {
			i = valueIdx
		}
	}
	return out
}

// splitFlag 拆 "--name value" / "--name=value"；非 flag 返回 isFlag=false。
func splitFlag(s string) (name string, value *string, isFlag bool) {
	if !strings.HasPrefix(s, "--") {
		return "", nil, false
	}
	rest := s[2:]
	if eq := strings.Index(rest, "="); eq >= 0 {
		v := rest[eq+1:]
		return rest[:eq], &v, true
	}
	return rest, nil, true
}

// flagValueAt 取 argv[i] 的 flag 值（支持 --k v 与 --k=v）。
func flagValueAt(argv []string, i int) (string, bool) {
	s := argv[i]
	if eq := strings.Index(s, "="); eq >= 0 && strings.HasPrefix(s, "--") {
		return s[eq+1:], true
	}
	if i+1 < len(argv) && !strings.HasPrefix(argv[i+1], "--") {
		return argv[i+1], true
	}
	return "", false
}

func isKVExec(argv []string) bool {
	return len(argv) >= 2 && argv[0] == "kv" && argv[1] == "exec"
}

// redactStatement 保留首关键字与前 200 字符（截断时不重复关键字）。
func redactStatement(stmt string) string {
	stmt = strings.TrimSpace(stmt)
	if len(stmt) <= 200 {
		return stmt
	}
	kw := sqlguard.FirstKeyword(stmt)
	head := stmt
	if len(head) > 200 {
		head = head[:200] + "..."
	}
	if kw != "" && strings.HasPrefix(stmt, kw+" ") {
		return kw + " " + head[len(kw)+1:]
	}
	return head
}

// destructiveArgv 判断是否破坏性命令（planv6.0 D3）。
// stdin 是工具参数内联的内容；宿主机文件不可读，分析对象只有 argv 与 stdin。
func destructiveArgv(argv []string, stdin string) bool {
	if len(argv) < 2 {
		return false
	}
	switch argv[0] + " " + argv[1] {
	case "database delete", "document delete", "object delete", "function delete",
		"cron delete", "cron trigger", "sandbox delete", "user delete", "user disable",
		"apikey revoke", "settings set":
		return true
	case "log retention":
		return len(argv) >= 3 && argv[2] == "set"
	case "kv exec":
		cmd, ok := argvFlag(argv, "command")
		return ok && strings.EqualFold(cmd, "DEL")
	case "sql exec":
		stmt, ok := argvFlag(argv, "statement")
		if !ok {
			return true
		}
		if stmt == "-" {
			stmt = stdin
		}
		return sqlDestructive(stmt)
	case "sql batch":
		// planv6.0 D3/--statement 内联优先；--file 只能是 "-"（stdin），
		// execSimplebase 已在更早处拒绝非 "-" 路径。
		if stmt, ok := argvFlag(argv, "statement"); ok {
			return sqlDestructive(stmt)
		}
		if path, ok := argvFlag(argv, "file"); ok && path == "-" {
			return sqlDestructive(stdin)
		}
		return true
	case "database create":
		path, ok := argvFlag(argv, "init-sql")
		if !ok {
			return false
		}
		if path == "-" {
			return sqlDestructive(stdin)
		}
		return sqlDestructive(path)
	default:
		return false
	}
}

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

// sqlDestructive 按 sqlguard 切分后逐句判定（planv6.0 §3.5：复用
// 认识引号与注释的切分，不在 cloudagent 里再写一套）。
func sqlDestructive(stmt string) bool {
	parts, err := sqlguard.SplitStatements(stmt)
	if err != nil || len(parts) == 0 {
		return true
	}
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if err := sqlguard.Validate(p, sqlguard.ReadOnly); err == nil {
			continue
		}
		switch sqlguard.FirstKeyword(p) {
		case "INSERT", "UPDATE", "CREATE", "REPLACE":
			continue
		case "ALTER":
			if strings.Contains(strings.ToUpper(p), "DROP") {
				return true
			}
			continue
		case "DROP", "DELETE", "TRUNCATE":
			return true
		default:
			return true
		}
	}
	return false
}

func (r *Runtime) confirmTimeout() time.Duration {
	if r.ConfirmTimeout > 0 {
		return r.ConfirmTimeout
	}
	return 2 * time.Minute
}

func (r *Runtime) delegationTTL() time.Duration {
	if r.DelegationTTL > 0 {
		return r.DelegationTTL
	}
	return 15 * time.Minute
}

func (r *Runtime) rememberToken(runID, token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tokens == nil {
		r.tokens = map[string]string{}
	}
	r.tokens[runID] = token
}

func (r *Runtime) forgetToken(runID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tokens, runID)
}

func (r *Runtime) tokenFor(runID string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tokens[runID]
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
