// 文件说明：group_runner.go — 本地群运行时的成员视图与任务执行：group.Members / group.Runner 的 CLI 实现
//
// 用法：groupMembers、groupRunner、lockedWriter 都由 group_runtime.go 的 newGroupRuntime 装配；
// 引擎经 Members 读群定义与候选，经 Runner.RunTask 为每个 dispatch 跑一个成员会话。
// 调用架构
//
//	group.Engine ── Members ──► groupMembers.Group / Candidates / History（读 GroupConfig 与运行时历史）
//	group.Engine ── Runner  ──► groupRunner.RunTask ──► SessionRunner（成员模型 + 工具子集 + plan.Tools）
//	                                   │ 流式正文 / 工具调用行
//	                                   ▼
//	                          lockedWriter（与跟随循环的结构帧串行写到同一终端）
//
// 主要符号：groupMembers、groupRunner、lockedWriter、memberTools、instruction、sessionRequestsReplan。
// 依赖：agentcore / group / plan 与同包 Session / SessionRunner；不 import cloudagent、宿主包。

package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"github.com/linkxzhou/SimpleBase/gocloudagent/group"
	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
)

// ---- 成员与候选 ----

// groupMembers 实现 group.Members：群配置、候选与近期历史。
type groupMembers struct {
	cfg     GroupConfig
	history func() []plan.Turn
}

var _ group.Members = (*groupMembers)(nil)

func (m *groupMembers) Group(context.Context, string) (group.Group, error) {
	g := group.Group{
		ID: m.cfg.Name, Policy: group.Policy{
			Confirm:       plan.ConfirmPolicy{Mode: strings.ToLower(strings.TrimSpace(m.cfg.ConfirmPolicy))},
			FallbackAgent: strings.TrimSpace(m.cfg.FallbackMember),
			MaxReplans:    m.cfg.MaxReplans,
		},
	}
	for _, member := range m.cfg.Members {
		g.MemberIDs = append(g.MemberIDs, member.ID)
	}
	if m.cfg.HumanEnabled() {
		g.HumanID = "me"
		g.HumanName = osUser()
	} else {
		g.HumanDisabled = true
	}
	return g, nil
}

func (m *groupMembers) Candidates(context.Context, string) ([]plan.Candidate, error) {
	out := make([]plan.Candidate, 0, len(m.cfg.Members))
	for _, member := range m.cfg.Members {
		out = append(out, plan.Candidate{
			ID: member.ID, Name: member.ID, Module: "member",
			Description: firstLine(member.Instruction, 200), Skills: append([]string(nil), member.Tools...),
		})
	}
	return out, nil
}

func (m *groupMembers) History(_ context.Context, _ string, limit int) ([]plan.Turn, error) {
	all := m.history()
	if limit > 0 && len(all) > limit {
		all = all[len(all)-limit:]
	}
	return append([]plan.Turn(nil), all...), nil
}

// ---- 任务执行 ----

// lockedWriter 串行化对底层输出的写入（结构帧与成员正文可能来自不同协程）。
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// groupRunner 实现 group.Runner：每个任务用成员自己的模型 / 指令 / 工具子集跑一个会话。
type groupRunner struct {
	env           []string
	cfg           Config
	def           GroupConfig
	client        agentcore.ChatClient
	tools         []tool.BaseTool
	planDir       string
	out           io.Writer
	in            io.Reader
	lineReader    *bufio.Reader
	maxIterations int
	warn          func(msg string, fields ...zap.Field)
	redact        func(string) string
}

var _ group.Runner = (*groupRunner)(nil)

// RunTask 执行一个 dispatch：成员会话的流式输出带 [成员] 前缀写到终端；
// 助手调用 plan(action=replan) 时上报 NeedReplan，由引擎回到栅栏。
func (r *groupRunner) RunTask(ctx context.Context, in group.TaskInput, emit func(group.Event)) (group.TaskResult, error) {
	member, ok := r.def.Member(in.AgentID)
	if !ok {
		return group.TaskResult{Failed: true, ErrorCode: "group_member_unavailable"}, nil
	}
	if r.client == nil {
		return group.TaskResult{Failed: true, ErrorCode: "model_not_configured"}, nil
	}
	memberTools, err := r.memberTools(member)
	if err != nil {
		return group.TaskResult{}, err
	}
	session, err := NewSessionWithMeta(r.env, r.cfg, "", func(e *Entry) {
		e.Group = r.def.Name
		e.Task = in.Ref
	})
	if err != nil {
		return group.TaskResult{}, err
	}
	model := strings.TrimSpace(member.Model)
	if model == "" {
		model = r.cfg.Model
	}
	// 成员前缀单独成行：结构帧（任务状态行）从跟随协程渲染，前缀与正文不能拼在同一行上。
	fmt.Fprintf(r.out, "\n[@%s]\n", member.ID)
	runner := &SessionRunner{
		Session: session, Client: r.client, Tools: memberTools, Model: model,
		MaxIterations: r.maxIterations, SystemPrompt: r.instruction(member, in),
		Warn: r.warn, Redact: r.redact,
	}
	result, runErr := runner.Run(ctx, in.Message, func(e CLIEvent) {
		switch e.Type {
		case "token":
			fmt.Fprint(r.out, e.Content)
		case "tool_call":
			fmt.Fprintf(r.out, "\n  [@%s ▶ %s(%s)] ", member.ID, e.Name, firstLine(e.Arguments, 120))
		}
	})
	fmt.Fprintln(r.out)
	out := group.TaskResult{Content: result.Content, NeedReplan: sessionRequestsReplan(session)}
	switch {
	case errors.Is(runErr, context.Canceled) || ctx.Err() != nil:
		out.Canceled = true
	case runErr != nil:
		out.Failed = true
		out.ErrorCode = "run_failed"
		out.Content = runErr.Error()
	}
	return out, nil
}

// memberTools 返回成员的工具子集（空 = 全部）加上绑定群约束的规划工具组。
func (r *groupRunner) memberTools(member GroupMember) ([]tool.BaseTool, error) {
	var base []tool.BaseTool
	if len(member.Tools) == 0 {
		base = append(base, r.tools...)
	} else {
		want := make(map[string]bool, len(member.Tools))
		for _, n := range member.Tools {
			want[n] = true
		}
		for _, t := range r.tools {
			info, err := t.Info(context.Background())
			if err == nil && info != nil && want[info.Name] {
				base = append(base, t)
			}
		}
	}
	members, _ := (&groupMembers{cfg: r.def}).Group(context.Background(), r.def.Name)
	planning := &plan.Tools{
		Store: &plan.FileStore{Dir: r.planDir},
		Constraints: func() (plan.Constraints, bool) {
			return plan.Constraints{Members: members.MemberIDs, HumanID: members.HumanID, HumanDisabled: members.HumanDisabled, Limits: plan.DefaultLimits()}, true
		},
	}
	// FileStore 首次写入时自己 MkdirAll。
	return append(base, planning.All()...), nil
}

// instruction 是成员任务的系统提示词：成员职责 + 本轮目标 + Plan 下发的指令 + 上游结果 + 近期群聊历史。
func (r *groupRunner) instruction(member GroupMember, in group.TaskInput) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(member.Instruction))
	b.WriteString("\n\nYou are member \"" + member.ID + "\" of the local group \"" + r.def.Name + "\". ")
	b.WriteString("The group's plan coordinator assigned you exactly one task; do only this task and report the result as your final answer.")
	if in.Goal != "" {
		b.WriteString("\n\nTurn goal: " + in.Goal)
	}
	b.WriteString("\n\nYour assignment from the plan:\n" + strings.TrimSpace(in.Message) + "\n")
	if ctxText := strings.TrimSpace(in.Context); ctxText != "" {
		b.WriteString("\nResults from upstream members (context):\n" + agentcore.Truncate(ctxText, 6000) + "\n")
	}
	if len(in.History) > 0 {
		var lines []string
		for _, t := range in.History {
			content := agentcore.Truncate(strings.ReplaceAll(t.Content, "\n", " "), 300)
			if t.Speaker != "" {
				lines = append(lines, fmt.Sprintf("%s(@%s): %s", t.Role, t.Speaker, content))
			} else {
				lines = append(lines, fmt.Sprintf("%s: %s", t.Role, content))
			}
		}
		b.WriteString("\nRecent group conversation:\n- " + strings.Join(lines, "\n- ") + "\n")
	}
	b.WriteString("\nIf the overall plan should change (a step cannot be done as planned, or new information changes the split), call the `plan` tool with action=replan and the new dispatches; use action=update only to record progress of your own step.")
	return b.String()
}

// sessionRequestsReplan 报告成员会话里是否有 plan(action=replan) 的工具调用。
func sessionRequestsReplan(session *Session) bool {
	for _, e := range session.Messages() {
		for _, tc := range e.ToolCalls {
			if tc.Name != plan.ToolPlan {
				continue
			}
			var input struct {
				Action string `json:"action"`
			}
			if json.Unmarshal([]byte(tc.Arguments), &input) == nil && strings.EqualFold(strings.TrimSpace(input.Action), plan.ActionReplan) {
				return true
			}
		}
	}
	return false
}
