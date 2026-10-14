package api

import (
	"net/http"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

// SEC-04：只读主体不能取消他人的 run（破坏性操作）。
// 复现路径：修复前路由仅要求 DatabaseRead，任何同项目只读 Key/角色
// 都能终止他人正在执行的运行。
func TestAgentCancelRun_ReadOnlyPrincipalRejected(t *testing.T) {
	hs := newAgentHarness(t, nil, fakellmScriptLong())
	defer hs.cleanupRun()

	threadID := hs.createThread(t)
	runID := hs.startRunAsync(t, threadID)

	// 只读主体（仅 DatabaseRead 的 Key）取消 → 403。
	rec := hs.reqAs("POST", "/agent-runs/"+runID+"/cancel", nil, auth.Principal{
		APIKeyID: "ro-key", TenantID: catalog.ReservedTenantID,
		ProjectIDs:  map[string]struct{}{"testprj1": {}},
		Permissions: map[auth.Permission]struct{}{auth.DatabaseRead: {}},
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("read-only cancel: got %d want 403, body=%s", rec.Code, rec.Body.String())
	}

	// 登录态只读角色（admin）同样拒绝。
	rec2 := hs.reqAs("POST", "/agent-runs/"+runID+"/cancel", nil, auth.Principal{
		UserID: "u-admin", Username: "admin", Role: auth.RoleAdmin, TenantID: catalog.ReservedTenantID,
		ProjectIDs: map[string]struct{}{"testprj1": {}},
	})
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("admin-role cancel: got %d want 403, body=%s", rec2.Code, rec2.Body.String())
	}

	// 同项目写主体但非发起人、无 project:admin（planv6.0 D5）→ 403。
	rec3 := hs.reqAs("POST", "/agent-runs/"+runID+"/cancel", nil, auth.Principal{
		APIKeyID: "rw-key", TenantID: catalog.ReservedTenantID,
		ProjectIDs:  map[string]struct{}{"testprj1": {}},
		Permissions: map[auth.Permission]struct{}{auth.DatabaseWrite: {}},
	})
	if rec3.Code != http.StatusForbidden {
		t.Fatalf("non-initiator write cancel: got %d want 403 (D5), body=%s", rec3.Code, rec3.Body.String())
	}

	// project:admin 的写主体 → 通过写门禁与发起人校验（200/404/409 均正常）。
	rec4 := hs.reqAs("POST", "/agent-runs/"+runID+"/cancel", nil, auth.Principal{
		APIKeyID: "admin-key", TenantID: catalog.ReservedTenantID,
		ProjectIDs:  map[string]struct{}{"testprj1": {}},
		Permissions: map[auth.Permission]struct{}{auth.DatabaseWrite: {}, auth.ProjectAdmin: {}},
	})
	if rec4.Code == http.StatusForbidden {
		t.Fatalf("project-admin cancel must pass: body=%s", rec4.Body.String())
	}
}

// SEC-04：只读主体不能确认（放行）工具执行。
func TestAgentConfirmRun_ReadOnlyPrincipalRejected(t *testing.T) {
	hs := newAgentHarness(t, nil, fakellmScriptLong())
	defer hs.cleanupRun()

	threadID := hs.createThread(t)
	runID := hs.startRunAsync(t, threadID)

	rec := hs.reqAs("POST", "/agent-runs/"+runID+"/confirmations/call-1",
		map[string]any{"approve": true}, auth.Principal{
			APIKeyID: "ro-key", TenantID: catalog.ReservedTenantID,
			ProjectIDs:  map[string]struct{}{"testprj1": {}},
			Permissions: map[auth.Permission]struct{}{auth.DatabaseRead: {}},
		})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("read-only confirm: got %d want 403, body=%s", rec.Code, rec.Body.String())
	}
}
