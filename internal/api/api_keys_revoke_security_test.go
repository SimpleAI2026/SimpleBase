package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// SEC-01：项目 A 的主体拿到项目 B 的 Key ID 后调用吊销接口，
// 修复前 UPDATE 只按 id 匹配，对方 Key 被吊销；修复后必须 404 且对方 Key 仍能认证。
func TestAPIKeys_RevokeCrossProjectDenied(t *testing.T) {
	e, db, svc := setupAPIKeyRouter(t, superPrincipal())
	// 项目 B 的 Key（不在当前 principal 的 ProjectIDs 内）。
	otherRaw := "sb_live_crossproject0000000000000000"
	if _, err := db.Exec(
		`INSERT INTO sys_projects(id, tenant_id, name, database_path, created_at) VALUES(?, ?, ?, '', ?)`,
		"proj-2", "tenant-1", "p2", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := auth.CreateAPIKey(context.Background(), db, "key-other", "proj-2",
		svc.HashKey(otherRaw), []auth.Permission{auth.DatabaseRead}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	// 修复前：能认证。
	if _, err := svc.Authenticate(context.Background(), otherRaw); err != nil {
		t.Fatalf("pre: %v", err)
	}
	// 用项目 A 的路由吊销项目 B 的 Key。
	rec := doRequest(e, http.MethodDelete, "/v1/projects/proj-1/api-keys/key-other", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-project revoke, got %d: %s", rec.Code, rec.Body.String())
	}
	// 对方 Key 必须仍然有效。
	if _, err := svc.Authenticate(context.Background(), otherRaw); err != nil {
		t.Fatalf("cross-project revoke must not invalidate victim key: %v", err)
	}
}

// SEC-01：不存在的 Key ID 修复前返回 204，修复后必须 404。
func TestAPIKeys_RevokeMissingKeyNotFound(t *testing.T) {
	e, _, _ := setupAPIKeyRouter(t, superPrincipal())
	rec := doRequest(e, http.MethodDelete, "/v1/projects/proj-1/api-keys/no-such-key", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// SEC-01：吊销其他项目的 Key 后，该项目自己的 Key 不受影响（回归保护）。
func TestAPIKeys_RevokeOwnKeyStillWorks(t *testing.T) {
	e, db, svc := setupAPIKeyRouter(t, superPrincipal())
	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/api-keys", map[string]any{})
	var created struct {
		ID     string `json:"id"`
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	// 再造一个项目 B 的 Key，确保按 project 条件不会误伤。
	if _, err := db.Exec(
		`INSERT INTO sys_projects(id, tenant_id, name, database_path, created_at) VALUES(?, ?, ?, '', ?)`,
		"proj-2", "tenant-1", "p2", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	_ = auth.CreateAPIKey(context.Background(), db, "key-b", "proj-2",
		svc.HashKey("sb_live_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), []auth.Permission{auth.DatabaseRead}, time.Now().UTC())

	del := doRequest(e, http.MethodDelete, "/v1/projects/proj-1/api-keys/"+created.ID, nil)
	if del.Code != http.StatusNoContent {
		t.Fatalf("own revoke expected 204, got %d: %s", del.Code, del.Body.String())
	}
	if _, err := svc.Authenticate(context.Background(), "sb_live_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"); err != nil {
		t.Fatalf("other project key must survive: %v", err)
	}
	// 重复吊销：已吊销的 Key 再次吊销按 404 处理（幂等语义：记录仍存在但已失效）。
	again := doRequest(e, http.MethodDelete, "/v1/projects/proj-1/api-keys/"+created.ID, nil)
	if again.Code != http.StatusNotFound {
		t.Fatalf("re-revoke expected 404, got %d: %s", again.Code, again.Body.String())
	}
}
