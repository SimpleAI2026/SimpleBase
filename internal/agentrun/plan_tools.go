// 文件说明：plan_tools.go — internal/agentrun 的内部实现（planningToolOptions、planningTools 等）
//
// 用法
//
//	同包调用：replayPlan() → plan_tools_test.go；planCallArguments() → plan_tools_test.go；requestsReplan() → group_runner.go, plan_tools_test.go
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentrun/plan_tools.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] confirm.go, cron.go, errors.go, group_engine.go, group_runner.go 等 17 个
//	[关键引用] replayPlan ← plan_tools_test.go；planCallArguments ← plan_tools_test.go；requestsReplan ← group_runner.go, plan_tools_test.go
//
// 主要符号
//
//	type planningToolOptions struct{…}
//	(s *Service) planningTools(ctx context.Context, projectID string, opts planningToolOptions) []tool.BaseTool
//	(s *Service) runPlanningTools(ctx context.Context, projectID, runID string, agent systemdb.CloudAgent, hist []...
//	(s *Service) turnPlanningTools(ctx context.Context, projectID string, turn systemdb.AgentTurn, taskID string, ...
//	replayPlan(msgs []systemdb.AgentMessage) (plan.Decision, bool)
//	planCallArguments(toolCallsJSON string) []string
//	requestsReplan(raw string) bool

package agentrun

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/gocloudagent/group"
	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// planningToolOptions 是装配规划工具组的参数；两条接入路径只差在约束与决策出口。
type planningToolOptions struct {
	// seed 返回本轮开始前已有的决策；nil 或 ok=false 表示从空开始。
	seed func(context.Context) (plan.Decision, bool)
	// constraints 返回群约束；nil 表示单助手，只做结构校验（dispatches 可空）。
	constraints func() (plan.Constraints, bool)
	// model 是 reflect 调模型用的模型名；空取项目默认。
	model string
	// onDecision 在 plan 工具记录决策后回调。
	onDecision func(ctx context.Context, d plan.Decision)
}

// planningTools 返回绑定 MemoryStore 的 plan / explore / reflect；reflect 复用 Runtime 的模型入口。
func (s *Service) planningTools(ctx context.Context, projectID string, opts planningToolOptions) []tool.BaseTool {
	memory := &plan.MemoryStore{}
	if opts.seed != nil {
		if d, ok := opts.seed(ctx); ok {
			_ = memory.SaveDecision(ctx, d)
		}
	}
	t := &plan.Tools{
		Store:       memory,
		Constraints: opts.constraints,
		OnDecision:  opts.onDecision,
		Scope:       projectID,
		Model:       opts.model,
	}
	if s.runtime != nil {
		t.LLM = s.runtime.LLM
	}
	return t.All()
}

// runPlanningTools 是固定助手单聊的规划工具组：当前计划从 hist 里的 plan 工具卡片重放；
// 决策经 run 的 Emit 以 v3 `plan` 帧下发（非流式 run 没有 Emit，只留在 MemoryStore）。
func (s *Service) runPlanningTools(ctx context.Context, projectID, runID string, agent systemdb.CloudAgent, hist []systemdb.AgentMessage) []tool.BaseTool {
	current, hasPlan := replayPlan(hist)
	return s.planningTools(ctx, projectID, planningToolOptions{
		seed:  func(context.Context) (plan.Decision, bool) { return current, hasPlan },
		model: agent.ModelOverride,
		onDecision: func(ctx context.Context, d plan.Decision) {
			rc, ok := cloudagent.RunContextFrom(ctx)
			if !ok || rc.Emit == nil {
				return
			}
			raw, err := json.Marshal(d)
			if err != nil {
				return
			}
			rc.Emit(cloudagent.Event{Type: group.EventPlan, RunID: runID, AgentID: agent.ID, Decision: raw})
		},
	})
}

// turnPlanningTools 是群任务的规划工具组：以轮次栅栏的 Decision 打底并按群约束校验，
// 助手因此可以在执行中 update 进度或 replan；决策写入轮次事件（前端 Plan 卡按 task 归属）。
func (s *Service) turnPlanningTools(ctx context.Context, projectID string, turn systemdb.AgentTurn, taskID string, agent systemdb.CloudAgent, emit func(group.Event)) []tool.BaseTool {
	route, err := parseGroupRoute(turn.RouteJSON)
	seed, hasSeed := route.Decision, err == nil && route.Decision.Action != ""
	members, _ := (&collabGroupMembers{svc: s, projectID: projectID}).Group(ctx, turn.TagID)
	return s.planningTools(ctx, projectID, planningToolOptions{
		seed: func(context.Context) (plan.Decision, bool) { return seed, hasSeed },
		constraints: func() (plan.Constraints, bool) {
			return plan.Constraints{
				Members: members.MemberIDs, HumanID: members.HumanID, HumanDisabled: members.HumanDisabled,
				Limits: plan.DefaultLimits(),
			}, true
		},
		model: agent.ModelOverride,
		onDecision: func(_ context.Context, decision plan.Decision) {
			data, _ := json.Marshal(map[string]any{"decision": decision, "confirm_required": decision.Confirm.Required})
			emit(group.Event{Type: group.EventPlan, TaskID: taskID, AgentID: "plan", Data: data})
		},
	})
}

// replayPlan 按时间顺序重放历史消息里的 plan 工具调用，得到会话的当前计划。
func replayPlan(msgs []systemdb.AgentMessage) (plan.Decision, bool) {
	var d plan.Decision
	var ok bool
	for _, m := range msgs {
		if m.Role != "assistant" {
			continue
		}
		for _, args := range planCallArguments(m.ToolCallsJSON) {
			d, ok = plan.ReplayCall(d, ok, args)
		}
	}
	return d, ok
}

// planCallArguments 返回工具卡片里 plan 调用的 arguments（保持调用顺序）；卡片不是 JSON 数组时返回 nil。
func planCallArguments(toolCallsJSON string) []string {
	var cards []struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	if json.Unmarshal([]byte(toolCallsJSON), &cards) != nil {
		return nil
	}
	var out []string
	for _, card := range cards {
		if card.Name == plan.ToolPlan {
			out = append(out, card.Arguments)
		}
	}
	return out
}

// requestsReplan 报告本轮工具卡片里是否有 action=replan 的 plan 调用（引擎据此回到栅栏）。
func requestsReplan(raw string) bool {
	for _, args := range planCallArguments(raw) {
		var input struct {
			Action string `json:"action"`
		}
		if json.Unmarshal([]byte(args), &input) == nil && strings.EqualFold(strings.TrimSpace(input.Action), plan.ActionReplan) {
			return true
		}
	}
	return false
}
