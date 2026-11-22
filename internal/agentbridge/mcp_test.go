package agentbridge

import (
	"context"
	"database/sql"
	"net"
	"testing"

	_ "github.com/uglyer/go-sqlite3"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// newMCPTestStore 构造带完整 schema 的内存库。
func newMCPTestStore(t *testing.T) *systemdb.Store {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := systemdb.EnsureSystemSchema(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return systemdb.NewStoreForTest(db)
}

func mcpRequest(projectID string) cloudagent.RunRequest {
	return cloudagent.RunRequest{
		Agent:   cloudagent.Agent{Writable: true},
		Context: RunContext{ProjectID: projectID, Principal: auth.Principal{UserID: "u1"}},
	}
}

// SSRF 防护的核心：这些地址一律拒绝（planv6.2 §4.2-5）。
func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1",       // 回环
		"::1",             // IPv6 回环
		"10.0.0.1",        // 私网 A
		"172.16.0.1",      // 私网 B
		"192.168.1.1",     // 私网 C
		"169.254.169.254", // 云元数据（链路本地）
		"fe80::1",         // IPv6 链路本地
		"0.0.0.0",         // 未指定
		"::",              // IPv6 未指定
		"224.0.0.1",       // 组播
	}
	for _, s := range blocked {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("bad test ip %q", s)
		}
		if !isBlockedIP(ip) {
			t.Fatalf("%s should be blocked", s)
		}
	}

	allowed := []string{"8.8.8.8", "1.1.1.1", "2606:4700::1111"}
	for _, s := range allowed {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("bad test ip %q", s)
		}
		if isBlockedIP(ip) {
			t.Fatalf("%s should be allowed", s)
		}
	}
	if !isBlockedIP(nil) {
		t.Fatal("nil ip should be blocked")
	}
}

func TestMCPConnectorRejectsHTTPByDefault(t *testing.T) {
	c := NewMCPConnector(nil, MCPOptions{})
	if _, err := c.httpClient(systemdb.MCPServer{URL: "http://example.com/mcp"}); err == nil {
		t.Fatal("http should be rejected unless allow_http is set")
	}
	// 显式开启后放行。
	c = NewMCPConnector(nil, MCPOptions{AllowHTTP: true})
	if _, err := c.httpClient(systemdb.MCPServer{URL: "http://example.com/mcp"}); err != nil {
		t.Fatalf("http allowed err=%v", err)
	}
}

func TestMCPConnectorRejectsBadURLs(t *testing.T) {
	c := NewMCPConnector(nil, MCPOptions{})
	for _, raw := range []string{"", "ftp://example.com/mcp", "://bad"} {
		if _, err := c.httpClient(systemdb.MCPServer{URL: raw}); err == nil {
			t.Fatalf("%q should be rejected", raw)
		}
	}
}

func TestMCPConnectorInjectsHeadersWithoutLeaking(t *testing.T) {
	c := NewMCPConnector(nil, MCPOptions{})
	client, err := c.httpClient(systemdb.MCPServer{
		URL:         "https://mcp.example.com/mcp",
		HeadersJSON: `{"Authorization":"Bearer secret"}`,
		TimeoutS:    30,
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, ok := client.Transport.(*headerRoundTripper)
	if !ok {
		t.Fatalf("transport=%T", client.Transport)
	}
	if rt.headers["Authorization"] != "Bearer secret" {
		t.Fatalf("headers=%v", rt.headers)
	}
	// 禁止跟随重定向：SSRF 的常见绕过手段。
	if client.CheckRedirect == nil {
		t.Fatal("CheckRedirect must be set")
	}
}

func TestMCPConnectorEmptyStoreReturnsEmptySet(t *testing.T) {
	c := NewMCPConnector(nil, MCPOptions{})
	set, err := c.Connect(context.Background(), mcpRequest("p1"), []string{"github"})
	if err != nil {
		t.Fatal(err)
	}
	if set == nil || len(set.Sessions()) != 0 {
		t.Fatalf("set=%v", set)
	}
}

func TestMCPPolicyFailsClosedOnUnknownServer(t *testing.T) {
	store := newMCPTestStore(t)
	p := NewMCPPolicy(store)
	// 未登记的 Server：策略缺失，必须报错（调用方随后拒绝执行）。
	if _, err := p.Classify(context.Background(), mcpRequest("p1"), "missing", "tool"); err == nil {
		t.Fatal("unknown server should error")
	}
	if _, err := NewMCPPolicy(nil).Classify(context.Background(), mcpRequest("p1"), "s", "t"); err == nil {
		t.Fatal("nil store should error")
	}
}

// 未标记的工具按「写」处理，标记过的按标记。
func TestMCPPolicyUnmarkedToolIsWritable(t *testing.T) {
	store := newMCPTestStore(t)
	_, err := store.CreateMCPServer(context.Background(), systemdb.MCPServer{
		ProjectID:       "p1",
		Name:            "s",
		URL:             "https://example.com/mcp",
		ToolsPolicyJSON: `{"read_me":{"read_only":true},"drop_me":{"destructive":true}}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	p := NewMCPPolicy(store)
	ctx := context.Background()

	if d, err := p.Classify(ctx, mcpRequest("p1"), "s", "read_me"); err != nil || !d.ReadOnly {
		t.Fatalf("read_me=%#v err=%v", d, err)
	}
	if d, err := p.Classify(ctx, mcpRequest("p1"), "s", "drop_me"); err != nil || !d.Destructive {
		t.Fatalf("drop_me=%#v err=%v", d, err)
	}
	// 未标记：非只读、非破坏性 → 走普通写权限校验。
	if d, err := p.Classify(ctx, mcpRequest("p1"), "s", "unknown"); err != nil || d.ReadOnly || d.Destructive {
		t.Fatalf("unknown=%#v err=%v", d, err)
	}
	// 项目隔离：别的项目查不到这个 Server。
	if _, err := p.Classify(ctx, mcpRequest("p2"), "s", "read_me"); err == nil {
		t.Fatal("cross-project policy lookup should fail")
	}
}
