package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// apiKeyPrincipal 构造 API Key 通道的 principal（UserID 为空）。
func apiKeyPrincipal(perms ...auth.Permission) auth.Principal {
	permSet := make(map[auth.Permission]struct{}, len(perms))
	for _, p := range perms {
		permSet[p] = struct{}{}
	}
	return auth.Principal{
		APIKeyID:    "key-self",
		TenantID:    "tenant-1",
		ProjectIDs:  map[string]struct{}{"proj-1": {}},
		Permissions: permSet,
	}
}

// SEC-02：只读 Key（仅 database:read + project:admin）不得签发带写权限的新 Key。
// 修复前 parseAPIKeyPermissions 不看签发者权限，任何 ProjectAdmin Key 都能签发全量权限。
func TestAPIKeys_IssueKeyBeyondCallerPermissions(t *testing.T) {
	// 只有读权限的 ProjectAdmin Key。
	e, _, _ := setupAPIKeyRouter(t, apiKeyPrincipal(auth.DatabaseRead, auth.ProjectAdmin))
	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/api-keys",
		map[string]any{"permissions": []string{"database:read", "database:write"}})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for issuing beyond own permissions, got %d: %s", rec.Code, rec.Body.String())
	}
}

// SEC-02：签发的权限必须是签发者权限的子集；请求中超出部分导致整体拒绝。
func TestAPIKeys_IssueSubsetPermissionsSucceeds(t *testing.T) {
	e, _, _ := setupAPIKeyRouter(t, apiKeyPrincipal(auth.DatabaseRead, auth.LLMInvoke, auth.ProjectAdmin))
	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/api-keys",
		map[string]any{"permissions": []string{"database:read", "llm:invoke"}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for subset issue, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Permissions []string `json:"permissions"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Permissions) != 2 {
		t.Fatalf("perms=%v", resp.Permissions)
	}
}

// SEC-02：空权限列表签发的 Key 只能继承签发者已有的权限，不再是全量权限。
func TestAPIKeys_EmptyPermissionsInheritCallerPerms(t *testing.T) {
	e, _, svc := setupAPIKeyRouter(t, apiKeyPrincipal(auth.DatabaseRead, auth.ProjectAdmin))
	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/api-keys", map[string]any{})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		ID          string   `json:"id"`
		Secret      string   `json:"secret"`
		Permissions []string `json:"permissions"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Permissions) != 2 {
		t.Fatalf("empty list must inherit caller perms (read+admin), got %v", resp.Permissions)
	}
	// 通过真实认证链路验证权限边界。
	p, err := svc.Authenticate(context.Background(), resp.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if !p.HasPermission(auth.DatabaseRead) || p.HasPermission(auth.DatabaseWrite) {
		t.Fatalf("issued key perms wrong: read=%v write=%v",
			p.HasPermission(auth.DatabaseRead), p.HasPermission(auth.DatabaseWrite))
	}
}
