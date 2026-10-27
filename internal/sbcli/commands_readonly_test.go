package sbcli

import (
	"strings"
	"testing"
)

// planv6.0 v2 §3.4/§3.8：CommandReadOnly 是助手写权限分级的事实来源，
// 必须覆盖 dispatch 的每一个命令，防止新增命令漏标后被 fail-closed 静默拒绝
// （或反过来被误判为只读）。

// 逐个列出所有可分发的命令，并声明期望的只读性。
// 新增命令必须同时补进 dispatch 与本表，否则对账失败。
var commandReadOnlyExpectation = map[string]bool{
	"database list":         true,
	"database get":          true,
	"database create":       false,
	"database delete":       false,
	"sql query":             true,
	"sql exec":              false,
	"sql batch":             false,
	"schema show":           true,
	"schema rows":           true,
	"schema create-table":   false,
	"schema add-column":     false,
	"collection list":       true,
	"collection create":     false,
	"collection truncate":   false,
	"collection schema-get": true,
	"collection schema-put": false,
	"document list":         true,
	"document get":          true,
	"document import":       false,
	"document export":       true,
	"document insert":       false,
	"document update":       false,
	"document patch":        false,
	"document batch":        false,
	"document delete":       false,
	"kv exec":               false,
	"object list":           true,
	"object upload":         false,
	"object delete":         false,
	"object presign":        true,
	"function list":         true,
	"function get":          true,
	"function create":       false,
	"function update":       false,
	"function delete":       false,
	"function versions":     false, // 含 create/activate，按写处理；子动作另有细分
	"function test":         true,
	"function invoke":       false,
	"function activate":     false,
	"cron list":             true,
	"cron get":              true,
	"cron create":           false,
	"cron update":           false,
	"cron delete":           false,
	"cron runs":             true,
	"cron trigger":          false,
	"sandbox capabilities":  true,
	"sandbox list":          true,
	"sandbox get":           true,
	"sandbox create":        false,
	"sandbox delete":        false,
	"sandbox connect":       false,
	"sandbox pause":         false,
	"log search":            true,
	"log retention":         false, // get/set 双形态，按写处理；子动作另有细分
	"user list":             true,
	"user get":              true,
	"user create":           false,
	"user update":           false,
	"user disable":          false,
	"user delete":           false,
	"project list":          true,
	"project create":        false,
	"apikey list":           true,
	"apikey create":         false,
	"apikey revoke":         false,
	"settings get":          true,
	"settings set":          false,
	"quota get":             true,
	"audit list":            true,
	"backup list":           true,
	"backup create":         false,
	"backup restore":        false,
	"backup delete":         false,
	"migrate status":        true,
	"migrate plan":          true,
}

func TestCommandReadOnlyReconcilesDispatch(t *testing.T) {
	// 反向对账：每个期望项都必须是可分发的命令。
	for key := range commandReadOnlyExpectation {
		parts := strings.SplitN(key, " ", 2)
		if len(parts) != 2 {
			t.Fatalf("bad expectation key %q", key)
		}
		if !DispatchKnown(parts[0], parts[1]) {
			t.Fatalf("expectation %q is not a dispatchable command", key)
		}
	}
	// 正向对账：CommandReadOnly 与期望表逐项一致。
	for key, want := range commandReadOnlyExpectation {
		parts := strings.SplitN(key, " ", 2)
		if got := CommandReadOnly(parts[0], parts[1]); got != want {
			t.Fatalf("CommandReadOnly(%q)=%v want %v", key, got, want)
		}
	}
}

func TestCommandReadOnlyFailClosed(t *testing.T) {
	if CommandReadOnly("nope", "nope") {
		t.Fatal("unknown command must be write (fail-closed)")
	}
	if CommandReadOnlyWithArgv(nil) {
		t.Fatal("empty argv must be write")
	}
}

func TestCommandReadOnlyWithArgvSubActions(t *testing.T) {
	cases := []struct {
		argv []string
		want bool
	}{
		{[]string{"log", "retention", "get"}, true},
		{[]string{"log", "retention", "set"}, false},
		{[]string{"log", "retention"}, false},
		{[]string{"function", "versions", "list"}, true},
		{[]string{"function", "versions", "get"}, true},
		{[]string{"function", "versions", "create"}, false},
		{[]string{"function", "versions", "activate"}, false},
		{[]string{"sandbox", "files", "list"}, false}, // 命令已删除，未知命令 fail-closed
		{[]string{"sandbox", "exec"}, false},
		{[]string{"sql", "query"}, true},
		{[]string{"sql", "exec"}, false},
		{[]string{"database"}, false},
	}
	for _, c := range cases {
		if got := CommandReadOnlyWithArgv(c.argv); got != c.want {
			t.Fatalf("CommandReadOnlyWithArgv(%v)=%v want %v", c.argv, got, c.want)
		}
	}
}
