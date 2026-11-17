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
	if code, out, errb = runCLI(t, env, []string{"site", "enable"}, ""); code != 0 {
		t.Fatalf("enable: %d %s %s", code, out, errb)
	}
	hits = stub.all()
	if len(hits) != 1 || hits[0].Method != http.MethodPut || !strings.Contains(hits[0].Body, `"enabled":true`) {
		t.Fatalf("enable hits=%+v", hits)
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
}
