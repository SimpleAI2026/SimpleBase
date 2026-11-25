// 文件说明：sites_test.go — internal/systemdb 的单元测试（TestPublicPathsCRUD、TestMatchPublicPath 等）
//
// 用法
//
//	go test ./internal/systemdb -run 'TestPublicPathsCRUD|TestMatchPublicPath'
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app 等 8 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/sites_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] agent_approvals.go, agent_approvals_test.go, agent_collab.go, agent_collab_cron.go, agent_collab_test.go 等 54 个
//
// 主要符号
//
//	TestPublicPathsCRUD(t *testing.T)
//	TestMatchPublicPath(t *testing.T)

package systemdb

import (
	"context"
	"testing"
)

func TestPublicPathsCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pid := "p1234567"
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.SetPublicPath(ctx, pid, "blog/", PublicPathFolder, true, "a"))
	must(s.SetPublicPath(ctx, pid, "blog/", PublicPathFolder, true, "a")) // 幂等
	must(s.SetPublicPath(ctx, pid, "a.html", PublicPathFile, true, "a"))
	must(s.SetPublicPath(ctx, pid, "blog/x/y.html", PublicPathFile, true, "a"))
	must(s.SetPublicPath(ctx, "other123", "a.html", PublicPathFile, true, "a"))
	paths, err := s.ListPublicPaths(ctx, pid)
	must(err)
	if len(paths) != 3 {
		t.Fatalf("paths=%+v", paths)
	}
	must(s.DeletePublicFile(ctx, pid, "a.html"))
	must(s.DeletePublicPathsUnder(ctx, pid, "blog/"))
	paths, _ = s.ListPublicPaths(ctx, pid)
	if len(paths) != 0 {
		t.Fatalf("after delete paths=%+v", paths)
	}
	other, _ := s.ListPublicPaths(ctx, "other123")
	if len(other) != 1 {
		t.Fatalf("other project touched: %+v", other)
	}
	must(s.DeletePublicPathsUnder(ctx, "other123", ""))
	if other, _ = s.ListPublicPaths(ctx, "other123"); len(other) != 0 {
		t.Fatalf("project purge: %+v", other)
	}
}

func TestMatchPublicPath(t *testing.T) {
	paths := []PublicPath{
		{Path: "blog/", Kind: PublicPathFolder},
		{Path: "blog/sub/", Kind: PublicPathFolder},
		{Path: "a.html", Kind: PublicPathFile},
		{Path: "bad", Kind: PublicPathFolder}, // 非 "/" 结尾的 folder 忽略
	}
	cases := []struct {
		key  string
		ok   bool
		path string
	}{
		{"a.html", true, "a.html"},
		{"blog/index.html", true, "blog/"},
		{"blog/sub/x.css", true, "blog/sub/"},
		{"blog2/x", false, ""},
		{"badx", false, ""},
		{"b.html", false, ""},
	}
	for _, tc := range cases {
		got, ok := MatchPublicPath(paths, tc.key)
		if ok != tc.ok || got.Path != tc.path {
			t.Errorf("%s: got %q %v", tc.key, got.Path, ok)
		}
	}
	if _, ok := MatchPublicPath([]PublicPath{{Path: "", Kind: PublicPathFolder}}, "any/x"); !ok {
		t.Error("root folder should cover everything")
	}
}
