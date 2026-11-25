// 文件说明：audit_helper_test.go — internal/api 的单元测试（TestRecordAuditActor、TestRecordAuditNilService 等）
//
// 用法
//
//	go test ./internal/api -run 'TestRecordAuditActor|TestRecordAuditNilService|TestRecordAuditSurvivesClientCancel'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/audit_helper_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	type ctxCheckAudit struct{…}
//	(a *ctxCheckAudit) Record(ctx context.Context, _ AuditEvent) error
//	(a *ctxCheckAudit) ListOperations(context.Context, string, string, int) ([]catalog.Operation, error)
//	cancelledRequest(r *http.Request) (*http.Request, context.CancelFunc)
//	auditHelperCtx(p *auth.Principal) echo.Context
//	TestRecordAuditActor(t *testing.T)
//	TestRecordAuditNilService(t *testing.T)
//	TestRecordAuditSurvivesClientCancel(t *testing.T)

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

type ctxCheckAudit struct {
	called bool
	ctxErr error
}

func (a *ctxCheckAudit) Record(ctx context.Context, _ AuditEvent) error {
	a.called, a.ctxErr = true, ctx.Err()
	return nil
}

func (a *ctxCheckAudit) ListOperations(context.Context, string, string, int) ([]catalog.Operation, error) {
	return nil, nil
}

func cancelledRequest(r *http.Request) (*http.Request, context.CancelFunc) {
	ctx, cancel := context.WithCancel(r.Context())
	return r.WithContext(ctx), cancel
}

func auditHelperCtx(p *auth.Principal) echo.Context {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	if p != nil {
		req = req.WithContext(WithPrincipal(req.Context(), *p))
	}
	req = req.WithContext(WithRequestID(req.Context(), "rid-1"))
	return echo.New().NewContext(req, httptest.NewRecorder())
}

// Bug-1：登录用户（只有 UserID）写审计时 PrincipalID 不能为空，
// 否则 audit.Service 会把用户操作记成 system。
func TestRecordAuditActor(t *testing.T) {
	cases := []struct {
		name string
		p    *auth.Principal
		want string
	}{
		{"login user", &auth.Principal{UserID: "u1"}, "user:u1"},
		{"api key", &auth.Principal{APIKeyID: "k1"}, "key:k1"},
		{"user wins", &auth.Principal{UserID: "u1", APIKeyID: "k1"}, "user:u1"},
		{"no principal", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			aud := &fakeAudit{}
			recordAudit(auditHelperCtx(tc.p), aud, "k", "p1", "ok", "d")
			got := aud.last
			if got.PrincipalID != tc.want || got.Kind != "k" || got.ProjectID != "p1" ||
				got.Status != "ok" || got.Detail != "d" || got.RequestID != "rid-1" {
				t.Fatalf("event=%+v want actor %q", got, tc.want)
			}
		})
	}
}

func TestRecordAuditNilService(t *testing.T) {
	recordAudit(auditHelperCtx(nil), nil, "k", "p1", "ok", "")
}

func TestRecordAuditSurvivesClientCancel(t *testing.T) {
	ctx := auditHelperCtx(&auth.Principal{UserID: "u1"})
	req, cancel := cancelledRequest(ctx.Request())
	cancel()
	c := echo.New().NewContext(req, httptest.NewRecorder())
	aud := &ctxCheckAudit{}
	recordAudit(c, aud, "k", "p1", "ok", "")
	if !aud.called || aud.ctxErr != nil {
		t.Fatalf("called=%v ctxErr=%v: 审计 ctx 不应随客户端断开取消", aud.called, aud.ctxErr)
	}
}
