package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// planv6.0 v2 §3.8：权限提升防护 —— 只读主体不能把助手设为可写。
// 测试直接走 handler 方法，避免依赖 /agents 路由的 require(...) 中间件，
// 以便单独验证 normalizeAgent 的判定。

// readonlyPrincipal 返回只读主体（只有 DatabaseRead）。
func readonlyPrincipal() auth.Principal {
	return auth.Principal{
		APIKeyID: "k-ro", TenantID: catalog.ReservedTenantID,
		Permissions: map[auth.Permission]struct{}{auth.DatabaseRead: {}},
	}
}

// writableFlag 返回 writable 开关用的指针。
func writableFlag(v bool) *bool { return &v }

func TestNormalizeAgentRejectsPrivilegeEscalation(t *testing.T) {
	store := openAgentStore(t)
	writable := true
	h := &cloudAgentHandler{store: store, writable: &writable}
	ctx := context.Background()
	if err := h.ensureAgents(ctx, "testprj1"); err != nil {
		t.Fatal(err)
	}

	yes := true
	no := false
	// 只读主体 + writable=true → 拒绝。
	if _, err := h.normalizeAgent("testprj1", upsertAgentBody{Name: "x", Module: "database", Writable: &yes}, systemdb.CloudAgent{}, readonlyPrincipal()); err == nil {
		t.Fatal("readonly principal must not enable agent writes")
	}
	// 只读主体 + writable=false → 放行（关闭不是提权）。
	if _, err := h.normalizeAgent("testprj1", upsertAgentBody{Name: "x", Module: "database", Writable: &no}, systemdb.CloudAgent{}, readonlyPrincipal()); err != nil {
		t.Fatalf("disabling writes must be allowed: %v", err)
	}
	// 只读主体 + 不传 writable → 放行（不涉及该字段）。
	if _, err := h.normalizeAgent("testprj1", upsertAgentBody{Name: "x", Module: "database"}, systemdb.CloudAgent{}, readonlyPrincipal()); err != nil {
		t.Fatalf("omitting writable must be allowed: %v", err)
	}
	// 写主体 + writable=true → 放行。
	got, err := h.normalizeAgent("testprj1", upsertAgentBody{Name: "x", Module: "database", Writable: &yes}, systemdb.CloudAgent{}, writePrincipal())
	if err != nil {
		t.Fatalf("write principal must be allowed: %v", err)
	}
	if !got.Writable {
		t.Fatal("writable flag must be applied")
	}
}

// writable 必须出现在 API 契约里（前端表单依赖它）。
func TestAgentDTOCarriesWritable(t *testing.T) {
	dto := toAgentDTO(systemdb.CloudAgent{ID: "a", Name: "n", Writable: true})
	if !dto.Writable {
		t.Fatal("agentDTO must carry writable")
	}
	raw, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back["writable"] != true {
		t.Fatalf("writable missing from JSON: %s", raw)
	}
}

// 端到端：只读主体经真实 handler 路径创建可写助手 → 400。
func TestCreateAgentPrivilegeEscalationRejected(t *testing.T) {
	store := openAgentStore(t)
	e := agentEcho(store)
	writable := true
	if err := (&cloudAgentHandler{store: store, writable: &writable}).ensureAgents(context.Background(), "testprj1"); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"name": "escalate", "module": "database", "writable": true})
	rec := agentReqAs(e, http.MethodPost, "/agents", body, readonlyPrincipal())
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for privilege escalation, got %d %s", rec.Code, rec.Body.String())
	}
}
