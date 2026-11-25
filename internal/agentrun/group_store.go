// 文件说明：group_store.go — 群引擎的仓储适配器（实现 gocloudagent/group.Store）
//
// 用法
//
//	store := &collabGroupStore{store: ports.Collab, projectID, claimTurn: svc.claimTurn}   // groupEngineFor 装配
//	(s *collabGroupStore) CreateTurn / TransitionTurn / SaveDecision / CreateTasks / TransitionTask …
//	(s *collabGroupStore) LoadTasks(ctx, turnID)                       // 任务模型视图（入口的轮次快照也经此读取）
//	parseGroupRoute / marshalGroupRoute                                // route_json 信封（version 3）
//
// 调用架构
//
//	[上游] gocloudagent/group（Engine 的全部持久化）、internal/agentrun（turn.go 的快照读取）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentrun/group_store.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent/group, gocloudagent/plan, internal/systemdb（行类型）
//	[同包] group_engine.go, turn.go, plan_tools.go
//
// 主要符号
//
//	type groupRouteEnvelope struct {…}
//	type collabGroupStore struct {…}
//	parseGroupRoute(raw string) (groupRouteEnvelope, error)
//	marshalGroupRoute(route groupRouteEnvelope) string
//	... 另有 19 个顶层符号
//
// 不新增列或迁移：决策、确认状态、历史与结果都复用既有 route_json（信封 version 3）。

package agentrun

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/group"
	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// groupRouteEnvelope 复用既有 route_json，避免新增列或迁移。
type groupRouteEnvelope struct {
	Version        int               `json:"version"`
	Decision       plan.Decision     `json:"decision"`
	ConfirmState   string            `json:"confirm_state,omitempty"`
	Replans        int               `json:"replans,omitempty"`
	Summarized     bool              `json:"summarized,omitempty"`
	UserText       string            `json:"user_text,omitempty"`
	Mentions       []string          `json:"mentions,omitempty"`
	MentionedHuman bool              `json:"mentioned_human,omitempty"`
	RoutingPrompt  string            `json:"routing_prompt,omitempty"`
	History        []plan.Turn       `json:"history,omitempty"`
	Results        map[string]string `json:"results,omitempty"`
}

// collabGroupStore 实现 group.Store：把通用轮次 / 任务 / 事件模型落到 systemdb 的协作表。
// 轮次 / 任务状态迁移只经本适配器（乐观锁 version + 非法迁移报错），宿主不另写状态机。
type collabGroupStore struct {
	store     CollabStore
	projectID string
	claimTurn func(context.Context, string, string, string) error
	mu        sync.Mutex
}

func (s *collabGroupStore) CreateTurn(ctx context.Context, in group.Turn) (group.Turn, bool, error) {
	kind, id := splitInitiator(in.Initiator)
	created, reused, err := s.store.CreateAgentTurn(ctx, systemdb.AgentTurn{
		ID: in.ID, ProjectID: s.projectID, ThreadID: in.ThreadID, TagID: in.GroupID,
		InitiatorKind: kind, InitiatorID: id, IdempotencyKey: in.IdempotencyKey,
		Status: in.Status, RouteJSON: marshalGroupRoute(groupRouteEnvelope{
			Version: 3, UserText: in.UserText, Mentions: in.Mentions,
			MentionedHuman: in.MentionedHuman, RoutingPrompt: in.RoutingPrompt, History: in.History,
		}),
	})
	if err != nil {
		return group.Turn{}, false, err
	}
	out, err := s.toGroupTurn(created)
	return out, reused, err
}

func splitInitiator(actor string) (string, string) {
	kind, id, ok := strings.Cut(actor, ":")
	if ok {
		return kind, id
	}
	return "", actor
}

func marshalGroupRoute(route groupRouteEnvelope) string {
	if route.Version == 0 {
		route.Version = 3
	}
	raw, err := json.Marshal(route)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func parseGroupRoute(raw string) (groupRouteEnvelope, error) {
	route := groupRouteEnvelope{Version: 3}
	if strings.TrimSpace(raw) != "" && strings.TrimSpace(raw) != "{}" {
		if err := json.Unmarshal([]byte(raw), &route); err != nil {
			return groupRouteEnvelope{}, err
		}
	}
	return route, nil
}

func (s *collabGroupStore) toGroupTurn(in systemdb.AgentTurn) (group.Turn, error) {
	route, err := parseGroupRoute(in.RouteJSON)
	if err != nil {
		return group.Turn{}, err
	}
	return group.Turn{
		ID: in.ID, ThreadID: in.ThreadID, GroupID: in.TagID, Status: in.Status,
		Version: int(in.Version), Initiator: joinInitiator(in.InitiatorKind, in.InitiatorID),
		IdempotencyKey: in.IdempotencyKey, UserText: route.UserText, Mentions: route.Mentions,
		MentionedHuman: route.MentionedHuman, History: route.History, RoutingPrompt: route.RoutingPrompt,
		Decision: route.Decision, ConfirmState: route.ConfirmState, Replans: route.Replans,
		Summarized: route.Summarized, CreatedAt: in.CreatedAt, UpdatedAt: in.UpdatedAt,
	}, nil
}

func joinInitiator(kind, id string) string {
	if kind == "" {
		return id
	}
	return kind + ":" + id
}

func (s *collabGroupStore) TransitionTurn(ctx context.Context, turnID, from, to, errorCode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.store.GetAgentTurn(ctx, s.projectID, turnID)
	if err != nil {
		return mapGroupStoreError(err)
	}
	if current.Status != from {
		return group.ErrIllegalTransition
	}
	if err := s.store.UpdateAgentTurnStatus(ctx, s.projectID, turnID, to, "", errorCode, current.Version); err != nil {
		return mapGroupStoreError(err)
	}
	if errorCode == "" && to == group.TurnRunning {
		return s.store.ClearAgentTurnError(ctx, s.projectID, turnID)
	}
	return nil
}

func (s *collabGroupStore) SaveDecision(ctx context.Context, turnID string, decision plan.Decision, confirmState string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updateRoute(ctx, turnID, func(route *groupRouteEnvelope) {
		route.Decision = decision
		route.ConfirmState = confirmState
	})
}

func (s *collabGroupStore) SaveReplans(ctx context.Context, turnID string, replans int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updateRoute(ctx, turnID, func(route *groupRouteEnvelope) {
		route.Replans = replans
	})
}

// updateRoute 的调用方持有 s.mu。
func (s *collabGroupStore) updateRoute(ctx context.Context, turnID string, update func(*groupRouteEnvelope)) error {
	current, err := s.store.GetAgentTurn(ctx, s.projectID, turnID)
	if err != nil {
		return mapGroupStoreError(err)
	}
	route, err := parseGroupRoute(current.RouteJSON)
	if err != nil {
		return err
	}
	update(&route)
	if err := s.store.UpdateAgentTurnStatus(ctx, s.projectID, turnID, current.Status,
		marshalGroupRoute(route), "", current.Version); err != nil {
		return mapGroupStoreError(err)
	}
	return nil
}

func (s *collabGroupStore) LoadTurn(ctx context.Context, turnID string) (group.Turn, bool, error) {
	turn, err := s.store.GetAgentTurn(ctx, s.projectID, turnID)
	if errors.Is(err, systemdb.ErrTurnNotFound) {
		return group.Turn{}, false, nil
	}
	if err != nil {
		return group.Turn{}, false, err
	}
	out, err := s.toGroupTurn(turn)
	return out, err == nil, err
}

func (s *collabGroupStore) CreateTasks(ctx context.Context, turnID string, tasks []group.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	turn, err := s.store.GetAgentTurn(ctx, s.projectID, turnID)
	if err != nil {
		return mapGroupStoreError(err)
	}
	route, err := parseGroupRoute(turn.RouteJSON)
	if err != nil {
		return err
	}
	rows := make([]systemdb.AgentTask, 0, len(tasks))
	for i, task := range tasks {
		rows = append(rows, toSystemTask(s.projectID, turn.ThreadID, task, i+1))
		if task.Status != group.TaskCompleted && route.Results != nil {
			delete(route.Results, task.ID)
		}
	}
	if err := s.store.ReplaceAgentTasks(ctx, s.projectID, turnID, rows); err != nil {
		return err
	}
	return s.store.UpdateAgentTurnStatus(ctx, s.projectID, turnID, turn.Status,
		marshalGroupRoute(route), "", turn.Version)
}

func toSystemTask(projectID, threadID string, task group.Task, seq int) systemdb.AgentTask {
	depends := make([]string, 0, len(task.DependsOn))
	for _, ref := range task.DependsOn {
		depends = append(depends, task.TurnID+"-"+ref)
	}
	assignee := ""
	if task.TargetKind == plan.TargetHuman {
		assignee = task.TargetID
	}
	return systemdb.AgentTask{
		ID: task.ID, ProjectID: projectID, TurnID: task.TurnID, ThreadID: threadID,
		Seq: int64(seq), TargetKind: task.TargetKind, TargetID: task.TargetID,
		Instruction: task.Message, DependsJSON: systemdb.MarshalAgentTaskDepends(depends),
		Status: task.Status, Attempt: int64(task.Attempt), RunID: task.RunID,
		AssigneeID: assignee, ErrorCode: task.ErrorCode, Version: int64(task.Version),
	}
}

func (s *collabGroupStore) TransitionTask(ctx context.Context, taskID, from, to, result, errorCode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.store.GetAgentTask(ctx, s.projectID, taskID)
	if err != nil {
		return mapGroupStoreError(err)
	}
	if current.Status != from {
		return group.ErrIllegalTransition
	}
	current.Status = to
	current.ErrorCode = errorCode
	if _, err := s.store.UpdateAgentTask(ctx, current, current.Version); err != nil {
		return mapGroupStoreError(err)
	}
	return s.updateRoute(ctx, current.TurnID, func(route *groupRouteEnvelope) {
		if route.Results == nil {
			route.Results = map[string]string{}
		}
		route.Results[taskID] = result
	})
}

func (s *collabGroupStore) SetTaskRunID(ctx context.Context, taskID, runID string) error {
	current, err := s.store.GetAgentTask(ctx, s.projectID, taskID)
	if err != nil {
		return mapGroupStoreError(err)
	}
	current.RunID = runID
	_, err = s.store.UpdateAgentTask(ctx, current, current.Version)
	return mapGroupStoreError(err)
}

func (s *collabGroupStore) LoadTasks(ctx context.Context, turnID string) ([]group.Task, error) {
	turn, err := s.store.GetAgentTurn(ctx, s.projectID, turnID)
	if err != nil {
		return nil, mapGroupStoreError(err)
	}
	route, err := parseGroupRoute(turn.RouteJSON)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.ListAgentTasks(ctx, s.projectID, turnID)
	if err != nil {
		return nil, err
	}
	dispatches := make(map[string]plan.Dispatch, len(route.Decision.Dispatches))
	for _, dispatch := range route.Decision.Dispatches {
		dispatches[dispatch.Ref] = dispatch
	}
	out := make([]group.Task, 0, len(rows))
	for _, row := range rows {
		ref := strings.TrimPrefix(row.ID, turnID+"-")
		task := group.Task{
			ID: row.ID, TurnID: row.TurnID, Ref: ref, TargetKind: row.TargetKind,
			TargetID: row.TargetID, Message: row.Instruction, Status: row.Status,
			Version: int(row.Version), Attempt: int(row.Attempt), RunID: row.RunID,
			Result: route.Results[row.ID], ErrorCode: row.ErrorCode,
		}
		if dispatch, ok := dispatches[ref]; ok {
			task.DependsOn = dispatch.DependsOn
			task.ContextFrom = dispatch.ContextFrom
			task.Risk = dispatch.Risk
			task.StepID = dispatch.StepID
			task.ExpectedOutput = dispatch.ExpectedOutput
		} else {
			for _, id := range systemdb.UnmarshalAgentTaskDepends(row.DependsJSON) {
				task.DependsOn = append(task.DependsOn, strings.TrimPrefix(id, turnID+"-"))
			}
		}
		if task.Result == "" && row.TargetKind == systemdb.TaskTargetHuman {
			task.Result = row.ReplyText
		}
		out = append(out, task)
	}
	return out, nil
}

func (s *collabGroupStore) AppendEvent(ctx context.Context, turnID string, event group.Event) (group.Event, error) {
	event.TurnID = turnID
	raw, err := json.Marshal(event)
	if err != nil {
		return group.Event{}, err
	}
	stored, err := s.store.AppendAgentTurnEventAtomic(ctx, systemdb.AgentTurnEvent{
		ProjectID: s.projectID, TurnID: turnID, Kind: event.Type,
		EntityID: event.TaskID, PayloadJSON: string(raw),
	})
	if err != nil {
		return group.Event{}, err
	}
	event.Seq = int(stored.Seq)
	return event, nil
}

func (s *collabGroupStore) ListEvents(ctx context.Context, turnID string, after int) ([]group.Event, error) {
	var out []group.Event
	for {
		rows, err := s.store.ListAgentTurnEvents(ctx, s.projectID, turnID, int64(after), 500)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			event := group.Event{}
			if err := json.Unmarshal([]byte(row.PayloadJSON), &event); err != nil {
				return nil, err
			}
			if event.Type == "" {
				event.Type = row.Kind
			}
			event.Seq = int(row.Seq)
			event.TurnID = row.TurnID
			if event.TaskID == "" {
				event.TaskID = row.EntityID
			}
			out = append(out, event)
		}
		if len(rows) < 500 {
			return out, nil
		}
		after = int(rows[len(rows)-1].Seq)
	}
}

func (s *collabGroupStore) Claim(ctx context.Context, threadID, turnID string) error {
	if s.claimTurn != nil {
		return s.claimTurn(ctx, s.projectID, threadID, turnID)
	}
	if err := s.store.ClaimAgentTurn(ctx, s.projectID, threadID, turnID); err != nil {
		if errors.Is(err, systemdb.ErrTurnBusy) {
			return group.ErrTurnBusy
		}
		return err
	}
	return nil
}

func (s *collabGroupStore) Release(ctx context.Context, threadID, turnID string) error {
	return s.store.ReleaseAgentTurn(ctx, s.projectID, threadID, turnID)
}

func (s *collabGroupStore) ListUnfinishedTurns(ctx context.Context) ([]group.Turn, error) {
	rows, err := s.store.ListRunningAgentTurns(ctx, time.Now().UTC().Add(time.Minute))
	if err != nil {
		return nil, err
	}
	out := make([]group.Turn, 0, len(rows))
	for _, row := range rows {
		if row.ProjectID != s.projectID {
			continue
		}
		turn, err := s.toGroupTurn(row)
		if err != nil {
			return nil, err
		}
		out = append(out, turn)
	}
	return out, nil
}

func mapGroupStoreError(err error) error {
	if errors.Is(err, systemdb.ErrTurnConflict) {
		return group.ErrIllegalTransition
	}
	return err
}
