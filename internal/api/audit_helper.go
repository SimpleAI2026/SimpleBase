// 文件说明：audit_helper.go — handler 写审计事件的统一入口
//
// 用法
//
//	recordAudit(c, svc, kind, projectID, status, detail)  // svc 为 nil 时直接返回
//
// 约定：主体统一取 auth.Principal.Actor()（user:<id> / key:<id>，无主体为空串，
// 由 audit.Service 回落为 auth.ActorSystem）；ctx 使用 WithoutCancel，客户端断开
// 不会丢审计；RequestID 取自请求上下文。
//
// 调用架构
//
//	[上游] internal/api 各 handler 的 recordAudit / record 方法
//	         │
//	         ▼
//	[本文件] internal/api/audit_helper.go
//	         │
//	         ▼
//	[下游] AuditService（router.go 定义，app 装配 audit.Service）
//
// 主要符号
//
//	func recordAudit(c echo.Context, svc AuditService, kind, projectID, status, detail string)

package api

import (
	"context"

	"github.com/labstack/echo/v4"
)

func recordAudit(c echo.Context, svc AuditService, kind, projectID, status, detail string) {
	if svc == nil {
		return
	}
	ctx := c.Request().Context()
	_ = svc.Record(context.WithoutCancel(ctx), AuditEvent{
		ProjectID:   projectID,
		PrincipalID: principalOf(c).Actor(),
		Kind:        kind,
		RequestID:   RequestIDFromContext(ctx),
		Status:      status,
		Detail:      detail,
	})
}
