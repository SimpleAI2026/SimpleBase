package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestMCPServerCRUDAndCredentialsStayServerSide(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	saved, err := store.CreateMCPServer(ctx, MCPServer{
		ProjectID:       "p1",
		Name:            "github",
		URL:             "https://mcp.example.com/mcp",
		HeadersJSON:     `{"Authorization":"Bearer secret"}`,
		ToolsPolicyJSON: `{"create_issue":{"read_only":false,"destructive":true}}`,
		Enabled:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.TimeoutS != 60 {
		t.Fatalf("default timeout=%d want 60", saved.TimeoutS)
	}

	got, err := store.GetMCPServerByName(ctx, "p1", "github")
	if err != nil {
		t.Fatal(err)
	}
	// 凭据只在存储层可见，由 handler 决定不回显。
	if ParseMCPHeaders(got.HeadersJSON)["Authorization"] != "Bearer secret" {
		t.Fatalf("headers=%q", got.HeadersJSON)
	}
	policies := ParseMCPToolPolicies(got.ToolsPolicyJSON)
	if !policies["create_issue"].Destructive {
		t.Fatalf("policies=%v", policies)
	}

	// 项目隔离：其他项目取不到。
	if _, err := store.GetMCPServerByName(ctx, "p2", "github"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-project err=%v want ErrNoRows", err)
	}

	// 同项目不同 Server 各自独立。
	if _, err := store.CreateMCPServer(ctx, MCPServer{ProjectID: "p1", Name: "fs", URL: "https://fs.example.com/mcp"}); err != nil {
		t.Fatal(err)
	}
	n, err := store.CountMCPServers(ctx, "p1")
	if err != nil || n != 2 {
		t.Fatalf("count=%d err=%v want 2", n, err)
	}

	// 更新后重新读取。
	got.Enabled = false
	got.TimeoutS = 30
	if _, err := store.UpdateMCPServer(ctx, got); err != nil {
		t.Fatal(err)
	}
	again, err := store.GetMCPServer(ctx, "p1", got.ID)
	if err != nil || again.Enabled || again.TimeoutS != 30 {
		t.Fatalf("updated=%#v err=%v", again, err)
	}

	// 软删后列表与计数都排除它。
	if err := store.ArchiveMCPServer(ctx, "p1", got.ID); err != nil {
		t.Fatal(err)
	}
	list, err := store.ListMCPServers(ctx, "p1")
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%d err=%v want 1", len(list), err)
	}
	// 重复删除是 ErrNoRows。
	if err := store.ArchiveMCPServer(ctx, "p1", got.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("second archive err=%v want ErrNoRows", err)
	}
}

func TestParseMCPToolPoliciesFailClosed(t *testing.T) {
	// 空串与非法 JSON 都回空表：调用方按「未标记 = 写」处理。
	if got := ParseMCPToolPolicies(""); len(got) != 0 {
		t.Fatalf("empty=%v", got)
	}
	if got := ParseMCPToolPolicies("not json"); len(got) != 0 {
		t.Fatalf("invalid=%v", got)
	}
	if got := ParseMCPHeaders(""); len(got) != 0 {
		t.Fatalf("headers=%v", got)
	}
}
