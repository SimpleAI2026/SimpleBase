// 文件说明：context.go — internal/agentbridge 中 RunContext 等符号的实现
//
// 用法
//
//	type RunContext struct{…}  // RunContext is SimpleBase's provider-owned request context. Skills 是本轮加载的技能（由助手所属...
//	同包调用：hostContext() → cron_agent_test.go, mcp.go；requireRun() → skilltool.go, tools.go
//
// 调用架构
//
//	[上游] internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/context.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] access_db.go, access_llm.go, access_object.go, access_test.go, bridge_test.go 等 33 个
//	[关键引用] RunContext ← cron_agent.go, failclosed_test.go；hostContext ← cron_agent_test.go, mcp.go；requireRun ← skilltool.go, tools.go
//
// 主要符号
//
//	type RunContext struct{…}
//	hostContext(value any) (RunContext, error)
//	requireRun(ctx context.Context) (RunContext, error)

package agentbridge

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// RunContext is SimpleBase's provider-owned request context.
//
// Skills 是本轮加载的技能（由助手所属 module 决定）；规划工具组不在这里：它由宿主 API 层
// 按会话历史装配后经 ExtraTools 追加（planv6.8 B14：固定六阶段流水线已由 plan.Tools 取代）。
type RunContext struct {
	ProjectID string
	Principal auth.Principal
	Skills    []string

	RunID      string
	ThreadID   string
	Headless   bool
	Writable   bool
	Emit       func(cloudagent.Event)
	ExtraTools []tool.BaseTool
}

func hostContext(value any) (RunContext, error) {
	switch rc := value.(type) {
	case RunContext:
		return rc, nil
	case *RunContext:
		if rc != nil {
			return *rc, nil
		}
	}
	return RunContext{}, fmt.Errorf("simplebase agent run context missing")
}

func requireRun(ctx context.Context) (RunContext, error) {
	generic, ok := cloudagent.RunContextFrom(ctx)
	if !ok {
		return RunContext{}, fmt.Errorf("agent run context missing")
	}
	rc, err := hostContext(generic.Value)
	if err != nil || rc.ProjectID == "" {
		return RunContext{}, fmt.Errorf("agent run context missing")
	}
	rc.RunID = generic.RunID
	rc.ThreadID = generic.ThreadID
	rc.Emit = generic.Emit
	return rc, nil
}
