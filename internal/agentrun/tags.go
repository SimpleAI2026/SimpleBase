// 文件说明：tags.go — 工作台标签（群）的领域默认值与解析
//
// 用法
//
//	agentrun.BuiltinAgentTagDefaults()                                  // 预置群的内置默认配置
//	agentrun.BuiltinAgentTagName(key)                                   // 预置群的固定显示名
//	agentrun.GeneralAgentTag(projectID, agents)                         // 未保存过配置时的 #通用 默认值
//	(s *Service) ResolveAgentTag(ctx, projectID, id)                    // 按 ID 解析：预置群无覆盖行时回退内置默认
//
// 调用架构
//
//	[上游] internal/api（标签 / 会话 / 群 / 定时任务 handler）、internal/agentrun（群引擎、群定时）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentrun/tags.go
//	         │ 依赖
//	         ▼
//	[下游] internal/systemdb（行类型与预置键）
//	[同包] group_engine.go, cron.go
//
// 主要符号
//
//	BuiltinAgentTagDefaults() map[string]systemdb.AgentTagConfig
//	BuiltinAgentTagName(key string) string
//	GeneralAgentTag(projectID string, agents []systemdb.CloudAgent) systemdb.AgentTag
//	(s *Service) ResolveAgentTag(ctx context.Context, projectID, id string) (systemdb.AgentTag, error)
//
// 归属（internal-ddd-hexagonal-implementation-plan §3.2）：标签是 Agent 协作上下文的领域概念，
// 「预置群恒存在」的解析规则只有这一份实现，入口与群引擎共用。

package agentrun

import (
	"context"
	"database/sql"
	"errors"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// BuiltinAgentTagDefaults 返回预置群的内置默认配置（builtin_key 引用助手，
// 读取时由 ResolveAgentTag 解析为本项目助手 ID）。
func BuiltinAgentTagDefaults() map[string]systemdb.AgentTagConfig {
	return map[string]systemdb.AgentTagConfig{
		systemdb.AgentTagGeneral: {
			RoutingPrompt: "根据用户目标选择最合适的助手；需要多个模块时拆分任务并汇总结果。",
		},
		systemdb.AgentTagAlerts: {
			AgentIDs: []string{"logs", "general"},
			Prompts: []systemdb.AgentTagPrompt{
				{ID: "alerts-p1", Title: "最近 1 小时有哪些错误日志", Content: "最近 1 小时有哪些错误日志？"},
				{ID: "alerts-p2", Title: "按级别统计今天的日志", Content: "按级别统计今天的日志。"},
			},
			RoutingPrompt: "先让日志助手定位错误，再由通用助手判断影响与处置建议；" +
				"涉及线上变更或需要人工确认时交给人。",
		},
		systemdb.AgentTagStats: {
			AgentIDs: []string{"database", "kv", "general"},
			Prompts: []systemdb.AgentTagPrompt{
				{ID: "stats-p1", Title: "统计各表的行数", Content: "统计各表的行数。"},
				{ID: "stats-p2", Title: "最近 7 天新增了多少数据", Content: "最近 7 天新增了多少数据？"},
			},
			RoutingPrompt: "结构性问题交给数据库助手，KV/对象数据交给 KV 助手，最后由通用助手汇总结论。",
		},
		systemdb.AgentTagScheduled: {
			AgentIDs: []string{"general"},
			Prompts: []systemdb.AgentTagPrompt{
				{ID: "sched-p1", Title: "生成今天的运行摘要", Content: "生成今天的运行摘要，列出失败与异常。"},
			},
			RoutingPrompt: "定时任务默认交给通用助手；需要多个助手协同时再拆分，不要生成人工任务。",
			HumanPolicy:   systemdb.HumanPolicyDisabled,
		},
	}
}

// GeneralAgentTag 返回未保存过配置时的 #通用 默认值：项目全部可用助手（通用助手作回退）。
func GeneralAgentTag(projectID string, agents []systemdb.CloudAgent) systemdb.AgentTag {
	cfg := BuiltinAgentTagDefaults()[systemdb.AgentTagGeneral]
	cfg.AgentIDs = make([]string, 0, len(agents))
	for _, agent := range agents {
		cfg.AgentIDs = append(cfg.AgentIDs, agent.ID)
		if agent.BuiltinKey == systemdb.AgentTagGeneral {
			cfg.FallbackAgentID = agent.ID
		}
	}
	return systemdb.AgentTag{
		ID: systemdb.AgentTagGeneral, ProjectID: projectID, BuiltinKey: systemdb.AgentTagGeneral,
		Name: BuiltinAgentTagName(systemdb.AgentTagGeneral), Config: cfg,
	}
}

// BuiltinAgentTagName 返回预置群的固定显示名（实际本地化由前端完成）。
func BuiltinAgentTagName(key string) string {
	switch key {
	case systemdb.AgentTagGeneral:
		return "通用"
	case systemdb.AgentTagAlerts:
		return "告警"
	case systemdb.AgentTagStats:
		return "统计"
	case systemdb.AgentTagScheduled:
		return "定时"
	}
	return key
}

// ResolveAgentTag 按 ID 取标签：预置群恒存在——无覆盖行时返回内置默认
// （Persisted=false，Version=0）；自定义标签需存在且未归档。入口的标签 / 会话 / 群管理与群引擎共用。
func (s *Service) ResolveAgentTag(ctx context.Context, projectID, id string) (systemdb.AgentTag, error) {
	if id == systemdb.AgentTagGeneral {
		t, err := s.ports.Tags.GetAgentTagRow(ctx, projectID, id)
		if err == nil {
			return t, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return systemdb.AgentTag{}, err
		}
		agents, err := s.ports.Agents.ListCloudAgents(ctx, projectID)
		if err != nil {
			return systemdb.AgentTag{}, err
		}
		return GeneralAgentTag(projectID, agents), nil
	}
	if systemdb.IsBuiltinAgentTag(id) {
		if t, err := s.ports.Tags.GetAgentTagRow(ctx, projectID, id); err == nil {
			return t, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return systemdb.AgentTag{}, err
		}
		cfg := BuiltinAgentTagDefaults()[id]
		return systemdb.AgentTag{
			ID: id, ProjectID: projectID, BuiltinKey: id,
			Name: BuiltinAgentTagName(id), Config: cfg, Version: 0, Persisted: false,
		}, nil
	}
	t, err := s.ports.Tags.GetAgentTagRow(ctx, projectID, id)
	if errors.Is(err, sql.ErrNoRows) {
		return systemdb.AgentTag{}, systemdb.ErrAgentTagNotFound
	}
	return t, err
}
