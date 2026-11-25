// 文件说明：group_runner.go — 群任务执行器（实现 gocloudagent/group.Runner）
//
// 用法
//
//	runner := &collabGroupRunner{svc: s, projectID}      // groupEngineFor 装配
//	(r *collabGroupRunner) RunTask(ctx, in, emit)        // 一个群任务 = 一次 headless 助手 run
//	agentNamesOf(ctx, s, projectID)                      // 助手 ID → 名称（解析 @ 名称）
//
// 调用架构
//
//	[上游] gocloudagent/group（Engine 执行循环）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentrun/group_runner.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent/{cloudagent,group}, internal/agentbridge, internal/llmgateway, internal/systemdb
//	[同包] group_engine.go, plan_tools.go, run.go
//
// 主要符号
//
//	type collabGroupRunner struct {…}
//	(r *collabGroupRunner) RunTask(ctx context.Context, in group.TaskInput, emit func(group.Event)) (group.TaskResult, error)
//	(r *collabGroupRunner) history(in group.TaskInput, currentAgent string) []cloudagent.Message
//	agentNamesOf(ctx context.Context, s *Service, projectID string) map[string]string
//
// run 的创建 / 收尾与固定助手会话共用 run.go 的 finishRun；ctx 取消即中止子 run（Canceled 不算失败）。

package agentrun

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/gocloudagent/group"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// collabGroupRunner 实现 group.Runner：一个群任务 = 一次助手 run（headless，事件只回传审批帧）。
type collabGroupRunner struct {
	svc       *Service
	projectID string
}

func (r *collabGroupRunner) RunTask(ctx context.Context, in group.TaskInput, emit func(group.Event)) (group.TaskResult, error) {
	s := r.svc
	agent, err := s.ports.Agents.GetCloudAgent(ctx, r.projectID, in.AgentID)
	if errors.Is(err, sql.ErrNoRows) {
		return group.TaskResult{Failed: true, ErrorCode: "group_member_unavailable"}, nil
	}
	if err != nil {
		return group.TaskResult{}, err
	}
	if (agent.Module == agentbridge.ModuleKV || agent.Module == agentbridge.ModuleGoFunction || agent.Module == agentbridge.ModuleCron) && !s.cfg.SkillsCLI {
		return group.TaskResult{Failed: true, ErrorCode: "cli_skills_unavailable"}, nil
	}
	if agent.Module == agentbridge.ModuleSandbox && !s.cfg.Sandbox {
		return group.TaskResult{Failed: true, ErrorCode: "sandbox_unavailable"}, nil
	}
	if s.runtime == nil {
		return group.TaskResult{Failed: true, ErrorCode: "llm_not_configured"}, nil
	}
	turn, err := s.ports.Collab.GetAgentTurn(ctx, r.projectID, in.TurnID)
	if err != nil {
		return group.TaskResult{}, err
	}
	identity := collabGroupIdentityFrom(ctx)
	runID := uuid.NewString()
	run, err := s.ports.Runs.CreateAgentRun(ctx, systemdb.AgentRun{
		ID: runID, ThreadID: turn.ThreadID, ProjectID: r.projectID, AgentID: agent.ID,
		Status: systemdb.AgentRunRunning, StartedAt: time.Now().UTC(), InitiatorID: identity.principal.Actor(),
	})
	if err != nil {
		return group.TaskResult{}, err
	}
	task, err := s.ports.Collab.GetAgentTask(ctx, r.projectID, in.TaskID)
	if err != nil {
		return group.TaskResult{}, err
	}
	task.Attempt++
	if _, err := s.ports.Collab.UpdateAgentTask(ctx, task, task.Version); err != nil {
		return group.TaskResult{}, err
	}
	if in.OnRunID != nil {
		in.OnRunID(runID)
	}
	if err := s.ports.Collab.AppendAgentTaskRun(ctx, systemdb.AgentTaskRun{
		ProjectID: r.projectID, TaskID: in.TaskID, Attempt: task.Attempt,
		RunID: runID, Status: systemdb.AgentRunRunning,
	}); err != nil {
		return group.TaskResult{}, err
	}

	userText := in.Message
	if strings.TrimSpace(in.Context) != "" {
		userText += "\n\n" + in.Context
	}
	extraTools := s.turnPlanningTools(ctx, r.projectID, turn, in.TaskID, agent, emit)
	req := cloudagent.RunRequest{
		Agent: agentbridge.Agent(agent), ThreadID: turn.ThreadID, RunID: runID,
		UserText: userText, History: r.history(in, agent.ID), Stream: true,
		Headless: identity.headless, Subject: identity.principal.Actor(),
		Context: agentbridge.RunContext{
			ProjectID: r.projectID, Principal: identity.principal,
			Skills: agentbridge.SkillsForModule(agent.Module), Headless: identity.headless,
			ExtraTools: extraTools,
		},
	}
	runCtx := llmgateway.WithTraceMeta(ctx, runID, "agent")
	onEvent := s.onRunEvent(runCtx, run, agent, func(event cloudagent.Event) {
		if event.Type != group.EventConfirmationRequired && event.Type != group.EventConfirmationResolved {
			return
		}
		data, _ := json.Marshal(event)
		emit(group.Event{Type: event.Type, TaskID: in.TaskID, RunID: runID, AgentID: agent.ID, Data: data})
	})
	result, runErr := s.runtime.StartRun(runCtx, req, onEvent)
	if err := s.finishRun(context.WithoutCancel(ctx), run, agent, result, runErr); err != nil {
		return group.TaskResult{}, err
	}
	status := systemdb.AgentRunCompleted
	if errors.Is(runErr, context.Canceled) {
		status = systemdb.AgentRunCanceled
	} else if runErr != nil {
		status = systemdb.AgentRunFailed
	}
	_ = s.ports.Collab.AppendAgentTaskRun(context.WithoutCancel(ctx), systemdb.AgentTaskRun{
		ProjectID: r.projectID, TaskID: in.TaskID, Attempt: task.Attempt, RunID: runID, Status: status,
	})
	out := group.TaskResult{Content: result.Content, RunID: runID, NeedReplan: requestsReplan(result.ToolCallsJSON)}
	if errors.Is(runErr, context.Canceled) || ctx.Err() != nil {
		out.Canceled = true
		return out, nil
	}
	if runErr != nil {
		out.Failed = true
		out.ErrorCode = agentbridge.ClassifyError(runErr).Code
		if out.Content == "" {
			out.Content = runErr.Error()
		}
	}
	return out, nil
}

func (r *collabGroupRunner) history(in group.TaskInput, currentAgent string) []cloudagent.Message {
	names := agentNamesOf(context.Background(), r.svc, r.projectID)
	out := make([]cloudagent.Message, 0, len(in.History))
	for _, turn := range in.History {
		content := turn.Content
		if turn.Role == "assistant" && turn.Speaker != "" && turn.Speaker != currentAgent {
			name := turn.Speaker
			if names[name] != "" {
				name = names[name]
			}
			content = "[" + name + "] " + content
		}
		out = append(out, cloudagent.Message{Role: turn.Role, Content: content})
	}
	return out
}

// agentNamesOf 返回项目助手的 ID → 名称映射（解析 @ 名称用）；读取失败时为空映射。
func agentNamesOf(ctx context.Context, s *Service, projectID string) map[string]string {
	agents, err := s.ports.Agents.ListCloudAgents(ctx, projectID)
	if err != nil {
		return nil
	}
	out := make(map[string]string, len(agents))
	for _, agent := range agents {
		out[agent.ID] = agent.Name
	}
	return out
}
