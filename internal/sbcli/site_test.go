// 文件说明：site_test.go — internal/sbcli 的单元测试（TestSiteCommands、TestSitePublishPrefixGuard 等）
//
// 用法
//
//	go test ./internal/sbcli -run 'TestSiteCommands|TestSitePublishPrefixGuard'
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/agentbridge
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/sbcli/site_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] cli_test.go, collection_transfer.go, collection_transfer_test.go, commands.go, commands_admin.go 等 27 个
//
// 主要符号
//
//	siteTestEnv(url string) []string
//	TestSiteCommands(t *testing.T)
//	TestSitePublishPrefixGuard(t *testing.T)

package sbcli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func siteTestEnv(url string) []string {
	return []string{"HOME=/nonexistent", "SIMPLEBASE_URL=" + url, "SIMPLEBASE_TOKEN=t", "SIMPLEBASE_PROJECT_ID=p1"}
}

func TestSiteCommands(t *testing.T) {
	stub := &cliStub{}
	srv := httptest.NewServer(stub)
	defer srv.Close()
	env := siteTestEnv(srv.URL)

	code, out, errb := runCLI(t, env, []string{"site", "status"}, "")
	if code != 0 {
		t.Fatalf("status: %d %s %s", code, out, errb)
	}
	hits := stub.all()
	if len(hits) != 1 || hits[0].Method != http.MethodGet || !strings.HasSuffix(hits[0].Path, "/v1/projects/p1/s3/site") {
		t.Fatalf("status hits=%+v", hits)
	}

	stub.reset()
	if code, out, errb = runCLI(t, env, []string{"site", "publish", "--prefix", "web/"}, ""); code != 0 {
		t.Fatalf("publish: %d %s %s", code, out, errb)
	}
	hits = stub.all()
	if len(hits) != 1 || hits[0].Method != http.MethodPut || !strings.HasSuffix(hits[0].Path, "/s3/public") ||
		!strings.Contains(hits[0].Body, `"key":"web/"`) || !strings.Contains(hits[0].Body, `"kind":"folder"`) || !strings.Contains(hits[0].Body, `"public":true`) {
		t.Fatalf("publish hits=%+v", hits)
	}

	stub.reset()
	if code, out, errb = runCLI(t, env, []string{"site", "enable"}, ""); code != 0 {
		t.Fatalf("enable: %d %s %s", code, out, errb)
	}
	hits = stub.all()
	if len(hits) != 1 || hits[0].Method != http.MethodPut || !strings.HasSuffix(hits[0].Path, "/s3/public") ||
		!strings.Contains(hits[0].Body, `"key":"web/"`) || !strings.Contains(hits[0].Body, `"kind":"folder"`) || !strings.Contains(hits[0].Body, `"public":true`) {
		t.Fatalf("enable hits=%+v", hits)
	}

	stub.reset()
	if code, out, errb = runCLI(t, env, []string{"site", "disable"}, ""); code != 0 {
		t.Fatalf("disable: %d %s %s", code, out, errb)
	}
	hits = stub.all()
	if len(hits) != 1 || !strings.Contains(hits[0].Body, `"key":"web/"`) || !strings.Contains(hits[0].Body, `"public":false`) {
		t.Fatalf("disable hits=%+v", hits)
	}

	stub.reset()
	if code, out, errb = runCLI(t, env, []string{"object", "public", "--key", "web/app.js", "--on"}, ""); code != 0 {
		t.Fatalf("object public: %d %s %s", code, out, errb)
	}
	hits = stub.all()
	if len(hits) != 1 || !strings.HasSuffix(hits[0].Path, "/s3/public") ||
		!strings.Contains(hits[0].Body, `"key":"web/app.js"`) || !strings.Contains(hits[0].Body, `"kind":"file"`) || !strings.Contains(hits[0].Body, `"public":true`) {
		t.Fatalf("object public hits=%+v", hits)
	}
}

// 助手只能公开 web/：缺省、空串、根、其它前缀都在发请求前被拒绝。
func TestSitePublishPrefixGuard(t *testing.T) {
	stub := &cliStub{}
	srv := httptest.NewServer(stub)
	defer srv.Close()
	env := siteTestEnv(srv.URL)

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"site", "publish"}, "site_root_publish_forbidden"},
		{[]string{"site", "publish", "--prefix", ""}, "site_root_publish_forbidden"},
		{[]string{"site", "publish", "--prefix", "/"}, "site_root_publish_forbidden"},
		{[]string{"site", "publish", "--prefix", "private/"}, "site_prefix_not_allowed"},
		{[]string{"site", "publish", "--prefix", "web"}, "site_prefix_not_allowed"},
		{[]string{"site", "publish", "--prefix", "web/../"}, "site_prefix_not_allowed"},
	}
	for _, tc := range cases {
		code, out, errb := runCLI(t, env, tc.args, "")
		if code == 0 || !strings.Contains(out+errb, tc.want) {
			t.Errorf("%v: code=%d out=%s err=%s", tc.args, code, out, errb)
		}
	}
	if hits := stub.all(); len(hits) != 0 {
		t.Fatalf("rejected publish must not hit the server: %+v", hits)
	}

	badPublic := []struct {
		args []string
		want string
	}{
		{[]string{"object", "public"}, "--key is required"},
		{[]string{"object", "public", "--key", "a.txt"}, "use exactly one of --on or --off"},
		{[]string{"object", "public", "--key", "a.txt", "--on", "--off"}, "use exactly one of --on or --off"},
		{[]string{"object", "public", "--key", "web", "--kind", "folder", "--on"}, "folder key must end with /"},
	}
	for _, tc := range badPublic {
		code, out, errb := runCLI(t, env, tc.args, "")
		if code == 0 || !strings.Contains(out+errb, tc.want) {
			t.Errorf("%v: code=%d out=%s err=%s", tc.args, code, out, errb)
		}
	}
	if hits := stub.all(); len(hits) != 0 {
		t.Fatalf("rejected public must not hit the server: %+v", hits)
	}
}
