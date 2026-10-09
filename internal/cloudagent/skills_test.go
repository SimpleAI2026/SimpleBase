package cloudagent

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/auth"
)

func TestResolveAtTokenAndSkills(t *testing.T) {
	id, isAgent := ResolveAtToken("数据库", []string{"数据库"})
	if id != "database" || isAgent {
		t.Fatalf("alias wins over agent name: %s %v", id, isAgent)
	}
	id, isAgent = ResolveAtToken("Database", []string{"Database"})
	if id != "" || !isAgent {
		t.Fatalf("@Database is an agent: %s %v", id, isAgent)
	}
	id, _ = ResolveAtToken("@云函数", nil)
	if id != "gofunction" {
		t.Fatal(id)
	}
	db, err := NormalizeSkills(nil, ModuleDatabase)
	if err != nil || !reflect.DeepEqual(db, []string{"database"}) {
		t.Fatalf("module fallback %v %v", db, err)
	}
	if !AllowArgv(db, []string{"sql", "exec"}) || AllowArgv(db, []string{"function", "delete"}) {
		t.Fatal("database prefixes")
	}
	both, err := NormalizeSkills([]string{"database", "gofunction", "database"}, ModuleGeneral)
	if err != nil || !AllowArgv(both, []string{"function", "delete"}) || !AllowArgv(both, []string{"sql", "query"}) {
		t.Fatalf("union %v %v", both, err)
	}
	none, err := NormalizeSkills(nil, ModuleGeneral)
	if err != nil || len(none) != 0 || AllowArgv(none, []string{"database", "list"}) {
		t.Fatalf("general %v", none)
	}
	if _, err := NormalizeSkills([]string{"nope"}, ModuleGeneral); err == nil || !strings.Contains(err.Error(), "unknown_skill") {
		t.Fatal(err)
	}
}

func TestEmbeddedAllowsMatchPrefixes(t *testing.T) {
	allows, err := EmbeddedSkillAllows()
	if err != nil {
		t.Fatal(err)
	}
	if len(allows["index"]) != 0 {
		t.Fatalf("index allow %v", allows["index"])
	}
	for id, want := range skillPrefixes {
		got := append([]string(nil), allows[id]...)
		exp := append([]string(nil), want...)
		sort.Strings(got)
		sort.Strings(exp)
		if !reflect.DeepEqual(got, exp) {
			t.Fatalf("skill %s allow %v want %v", id, got, exp)
		}
	}
}

func TestDestructiveArgv(t *testing.T) {
	if !destructiveArgv([]string{"database", "delete", "--id", "shop"}) {
		t.Fatal("delete")
	}
	if destructiveArgv([]string{"sql", "exec", "--database", "d", "--statement", "INSERT INTO t VALUES (1)"}) {
		t.Fatal("insert")
	}
	if !destructiveArgv([]string{"sql", "exec", "--database", "d", "--statement", "DELETE FROM t"}) {
		t.Fatal("sql delete")
	}
	if destructiveArgv([]string{"kv", "exec", "--command", "SET", "--arg", "a", "--arg", "1"}) {
		t.Fatal("set")
	}
	if !destructiveArgv([]string{"kv", "exec", "--command", "DEL", "--arg", "a"}) {
		t.Fatal("del")
	}
	if !destructiveArgv([]string{"sql", "exec", "--database", "d", "--statement", "CALL something()"}) {
		t.Fatal("unsure")
	}
}

func TestTokenInArgvRejected(t *testing.T) {
	rt := &Runtime{}
	out, err := rt.execSimplebase(context.Background(), RunRequest{ProjectID: "p", RunID: "r"}, []string{"database", "list", "SIMPLEBASE_TOKEN"})
	if err != nil || !strings.Contains(out, "token_in_argv") {
		t.Fatalf("out=%s err=%v", out, err)
	}
}

func TestHeadlessDestructiveUnavailable(t *testing.T) {
	rt := &Runtime{APIBaseURL: "http://127.0.0.1:9", Issuer: stubIssuer{}}
	ctx := withRunContext(context.Background(), RunContext{Headless: true})
	out, err := rt.execSimplebase(ctx, RunRequest{ProjectID: "p", RunID: "r", Skills: []string{"database"}, Headless: true}, []string{"database", "delete", "--id", "shop"})
	if err != nil || !strings.Contains(out, "confirmation_unavailable") {
		t.Fatalf("out=%s err=%v", out, err)
	}
}

type stubIssuer struct{}

func (stubIssuer) Issue(auth.Principal, string, string, time.Duration) (string, error) {
	return "tok", nil
}
func (stubIssuer) RevokeRun(string) {}
