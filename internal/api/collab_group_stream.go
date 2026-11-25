// 文件说明：collab_group_stream.go — internal/api 的内部实现（cloudAgentHandler.streamGroupTurn、groupEventPayload 等）
//
// 用法
//
//	同包调用：cloudAgentHandler.streamGroupTurn → collab_coordinator.go, collab_query_handler.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/collab_group_stream.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//	[关键引用] cloudAgentHandler.streamGroupTurn ← collab_coordinator.go, collab_query_handler.go
//
// 主要符号
//
//	(h *cloudAgentHandler) streamGroupTurn(c echo.Context, projectID, turnID string, after int) error
//	groupEventPayload(event group.Event) map[string]any

package api

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gocloudagent/group"
)

// streamGroupTurn 把持久化/实时同形的 v3 帧写到 SSE；断开只关闭订阅。
//
// echo 的 Context 在 handler 返回后会被池化复用，因此断开监听协程只能捕获请求的 ctx，
// 并在返回前等待它退出（否则它会读到下一个请求的 Context，构成数据竞争）。
func (h *cloudAgentHandler) streamGroupTurn(c echo.Context, projectID, turnID string, after int) error {
	sub, err := h.svc.SubscribeTurn(c.Request().Context(), projectID, turnID, after)
	if err != nil {
		return WriteError(c, err)
	}
	requestCtx := c.Request().Context()
	closed := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-requestCtx.Done():
			sub.Close()
		case <-closed:
		}
	}()
	// 先停监听协程，再归还订阅：两者都只引用 sub 与 requestCtx，不再碰 echo Context。
	defer func() {
		close(closed)
		<-watcherDone
		sub.Close()
	}()
	newSSEWriter(c, "X-Turn-Id", turnID)
	flusher, _ := c.Response().Writer.(http.Flusher)
	for {
		event, ok := sub.Next()
		if !ok {
			return nil
		}
		writeSSEFrame(c.Response().Writer, flusher, int64(event.Seq), groupEventPayload(event))
	}
}

func groupEventPayload(event group.Event) map[string]any {
	payload := map[string]any{}
	var data map[string]any
	if json.Unmarshal(event.Data, &data) == nil {
		for key, value := range data {
			payload[key] = value
		}
	}
	payload["type"] = event.Type
	payload["seq"] = event.Seq
	payload["turn_id"] = event.TurnID
	if event.TaskID != "" {
		payload["task_id"] = event.TaskID
	}
	if event.RunID != "" {
		payload["run_id"] = event.RunID
	}
	if event.AgentID != "" {
		payload["agent_id"] = event.AgentID
	}
	return payload
}
