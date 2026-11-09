package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/uglyer/go-sqlite3"

	"github.com/linkxzhou/SimpleBase/internal/utils/sqlval"
)

func TestStatisticalBuiltinsSeedWithoutOverwritingExisting(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	const project = "statistical-builtins"
	if err := s.SeedDefaultCloudAgentsWithCapabilities(ctx, project, false, true); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListCloudAgents(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	byKey := make(map[string]CloudAgent, len(list))
	for _, agent := range list {
		byKey[agent.BuiltinKey] = agent
	}
	for _, key := range []string{"general", "database", "kv", "gofunction", "cron", "s3", "logs"} {
		if byKey[key].ID == "" {
			t.Fatalf("builtin %s missing", key)
		}
	}
	if byKey["users"].ID != "" || byKey["sandbox"].ID != "" {
		t.Fatal("restricted or unavailable agent must not be seeded for everyone")
	}
	before := byKey["database"].ID
	if err := s.SeedDefaultCloudAgentsWithCapabilities(ctx, project, false, true); err != nil {
		t.Fatal(err)
	}
	again, err := s.ListCloudAgents(ctx, project)
	if err != nil || len(again) != len(list) {
		t.Fatalf("idempotent seed: agents=%d err=%v", len(again), err)
	}
	for _, agent := range again {
		if agent.BuiltinKey == "database" && agent.ID != before {
			t.Fatalf("existing database agent id changed: %q != %q", agent.ID, before)
		}
	}
	const later = "statistical-enabled-later"
	if err := s.SeedDefaultCloudAgents(ctx, later); err != nil {
		t.Fatal(err)
	}
	old, err := s.ListCloudAgents(ctx, later)
	if err != nil || len(old) != 4 {
		t.Fatalf("legacy defaults=%d err=%v", len(old), err)
	}
	legacyID := old[0].ID
	if err := s.SeedDefaultCloudAgentsWithCapabilities(ctx, later, false, true); err != nil {
		t.Fatal(err)
	}
	updated, err := s.ListCloudAgents(ctx, later)
	if err != nil || len(updated) != 7 || updated[0].ID != legacyID {
		t.Fatalf("enabling CLI should add three agents without replacing existing ones: agents=%d err=%v", len(updated), err)
	}
}

func TestSandboxBuiltinFollowsAvailabilityWithoutReplacingExisting(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	const project = "sandbox-seed-lifecycle"
	for step, available := range []bool{false, true, false, true} {
		if err := s.SeedDefaultCloudAgentsWithCapabilities(ctx, project, available, false); err != nil {
			t.Fatal(err)
		}
		list, err := s.ListCloudAgents(ctx, project)
		if err != nil {
			t.Fatal(err)
		}
		want := 4
		if step > 0 {
			want = 5
		}
		if len(list) != want {
			t.Fatalf("sandboxAvailable=%v agents=%d want=%d", available, len(list), want)
		}
	}
	list, err := s.ListCloudAgents(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	var sandbox CloudAgent
	for _, agent := range list {
		if agent.BuiltinKey == "sandbox" {
			sandbox = agent
		}
	}
	if sandbox.ID == "" || sandbox.Module != "sandbox" || len(sandbox.ToolIDs) != 4 {
		t.Fatalf("missing sandbox builtin: %+v", sandbox)
	}
	originalID := sandbox.ID
	if err := s.ArchiveCloudAgent(ctx, project, sandbox.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedDefaultCloudAgentsWithCapabilities(ctx, project, true, false); err != nil {
		t.Fatal(err)
	}
	list, err = s.ListCloudAgents(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range list {
		if agent.BuiltinKey == "sandbox" || agent.ID == originalID {
			t.Fatalf("dismissed sandbox restored: %+v", agent)
		}
	}
}

func TestAgentSeedKeysPreserveDismissalAndProjectIsolation(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	const project = "agent-seed-test"
	if err := s.SeedDefaultCloudAgentsWithCapabilities(ctx, project, false, true); err != nil {
		t.Fatal(err)
	}
	agents, err := s.ListCloudAgents(ctx, project)
	if err != nil || len(agents) != len(defaultCloudAgentSpecs)-1 {
		t.Fatalf("initial agents=%d err=%v", len(agents), err)
	}
	var generalID string
	for _, agent := range agents {
		if agent.BuiltinKey == "general" {
			generalID = agent.ID
		}
	}
	if generalID == "" {
		t.Fatal("general agent missing")
	}
	if err := s.ArchiveCloudAgent(ctx, project, generalID); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedDefaultCloudAgentsWithCapabilities(ctx, project, false, true); err != nil {
		t.Fatal(err)
	}
	agents, err = s.ListCloudAgents(ctx, project)
	if err != nil || len(agents) != len(defaultCloudAgentSpecs)-2 {
		t.Fatalf("dismissed agent restored: agents=%d err=%v", len(agents), err)
	}
	other, err := s.ListCloudAgents(ctx, "another-project")
	if err != nil || len(other) != 0 {
		t.Fatalf("cross-project agents=%d err=%v", len(other), err)
	}
	if err := s.SeedDefaultCloudAgentsWithCapabilities(ctx, "another-project", false, true); err != nil {
		t.Fatal(err)
	}
	other, err = s.ListCloudAgents(ctx, "another-project")
	if err != nil || len(other) != len(defaultCloudAgentSpecs)-1 {
		t.Fatalf("other project agents=%d err=%v", len(other), err)
	}
}

func TestSeedDefaultCloudAgentsConcurrentFirstVisit(t *testing.T) {
	s := newTestStore(t)
	const project = "concurrent-first-visit"
	const n = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = s.SeedDefaultCloudAgentsWithCapabilities(context.Background(), project, false, true)
		}(i)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent seed %d failed: %v", i, err)
		}
	}
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT builtin_key, count(*) FROM sys_cloud_agents WHERE project_id=? AND archived_at IS NULL GROUP BY builtin_key`, project)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := make(map[string]int)
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			t.Fatal(err)
		}
		seen[key] = count
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(seen) != len(defaultCloudAgentSpecs)-1 {
		t.Fatalf("builtin keys=%d want %d", len(seen), len(defaultCloudAgentSpecs)-1)
	}
	for key, count := range seen {
		if count != 1 {
			t.Fatalf("key %s has %d active rows", key, count)
		}
	}
}

func TestSeedProjectLockWaitCanCancel(t *testing.T) {
	s := newTestStore(t)
	release, err := s.lockAgentSeed(context.Background(), "busy")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.SeedDefaultCloudAgents(ctx, "busy"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled seed waited for busy project: %v", err)
	}
	if err := s.SeedDefaultCloudAgents(context.Background(), "other"); err != nil {
		t.Fatalf("unrelated project blocked: %v", err)
	}
}

func TestAgentMessagesReturnMostRecentPageInChronologicalOrder(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := EnsureSystemSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	store := NewStoreForTest(db)
	thread, err := store.CreateAgentThread(ctx, AgentThread{ProjectID: "p", Title: "recent"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		if _, err := store.AppendAgentMessage(ctx, AgentMessage{ID: fmt.Sprintf("%03d", i), ThreadID: thread.ID, ProjectID: "p", Role: "user", Content: fmt.Sprintf("message-%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	msgs, err := store.ListAgentMessages(ctx, "p", thread.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 5 || msgs[0].Content != "message-45" || msgs[4].Content != "message-49" {
		t.Fatalf("history=%+v", msgs)
	}
}

func TestCloudAgentsThreadsAndRuns(t *testing.T) {
	ctx := context.Background()
	var nilStore *Store
	if _, err := nilStore.CreateCloudAgent(ctx, CloudAgent{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.ListCloudAgents(ctx, "p"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.GetCloudAgent(ctx, "p", "id"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.UpdateCloudAgent(ctx, CloudAgent{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	assertUnavailable(t, nilStore.ArchiveCloudAgent(ctx, "p", "id"))
	if _, err := nilStore.CountCloudAgents(ctx, "p"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.CreateAgentThread(ctx, AgentThread{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.ListAgentThreads(ctx, "p", "", 0); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.GetAgentThread(ctx, "p", "id"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	assertUnavailable(t, nilStore.ArchiveAgentThread(ctx, "p", "id"))
	if _, err := nilStore.AppendAgentMessage(ctx, AgentMessage{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.ListAgentMessages(ctx, "p", "t", 0); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.CreateAgentRun(ctx, AgentRun{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.GetAgentRun(ctx, "p", "id"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	assertUnavailable(t, nilStore.UpdateAgentRunStatus(ctx, "p", "id", AgentRunFailed, "e", nil, nil))
	assertUnavailable(t, nilStore.SeedDefaultCloudAgents(ctx, "p"))

	s := newTestStore(t)
	pid := "proj-a"
	a, err := s.CreateCloudAgent(ctx, CloudAgent{ProjectID: pid, Name: "A", Module: "m", TeamEnabled: true, ToolIDs: []string{"t1"}})
	if err != nil || a.ID == "" || !a.TeamEnabled {
		t.Fatalf("create: %+v err=%v", a, err)
	}
	fixed := uuid.NewString()
	if _, err := s.CreateCloudAgent(ctx, CloudAgent{ID: fixed, ProjectID: pid, Name: "B", Module: "m"}); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListCloudAgents(ctx, pid)
	if err != nil || len(list) != 2 {
		t.Fatalf("list: n=%d err=%v", len(list), err)
	}
	got, err := s.GetCloudAgent(ctx, pid, a.ID)
	if err != nil || got.Name != "A" || len(got.ToolIDs) != 1 {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	if _, err := s.GetCloudAgent(ctx, pid, uuid.NewString()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing agent: %v", err)
	}
	got.Description = "upd"
	got.TeamEnabled = false
	got.ToolIDs = nil
	upd, err := s.UpdateCloudAgent(ctx, got)
	if err != nil || upd.TeamEnabled || upd.Description != "upd" {
		t.Fatalf("update: %+v err=%v", upd, err)
	}
	if _, err := s.UpdateCloudAgent(ctx, CloudAgent{ID: uuid.NewString(), ProjectID: pid, Name: "nope"}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("update missing: %v", err)
	}
	if err := s.SeedDefaultCloudAgents(ctx, pid); err != nil {
		t.Fatal(err)
	}
	// 已有任意 agent 时播种补齐 4 个内置助手（BUG-11：按 builtin_key 幂等补齐）。
	n, err := s.CountCloudAgents(ctx, pid)
	if err != nil || n != 6 {
		t.Fatalf("count existing: %d err=%v", n, err)
	}
	list, err = s.ListCloudAgents(ctx, pid)
	if err != nil || len(list) == 0 || list[0].BuiltinKey != "general" {
		t.Fatalf("general agent should sort first: %+v err=%v", list, err)
	}
	if err := s.ArchiveCloudAgent(ctx, pid, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ArchiveCloudAgent(ctx, pid, a.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("archive again: %v", err)
	}

	th, err := s.CreateAgentThread(ctx, AgentThread{ProjectID: pid})
	if err != nil || th.Title != "New thread" {
		t.Fatalf("thread: %+v err=%v", th, err)
	}
	if _, err := s.CreateAgentThread(ctx, AgentThread{ID: uuid.NewString(), ProjectID: pid, Title: "named", CreatedBy: "u"}); err != nil {
		t.Fatal(err)
	}
	threads, err := s.ListAgentThreads(ctx, pid, "", 0)
	if err != nil || len(threads) != 2 {
		t.Fatalf("threads: n=%d err=%v", len(threads), err)
	}
	if _, err := s.GetAgentThread(ctx, pid, th.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAgentThread(ctx, pid, uuid.NewString()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}

	long := strings.Repeat("x", 50)
	if _, err := s.AppendAgentMessage(ctx, AgentMessage{ThreadID: th.ID, ProjectID: pid, Role: "user", Content: long}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendAgentMessage(ctx, AgentMessage{ID: uuid.NewString(), ThreadID: th.ID, ProjectID: pid, Role: "assistant", Content: "ok", MentionsJSON: `["a"]`, ToolCallsJSON: `[]`}); err != nil {
		t.Fatal(err)
	}
	msgs, err := s.ListAgentMessages(ctx, pid, th.ID, 0)
	if err != nil || len(msgs) != 2 {
		t.Fatalf("msgs: n=%d err=%v", len(msgs), err)
	}
	msgs, err = s.ListAgentMessages(ctx, pid, th.ID, 900)
	if err != nil || len(msgs) != 2 {
		t.Fatalf("clamped msgs: n=%d", len(msgs))
	}
	named, err := s.GetAgentThread(ctx, pid, th.ID)
	if err != nil || named.Title == "New thread" || len(named.Title) > 40 {
		t.Fatalf("thread title: %+v", named)
	}

	run, err := s.CreateAgentRun(ctx, AgentRun{ThreadID: th.ID, ProjectID: pid, AgentID: fixed})
	if err != nil || run.Status != AgentRunQueued || run.ID == "" {
		t.Fatalf("run defaults: %+v err=%v", run, err)
	}
	started := time.Now().UTC()
	run2, err := s.CreateAgentRun(ctx, AgentRun{ID: uuid.NewString(), ThreadID: th.ID, ProjectID: pid, AgentID: fixed, Status: AgentRunRunning, StartedAt: started})
	if err != nil {
		t.Fatal(err)
	}
	gotRun, err := s.GetAgentRun(ctx, pid, run2.ID)
	if err != nil || gotRun.Status != AgentRunRunning || gotRun.StartedAt.IsZero() {
		t.Fatalf("get run: %+v err=%v", gotRun, err)
	}
	if _, err := s.GetAgentRun(ctx, pid, uuid.NewString()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	fin := started.Add(time.Second)
	if err := s.UpdateAgentRunStatus(ctx, pid, run2.ID, AgentRunCompleted, "", &started, &fin); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateAgentRunStatus(ctx, pid, run.ID, AgentRunFailed, "boom", nil, nil); err != nil {
		t.Fatal(err)
	}
	failed, err := s.GetAgentRun(ctx, pid, run.ID)
	if err != nil || failed.Status != AgentRunFailed || failed.Error != "boom" {
		t.Fatalf("failed run: %+v", failed)
	}

	if err := s.ArchiveAgentThread(ctx, pid, th.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ArchiveAgentThread(ctx, pid, th.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}

	// bad tool_ids JSON
	if _, err := s.db.ExecContext(ctx, `INSERT INTO sys_cloud_agents(id, project_id, name, module, description, system_prompt, tool_ids, model_override, team_enabled, created_at, updated_at, archived_at)
		VALUES(?, ?, 'bad', '', '', '', '{', '', 0, ?, ?, NULL)`, uuid.NewString(), pid, time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListCloudAgents(ctx, pid); err == nil {
		t.Fatal("bad tool json should fail list")
	}

	empty := newTestStore(t)
	if err := empty.SeedDefaultCloudAgents(ctx, "fresh"); err != nil {
		t.Fatal(err)
	}
	n, err = empty.CountCloudAgents(ctx, "fresh")
	if err != nil || n != 4 {
		t.Fatalf("seeded n=%d err=%v", n, err)
	}
}

// TestAgentThreadsFilteredByAgent 覆盖 planv6.0 §3.7 修订：会话归属助手。
// 每个助手一份列表；未绑定（空 agent_id）的历史数据只在不过滤时可见。
func TestAgentThreadsFilteredByAgent(t *testing.T) {
	ctx := context.Background()
	var nilStore *Store
	if _, err := nilStore.ListAgentThreads(ctx, "p", "a", 0); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, _, err := nilStore.PageAgentThreads(ctx, "p", "a", "", 0); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}

	s := newTestStore(t)
	const pid = "thread-by-agent"
	for _, in := range []AgentThread{
		{ProjectID: pid, Title: "a1", AgentID: "ag-a"},
		{ProjectID: pid, Title: "a2", AgentID: "ag-a"},
		{ProjectID: pid, Title: "b1", AgentID: "ag-b"},
		{ProjectID: pid, Title: "legacy"}, // 历史数据：未绑定
	} {
		if _, err := s.CreateAgentThread(ctx, in); err != nil {
			t.Fatalf("create %s: %v", in.Title, err)
		}
	}

	// 按助手过滤：只拿到自己的
	aThreads, err := s.ListAgentThreads(ctx, pid, "ag-a", 0)
	if err != nil || len(aThreads) != 2 {
		t.Fatalf("ag-a threads: n=%d err=%v", len(aThreads), err)
	}
	for _, th := range aThreads {
		if th.AgentID != "ag-a" {
			t.Fatalf("unexpected agent_id %q", th.AgentID)
		}
	}
	bThreads, err := s.ListAgentThreads(ctx, pid, "ag-b", 0)
	if err != nil || len(bThreads) != 1 {
		t.Fatalf("ag-b threads: n=%d err=%v", len(bThreads), err)
	}
	// 不带过滤（空串 = 不过滤）：全部，含未绑定的历史数据
	all, err := s.ListAgentThreads(ctx, pid, "", 0)
	if err != nil || len(all) != 4 {
		t.Fatalf("all threads: n=%d err=%v", len(all), err)
	}

	// 分页同样按助手过滤
	page, _, err := s.PageAgentThreads(ctx, pid, "ag-a", "", 50)
	if err != nil || len(page) != 2 {
		t.Fatalf("page ag-a: n=%d err=%v", len(page), err)
	}
	pageAll, _, err := s.PageAgentThreads(ctx, pid, "", "", 50)
	if err != nil || len(pageAll) != 4 {
		t.Fatalf("page all: n=%d err=%v", len(pageAll), err)
	}

	// 取回的单条也带 agent_id
	got, err := s.GetAgentThread(ctx, pid, bThreads[0].ID)
	if err != nil || got.AgentID != "ag-b" {
		t.Fatalf("get: %+v err=%v", got, err)
	}
}

func TestAgentSchedulesAndRuns(t *testing.T) {
	ctx := context.Background()
	var nilStore *Store
	if _, err := nilStore.CreateAgentSchedule(ctx, AgentSchedule{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.GetAgentSchedule(ctx, "p", "id"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.GetAgentScheduleByAgent(ctx, "p", "a"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.ListAgentSchedules(ctx, "p"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.UpdateAgentSchedule(ctx, AgentSchedule{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	assertUnavailable(t, nilStore.ArchiveAgentSchedule(ctx, "p", "id"))
	if _, err := nilStore.ListDueAgentSchedules(ctx, time.Now(), 0); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.ClaimAgentSchedule(ctx, "id", time.Time{}, time.Time{}, time.Time{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	assertUnavailable(t, nilStore.InsertAgentScheduleRun(ctx, AgentScheduleRun{}))
	assertUnavailable(t, nilStore.UpdateAgentScheduleRunStatus(ctx, "p", "id", AgentRunFailed, "", nil))
	if _, err := nilStore.ListAgentScheduleRuns(ctx, "p", "s", 0); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}

	s := newTestStore(t)
	pid := "proj-s"
	now := time.Now().UTC().Truncate(time.Second)
	due := now.Add(-time.Minute)
	sc, err := s.CreateAgentSchedule(ctx, AgentSchedule{
		ProjectID: pid, AgentID: "ag1", ThreadID: "th1", Prompt: "run", CronExpr: "* * * * *",
		Enabled: true, NextRunAt: due, LastRunAt: now.Add(-time.Hour), CreatedBy: "u",
	})
	if err != nil || sc.ID == "" {
		t.Fatalf("create: %+v err=%v", sc, err)
	}
	if _, err := s.CreateAgentSchedule(ctx, AgentSchedule{ID: uuid.NewString(), ProjectID: pid, AgentID: "ag2", CronExpr: "0 * * * *"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetAgentSchedule(ctx, pid, sc.ID)
	if err != nil || !got.Enabled || got.LastRunAt.IsZero() || got.NextRunAt.IsZero() {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	if _, err := s.GetAgentSchedule(ctx, pid, uuid.NewString()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	byAgent, err := s.GetAgentScheduleByAgent(ctx, pid, "ag1")
	if err != nil || byAgent.ID != sc.ID {
		t.Fatalf("by agent: %+v err=%v", byAgent, err)
	}
	if _, err := s.GetAgentScheduleByAgent(ctx, pid, "none"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	all, err := s.ListAgentSchedules(ctx, pid)
	if err != nil || len(all) != 2 {
		t.Fatalf("list: n=%d err=%v", len(all), err)
	}

	got.Prompt = "updated"
	got.Enabled = true
	upd, err := s.UpdateAgentSchedule(ctx, got)
	if err != nil || upd.Prompt != "updated" {
		t.Fatalf("update: %+v err=%v", upd, err)
	}
	if _, err := s.UpdateAgentSchedule(ctx, AgentSchedule{ID: uuid.NewString(), ProjectID: pid}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}

	dueList, err := s.ListDueAgentSchedules(ctx, now, 0)
	if err != nil || len(dueList) != 1 || dueList[0].ID != sc.ID {
		t.Fatalf("due: %+v err=%v", dueList, err)
	}
	dueList, err = s.ListDueAgentSchedules(ctx, now, 900)
	if err != nil || len(dueList) != 1 {
		t.Fatalf("clamped due: n=%d err=%v", len(dueList), err)
	}

	newNext := now.Add(10 * time.Minute)
	ok, err := s.ClaimAgentSchedule(ctx, sc.ID, sc.NextRunAt, newNext, now)
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	ok, err = s.ClaimAgentSchedule(ctx, sc.ID, sc.NextRunAt, newNext, now)
	if err != nil || ok {
		t.Fatalf("stale claim: ok=%v err=%v", ok, err)
	}
	ok, err = s.ClaimAgentSchedule(ctx, uuid.NewString(), time.Time{}, time.Time{}, time.Time{})
	if err != nil || ok {
		t.Fatalf("missing claim: ok=%v err=%v", ok, err)
	}

	if err := s.InsertAgentScheduleRun(ctx, AgentScheduleRun{
		ScheduleID: sc.ID, ProjectID: pid, AgentID: "ag1", ThreadID: "th1",
		Trigger: "scheduled", Status: AgentRunRunning, StartedAt: now, FinishedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertAgentScheduleRun(ctx, AgentScheduleRun{ID: uuid.NewString(), ScheduleID: sc.ID, ProjectID: pid, Trigger: "manual", Status: AgentRunQueued}); err != nil {
		t.Fatal(err)
	}
	runs, err := s.ListAgentScheduleRuns(ctx, pid, sc.ID, 0)
	if err != nil || len(runs) != 2 {
		t.Fatalf("runs: n=%d err=%v", len(runs), err)
	}
	fin := now.Add(2 * time.Second)
	if err := s.UpdateAgentScheduleRunStatus(ctx, pid, runs[0].ID, AgentRunCompleted, "", &fin); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateAgentScheduleRunStatus(ctx, pid, runs[1].ID, AgentRunFailed, "x", nil); err != nil {
		t.Fatal(err)
	}
	runs, err = s.ListAgentScheduleRuns(ctx, pid, sc.ID, 200)
	if err != nil || len(runs) != 2 {
		t.Fatalf("updated runs: n=%d err=%v", len(runs), err)
	}

	if err := s.ArchiveAgentSchedule(ctx, pid, sc.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ArchiveAgentSchedule(ctx, pid, sc.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
}

func TestCronAndGoFunctionRemainingBranches(t *testing.T) {
	ctx := context.Background()
	var nilStore *Store
	if _, err := nilStore.CreateCronJob(ctx, CronJob{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.GetCronJob(ctx, "p", "id"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.GetCronJobByName(ctx, "p", "n"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.ListCronJobs(ctx, "p"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.UpdateCronJob(ctx, CronJob{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	assertUnavailable(t, nilStore.ArchiveCronJob(ctx, "p", "id"))
	if _, err := nilStore.ListDueCronJobs(ctx, time.Now(), 0); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.ClaimCronJob(ctx, "id", time.Time{}, time.Time{}, time.Time{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.InsertCronJobRun(ctx, CronJobRun{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	assertUnavailable(t, nilStore.UpdateCronJobRunStatus(ctx, "p", "id", CronJobRunFailed, "", 0, "", nil))
	if _, err := nilStore.ListCronJobRuns(ctx, "p", "j", 0); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.ListGoFuncs(ctx, "p"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.GetGoFunc(ctx, "p", "n"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.CreateGoFunc(ctx, GoFunc{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if err := nilStore.UpdateGoFuncMeta(ctx, GoFunc{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	assertUnavailable(t, nilStore.ArchiveGoFunc(ctx, "p", "n"))
	if _, err := nilStore.CountGoFuncs(ctx, "p"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}

	s := newTestStore(t)
	if _, err := s.GetCronJob(ctx, "p", "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if _, err := s.GetCronJobByName(ctx, "p", "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if _, err := s.UpdateCronJob(ctx, CronJob{ID: "x", ProjectID: "p"}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if err := s.ArchiveCronJob(ctx, "p", "x"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if _, err := s.ListDueCronJobs(ctx, time.Now(), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListDueCronJobs(ctx, time.Now(), 900); err != nil {
		t.Fatal(err)
	}
	ok, err := s.ClaimCronJob(ctx, "missing", time.Time{}, time.Time{}, time.Time{})
	if err != nil || ok {
		t.Fatalf("claim missing: ok=%v err=%v", ok, err)
	}
	if _, err := s.ListCronJobRuns(ctx, "p", "j", 0); err != nil {
		t.Fatal(err)
	}

	j, err := s.CreateCronJob(ctx, CronJob{
		ID: uuid.NewString(), ProjectID: "p", Name: "with-times",
		ScheduleKind: CronJobKindInterval, IntervalSeconds: 30,
		FuncFile: "f", FuncExport: "E", Enabled: true,
		LastRunAt: time.Now(), NextRunAt: time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	fin := started.Add(time.Second)
	run, err := s.InsertCronJobRun(ctx, CronJobRun{
		ID: uuid.NewString(), JobID: j.ID, ProjectID: "p", Trigger: "manual",
		Status: CronJobRunRunning, StartedAt: started, FinishedAt: fin, ResponseJSON: `{"ok":1}`,
	})
	if err != nil || run.ID == "" {
		t.Fatal(err)
	}
	if err := s.UpdateCronJobRunStatus(ctx, "p", run.ID, CronJobRunFailed, "e", 9, "", nil); err != nil {
		t.Fatal(err)
	}

	if err := s.UpdateGoFuncMeta(ctx, GoFunc{ID: "missing", ProjectID: "p", Name: "missing"}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if _, err := s.CreateGoFunc(ctx, GoFunc{ID: uuid.NewString(), ProjectID: "p", Name: "fixed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendGoFuncVersion(ctx, GoFuncVersion{ProjectID: "p", Name: "fixed", Source: "package main"}); err != nil {
		t.Fatal(err)
	}

	if sqlval.NullString("") != nil || sqlval.NullString("x") == nil {
		t.Fatal("nullString")
	}
	if nullInt64IfZero(0) != nil || nullInt64IfZero(3) == nil {
		t.Fatal("nullInt64IfZero")
	}
}
