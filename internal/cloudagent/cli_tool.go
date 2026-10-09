package cloudagent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/database/sqlguard"
)

// DelegationIssuer 签发本轮 CLI 使用的短时令牌。
type DelegationIssuer interface {
	Issue(p auth.Principal, projectID, runID string, ttl time.Duration) (string, error)
	RevokeRun(runID string)
}

// ConfirmationWaiter 在破坏性命令执行前等待界面确认。
type ConfirmationWaiter interface {
	Wait(ctx context.Context, runID, callID string, timeout time.Duration) (approved bool, err error)
}

type simplebaseArgs struct {
	Argv []string `json:"argv" jsonschema:"description=simplebase CLI arguments, without the binary name"`
}

type callGate struct {
	mu sync.Mutex
	ch map[string]chan struct{}
}

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

func (r *Runtime) ensureGate() *callGate {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.gate == nil {
		r.gate = &callGate{}
	}
	return r.gate
}

func (r *Runtime) buildSkillTools(req RunRequest) ([]tool.BaseTool, error) {
	sb, err := utils.InferTool("simplebase", "Run the simplebase CLI for resources loaded in this turn. Pass argv as a string array. Do not include the binary name, a shell pipeline, or any token.",
		func(ctx context.Context, in simplebaseArgs) (string, error) {
			return r.execSimplebase(ctx, req, in.Argv)
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

func (r *Runtime) execSimplebase(ctx context.Context, req RunRequest, argv []string) (string, error) {
	rc, _ := runContextFrom(ctx)
	callID := compose.GetToolCallID(ctx)
	if callID != "" {
		r.ensureGate().wait(callID, 5*time.Second)
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if len(argv) == 0 || len(argv) > 64 {
		return skillFail("usage", "argv 必须是 1 到 64 段"), nil
	}
	var total int
	for _, a := range argv {
		if len(a) > 32<<10 {
			return skillFail("usage", "单个参数过长"), nil
		}
		total += len(a)
		if strings.Contains(a, "SIMPLEBASE_TOKEN") {
			return skillFail("token_in_argv", "argv 不能携带令牌"), nil
		}
	}
	if total > 256<<10 {
		return skillFail("usage", "参数合计过长"), nil
	}
	token := r.tokenFor(req.RunID)
	if token != "" {
		for _, a := range argv {
			if strings.Contains(a, token) {
				return skillFail("token_in_argv", "argv 不能携带令牌"), nil
			}
		}
	}
	skills := req.Skills
	if len(rc.Skills) > 0 {
		skills = rc.Skills
	}
	if !AllowArgv(skills, argv) {
		return skillFail("skill_not_loaded", "当前回合没有加载该命令"), nil
	}
	emit := rc.Emit
	if destructiveArgv(argv) {
		if req.Headless || rc.Headless {
			if emit != nil {
				emit(Event{Type: "confirmation_resolved", CallID: callID, Name: "simplebase", Approve: boolPtr(false), Message: "定时任务不能确认破坏性操作"})
			}
			return skillFail("confirmation_unavailable", "定时任务不能确认破坏性操作"), nil
		}
		if emit != nil {
			emit(Event{Type: "confirmation_required", CallID: callID, Name: "simplebase", Argv: redactArgv(argv), Message: confirmMessage(argv)})
		}
		approved := false
		if r.Confirm != nil && callID != "" {
			ok, err := r.Confirm.Wait(ctx, req.RunID, callID, r.confirmTimeout())
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				return "", ctx.Err()
			}
			approved = err == nil && ok
		}
		if emit != nil {
			emit(Event{Type: "confirmation_resolved", CallID: callID, Name: "simplebase", Approve: boolPtr(approved)})
		}
		if !approved {
			return skillFail("confirmation_denied", "已取消"), nil
		}
	}
	if emit != nil {
		emit(Event{Type: "tool_start", CallID: callID, Name: "simplebase"})
	}
	if r.APIBaseURL == "" || r.Issuer == nil {
		return skillFail("cli_not_configured", "CLI 未配置"), nil
	}
	if token == "" {
		return skillFail("cli_not_configured", "委托令牌未签发"), nil
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
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stdout, stderr, code, err := runner.Run(runCtx, argv, []string{
		"SIMPLEBASE_URL=" + strings.TrimRight(r.APIBaseURL, "/"),
		"SIMPLEBASE_TOKEN=" + token,
		"SIMPLEBASE_PROJECT_ID=" + req.ProjectID,
		"SIMPLEBASE_OUTPUT=json",
		"SIMPLEBASE_AGENT_RUN=1",
		"PATH=/usr/bin:/bin",
	})
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

func boolPtr(v bool) *bool { return &v }

func confirmMessage(argv []string) string {
	return "将执行破坏性命令：" + strings.Join(argv, " ")
}

func redactArgv(argv []string) []string {
	out := make([]string, len(argv))
	copy(out, argv)
	return out
}

func destructiveArgv(argv []string) bool {
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
	case "sql exec", "sql batch":
		if argv[1] == "batch" {
			path, ok := argvFlag(argv, "file")
			if !ok {
				return true
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return true
			}
			return sqlDestructive(string(b))
		}
		stmt, ok := argvFlag(argv, "statement")
		if !ok {
			return true
		}
		return sqlDestructive(stmt)
	case "database create":
		path, ok := argvFlag(argv, "init-sql")
		if !ok {
			return false
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return true
		}
		return sqlDestructive(string(b))
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

func sqlDestructive(stmt string) bool {
	parts := splitSQL(stmt)
	if len(parts) == 0 {
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

func splitSQL(stmt string) []string {
	return strings.Split(stmt, ";")
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
