// 文件说明：agent_tags_test.go — internal/systemdb 的单元测试（TestCreateAgentTagBasicAndListOrder、TestCreateAgentTagDuplicateCaseInsensitive 等）
//
// 用法
//
//	go test ./internal/systemdb -run 'TestCreateAgentTagBasicAndListOrder|TestCreateAgentTagDuplicateCaseInsensitive|TestCreateAgentTagLimit|TestUpdateAgentTagVersionAndRename'
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app 等 8 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/agent_tags_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] agent_approvals.go, agent_approvals_test.go, agent_collab.go, agent_collab_cron.go, agent_collab_test.go 等 54 个
//
// 主要符号
//
//	tagCtx(t *testing.T) context.Context
//	mustCreateTag(t *testing.T, s *Store, projectID, name string, agentIDs []string) AgentTag
//	TestCreateAgentTagBasicAndListOrder(t *testing.T)
//	TestCreateAgentTagDuplicateCaseInsensitive(t *testing.T)
//	TestCreateAgentTagLimit(t *testing.T)
//	TestUpdateAgentTagVersionAndRename(t *testing.T)
//	TestUpsertBuiltinAgentTag(t *testing.T)
//	TestArchiveAgentTagTransaction(t *testing.T)
//	... 另有 3 个顶层符号

// planv6.7 AGC-NAV：agent_tags.go 的仓储测试（§7.12）。
// 覆盖：建/改/归档、重名（大小写）、50 上限、版本冲突、预置 upsert、
// 归档事务（tag_id 清空且 dispatch_mode 保留）、CreateAgentThreadWithWorkspace
// 零值不写元数据、PageAgentThreadsFiltered 的 chat 规则与标签分区。
// （run 路由记录随一轮一助手 auto 调度一并下线，见 planv6.8 §13.15 CB-BE-13。）
package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

func tagCtx(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}

func mustCreateTag(t *testing.T, s *Store, projectID, name string, agentIDs []string) AgentTag {
	t.Helper()
	tag, err := s.CreateAgentTag(tagCtx(t), AgentTag{
		ProjectID: projectID,
		Name:      name,
		CreatedBy: "user-1",
		Config:    AgentTagConfig{AgentIDs: agentIDs, Prompts: []AgentTagPrompt{{ID: "p1", Title: "T", Content: "C"}}},
	})
	if err != nil {
		t.Fatalf("CreateAgentTag(%s): %v", name, err)
	}
	return tag
}

func TestCreateAgentTagBasicAndListOrder(t *testing.T) {
	s := newTestStore(t)
	ctx := tagCtx(t)
	a := mustCreateTag(t, s, "p1", "发布检查", []string{"a1"})
	if a.ID == "" || a.Version != 1 || a.BuiltinKey != "" || !a.Persisted {
		t.Fatalf("bad created tag: %+v", a)
	}
	b := mustCreateTag(t, s, "p1", "统计复盘", []string{"a2"})
	rows, err := s.ListAgentTagRows(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID != a.ID || rows[1].ID != b.ID {
		t.Fatalf("list order: %+v", rows)
	}
	// 项目隔离
	other, err := s.ListAgentTagRows(ctx, "p2")
	if err != nil || len(other) != 0 {
		t.Fatalf("other project: %v %+v", err, other)
	}
	got, err := s.GetAgentTagRow(ctx, "p1", a.ID)
	if err != nil || got.Name != "发布检查" || len(got.Config.AgentIDs) != 1 {
		t.Fatalf("get: %v %+v", err, got)
	}
	if _, err := s.GetAgentTagRow(ctx, "p1", "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing row: %v", err)
	}
}

func TestCreateAgentTagDuplicateCaseInsensitive(t *testing.T) {
	s := newTestStore(t)
	mustCreateTag(t, s, "p1", "发布", nil)
	for _, dup := range []string{"发布"} {
		if _, err := s.CreateAgentTag(tagCtx(t), AgentTag{ProjectID: "p1", Name: dup}); err == nil {
			t.Fatalf("expected duplicate rejection for %q", dup)
		} else if !errors.Is(err, ErrAgentTagDuplicate) {
			t.Fatalf("wrong error for %q: %v", dup, err)
		}
	}
	// 英文大小写不敏感
	mustCreateTag(t, s, "p1", "Deploy", nil)
	if _, err := s.CreateAgentTag(tagCtx(t), AgentTag{ProjectID: "p1", Name: "deploy"}); !errors.Is(err, ErrAgentTagDuplicate) {
		t.Fatalf("case-insensitive duplicate: %v", err)
	}
	// 同名但不同项目允许
	if _, err := s.CreateAgentTag(tagCtx(t), AgentTag{ProjectID: "p2", Name: "发布"}); err != nil {
		t.Fatalf("other project duplicate should pass: %v", err)
	}
	// 归档后可复用名字
	a := mustCreateTag(t, s, "p1", "临时", nil)
	if err := s.ArchiveAgentTag(tagCtx(t), "p1", a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAgentTag(tagCtx(t), AgentTag{ProjectID: "p1", Name: "临时"}); err != nil {
		t.Fatalf("reuse archived name: %v", err)
	}
}

func TestCreateAgentTagLimit(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < agentTagCustomLimit; i++ {
		mustCreateTag(t, s, "p1", "tag-"+strings.Repeat("a", 0)+string(rune('a'+i%26))+string(rune('a'+i/26)), nil)
	}
	_, err := s.CreateAgentTag(tagCtx(t), AgentTag{ProjectID: "p1", Name: "overflow"})
	if !errors.Is(err, ErrAgentTagLimit) {
		t.Fatalf("expected limit error, got %v", err)
	}
	// 归档一个后可再建
	tags, _ := s.ListAgentTagRows(tagCtx(t), "p1")
	if err := s.ArchiveAgentTag(tagCtx(t), "p1", tags[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAgentTag(tagCtx(t), AgentTag{ProjectID: "p1", Name: "overflow"}); err != nil {
		t.Fatalf("after archive: %v", err)
	}
}

func TestUpdateAgentTagVersionAndRename(t *testing.T) {
	s := newTestStore(t)
	ctx := tagCtx(t)
	a := mustCreateTag(t, s, "p1", "一号", []string{"a1"})
	mustCreateTag(t, s, "p1", "二号", nil)

	// 版本不符 → conflict
	if _, err := s.UpdateAgentTag(ctx, "p1", a.ID, "改名", AgentTagConfig{}, 99); !errors.Is(err, ErrAgentTagConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	// 改成既有名 → duplicate
	if _, err := s.UpdateAgentTag(ctx, "p1", a.ID, "二号", AgentTagConfig{}, 1); !errors.Is(err, ErrAgentTagDuplicate) {
		t.Fatalf("expected duplicate, got %v", err)
	}
	// 正常更新：version+1
	upd, err := s.UpdateAgentTag(ctx, "p1", a.ID, "一号改名", AgentTagConfig{AgentIDs: []string{"x"}}, 1)
	if err != nil || upd.Version != 2 || upd.Name != "一号改名" {
		t.Fatalf("update: %v %+v", err, upd)
	}
	// 旧版本再更新 → conflict
	if _, err := s.UpdateAgentTag(ctx, "p1", a.ID, "再改", AgentTagConfig{}, 1); !errors.Is(err, ErrAgentTagConflict) {
		t.Fatalf("stale version: %v", err)
	}
	// 不存在 → not found
	if _, err := s.UpdateAgentTag(ctx, "p1", "missing", "n", AgentTagConfig{}, 1); !errors.Is(err, ErrAgentTagNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestUpsertBuiltinAgentTag(t *testing.T) {
	s := newTestStore(t)
	ctx := tagCtx(t)
	cfg := AgentTagConfig{AgentIDs: []string{"g1"}, Prompts: []AgentTagPrompt{{ID: "p", Title: "t", Content: "c"}}}

	// 首次：expected_version=0 插入 version=1
	first, err := s.UpsertBuiltinAgentTag(ctx, "p1", AgentTagStats, "统计", cfg, 0)
	if err != nil || first.ID != AgentTagStats || first.BuiltinKey != AgentTagStats || first.Version != 1 {
		t.Fatalf("first upsert: %v %+v", err, first)
	}
	// 首次但 expected_version 非 0 → conflict
	if _, err := s.UpsertBuiltinAgentTag(ctx, "p2", AgentTagAlerts, "告警", cfg, 3); !errors.Is(err, ErrAgentTagConflict) {
		t.Fatalf("bad first expect: %v", err)
	}
	// 版本相符 → version+1
	second, err := s.UpsertBuiltinAgentTag(ctx, "p1", AgentTagStats, "统计", cfg, 1)
	if err != nil || second.Version != 2 {
		t.Fatalf("second upsert: %v %+v", err, second)
	}
	// 旧版本 → conflict
	if _, err := s.UpsertBuiltinAgentTag(ctx, "p1", AgentTagStats, "统计", cfg, 1); !errors.Is(err, ErrAgentTagConflict) {
		t.Fatalf("stale upsert: %v", err)
	}
	// 非法 key → not found
	if _, err := s.UpsertBuiltinAgentTag(ctx, "p1", "chat", "对话", cfg, 0); !errors.Is(err, ErrAgentTagNotFound) {
		t.Fatalf("bad key: %v", err)
	}
	// 自定义标签 ID 不走 upsert 路径
	mustCreateTag(t, s, "p1", "自定义", nil)
}

func TestArchiveAgentTagTransaction(t *testing.T) {
	s := newTestStore(t)
	ctx := tagCtx(t)
	tag := mustCreateTag(t, s, "p1", "发布", []string{"a1"})

	// 标签会话：tag_id=tag.ID, dispatch_mode=auto
	wsThread, err := s.CreateAgentThreadWithWorkspace(ctx, AgentThread{ProjectID: "p1", Title: "t1"}, AgentThreadWorkspace{TagID: tag.ID, DispatchMode: DispatchCollaborative, Source: ThreadSourceWorkspace})
	if err != nil {
		t.Fatal(err)
	}
	// 普通会话：无元数据
	plainThread, err := s.CreateAgentThread(ctx, AgentThread{ProjectID: "p1", Title: "t2"})
	if err != nil {
		t.Fatal(err)
	}

	// 预置标签不可归档
	if err := s.ArchiveAgentTag(ctx, "p1", AgentTagAlerts); !errors.Is(err, ErrAgentTagBuiltin) {
		t.Fatalf("builtin archive: %v", err)
	}
	if err := s.ArchiveAgentTag(ctx, "p1", AgentTagStats); !errors.Is(err, ErrAgentTagBuiltin) {
		t.Fatalf("builtin archive stats: %v", err)
	}
	// 不存在
	if err := s.ArchiveAgentTag(ctx, "p1", "missing"); !errors.Is(err, ErrAgentTagNotFound) {
		t.Fatalf("missing archive: %v", err)
	}

	// 归档自定义标签
	if err := s.ArchiveAgentTag(ctx, "p1", tag.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAgentTagRow(ctx, "p1", tag.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("archived tag still visible: %v", err)
	}
	// 会话元数据：v2 §9.2 归档**保留** tag_id 归属（历史仍可在群下查看），
	// dispatch_mode 同样保留；候选范围不扩大由 API 层按 archived_at 判断。
	ws, err := s.GetAgentThreadWorkspace(ctx, "p1", wsThread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ws.TagID != tag.ID || ws.DispatchMode != DispatchCollaborative {
		t.Fatalf("workspace after archive should keep ownership: %+v", ws)
	}
	// 普通会话仍无元数据行
	if ws2, err := s.GetAgentThreadWorkspace(ctx, "p1", plainThread.ID); err != nil || ws2 != (AgentThreadWorkspace{}) {
		t.Fatalf("plain thread meta: %v %+v", err, ws2)
	}
	// 归档后群定时绑定同步暂停（v2 §11.4）
	if err := s.ArchiveAgentGroupCronsByTag(ctx, "p1", tag.ID); err != nil {
		t.Fatal(err)
	}
}

func TestCreateAgentThreadWithWorkspaceZeroValue(t *testing.T) {
	s := newTestStore(t)
	ctx := tagCtx(t)
	// 零值 ws：不写元数据行（等价旧 CreateAgentThread）
	th, err := s.CreateAgentThreadWithWorkspace(ctx, AgentThread{ProjectID: "p1"}, AgentThreadWorkspace{})
	if err != nil {
		t.Fatal(err)
	}
	if th.Title != "New thread" || th.ID == "" {
		t.Fatalf("bad thread: %+v", th)
	}
	ws, err := s.GetAgentThreadWorkspace(ctx, "p1", th.ID)
	if err != nil || ws != (AgentThreadWorkspace{}) {
		t.Fatalf("zero ws should not persist: %v %+v", err, ws)
	}
	// 带元数据
	th2, err := s.CreateAgentThreadWithWorkspace(ctx, AgentThread{ProjectID: "p1", Title: "x"}, AgentThreadWorkspace{DispatchMode: DispatchCollaborative, Source: ThreadSourceWorkspace})
	if err != nil {
		t.Fatal(err)
	}
	ws2, err := s.GetAgentThreadWorkspace(ctx, "p1", th2.ID)
	if err != nil || ws2.DispatchMode != DispatchCollaborative || ws2.Source != ThreadSourceWorkspace {
		t.Fatalf("meta: %v %+v", err, ws2)
	}
	// 批量读取
	m, err := s.ListAgentThreadWorkspaces(ctx, "p1", []string{th.ID, th2.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 1 || m[th2.ID].DispatchMode != DispatchCollaborative {
		t.Fatalf("batch: %+v", m)
	}
	if empty, err := s.ListAgentThreadWorkspaces(ctx, "p1", nil); err != nil || len(empty) != 0 {
		t.Fatalf("empty batch: %v", err)
	}
}

func TestPageAgentThreadsFilteredTagRules(t *testing.T) {
	s := newTestStore(t)
	ctx := tagCtx(t)
	tag := mustCreateTag(t, s, "p1", "标签", nil)

	mk := func(title, agentID, createdBy string, ws AgentThreadWorkspace) string {
		th, err := s.CreateAgentThreadWithWorkspace(ctx, AgentThread{ProjectID: "p1", Title: title, AgentID: agentID, CreatedBy: createdBy}, ws)
		if err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
		return th.ID
	}
	generalID := mk("通用会话", "a1", "", AgentThreadWorkspace{TagID: AgentTagGeneral, DispatchMode: DispatchCollaborative})
	generalAPIKeyID := mk("通用 API 会话", "a2", "key-123", AgentThreadWorkspace{TagID: AgentTagGeneral, DispatchMode: DispatchCollaborative})
	tagThreadID := mk("标签会话", "", "", AgentThreadWorkspace{TagID: tag.ID, DispatchMode: DispatchCollaborative})
	mk("无标签会话", "a3", "", AgentThreadWorkspace{})

	threads, next, err := s.PageAgentThreadsFiltered(ctx, "p1", AgentThreadFilter{TagID: AgentTagGeneral}, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if next != "" {
		t.Fatalf("unexpected cursor %q", next)
	}
	got := map[string]bool{}
	for _, th := range threads {
		got[th.ID] = true
	}
	if len(threads) != 2 || !got[generalID] || !got[generalAPIKeyID] {
		t.Fatalf("general filter: %v", got)
	}

	tagThreads, _, err := s.PageAgentThreadsFiltered(ctx, "p1", AgentThreadFilter{TagID: tag.ID}, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(tagThreads) != 1 || tagThreads[0].ID != tagThreadID {
		t.Fatalf("tag filter: %+v", tagThreads)
	}

	agentFiltered, _, err := s.PageAgentThreads(ctx, "p1", "a1", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(agentFiltered) != 1 || agentFiltered[0].ID != generalID {
		t.Fatalf("agent filter: %+v", agentFiltered)
	}

	both, _, err := s.PageAgentThreadsFiltered(ctx, "p1", AgentThreadFilter{TagID: AgentTagGeneral, AgentID: "a1"}, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(both) != 1 || both[0].ID != generalID {
		t.Fatalf("intersection: %+v", both)
	}

	all, _, err := s.PageAgentThreadsFiltered(ctx, "p1", AgentThreadFilter{}, "", 50)
	if err != nil || len(all) != 4 {
		t.Fatalf("no filter: %v %d", err, len(all))
	}
	if _, _, err := s.PageAgentThreadsFiltered(ctx, "p1", AgentThreadFilter{TagID: AgentTagGeneral}, "no-such-cursor", 50); !errors.Is(err, ErrAgentThreadCursor) {
		t.Fatalf("bad cursor: %v", err)
	}
}

func TestPageAgentThreadsFilteredPagination(t *testing.T) {
	s := newTestStore(t)
	ctx := tagCtx(t)
	for i := 0; i < 5; i++ {
		mkThread := AgentThread{ProjectID: "p1", Title: "t"}
		if _, err := s.CreateAgentThreadWithWorkspace(ctx, mkThread, AgentThreadWorkspace{TagID: AgentTagGeneral, DispatchMode: DispatchCollaborative}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}
	page1, cursor, err := s.PageAgentThreadsFiltered(ctx, "p1", AgentThreadFilter{TagID: AgentTagGeneral}, "", 2)
	if err != nil || len(page1) != 2 || cursor == "" {
		t.Fatalf("page1: %v %d %q", err, len(page1), cursor)
	}
	page2, next, err := s.PageAgentThreadsFiltered(ctx, "p1", AgentThreadFilter{TagID: AgentTagGeneral}, cursor, 2)
	if err != nil || len(page2) != 2 || next == "" {
		t.Fatalf("page2: %v %d %q", err, len(page2), next)
	}
	if page1[0].ID == page2[0].ID || page1[1].ID == page2[1].ID {
		t.Fatalf("pages overlap: %v / %v", page1, page2)
	}
	seen := map[string]bool{}
	for _, th := range append(append([]AgentThread{}, page1...), page2...) {
		seen[th.ID] = true
	}
	if len(seen) != 4 {
		t.Fatalf("distinct: %d", len(seen))
	}
}
