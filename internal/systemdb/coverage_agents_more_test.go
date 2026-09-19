package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

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
	if _, err := nilStore.ListAgentThreads(ctx, "p", 0); !errors.Is(err, ErrUnavailable) {
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
	n, err := s.CountCloudAgents(ctx, pid)
	if err != nil || n != 2 {
		t.Fatalf("count existing: %d err=%v", n, err)
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
	threads, err := s.ListAgentThreads(ctx, pid, 0)
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
	if err != nil || n != 3 {
		t.Fatalf("seeded n=%d err=%v", n, err)
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
	if _, err := nilStore.ListGoFunctions(ctx, "p"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.GetGoFunction(ctx, "p", "n"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.CreateGoFunction(ctx, GoFunction{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.UpdateGoFunction(ctx, GoFunction{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	assertUnavailable(t, nilStore.ArchiveGoFunction(ctx, "p", "n"))
	if _, err := nilStore.CountGoFunctions(ctx, "p"); !errors.Is(err, ErrUnavailable) {
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

	if _, err := s.UpdateGoFunction(ctx, GoFunction{ProjectID: "p", Name: "missing"}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if _, err := s.CreateGoFunction(ctx, GoFunction{ID: uuid.NewString(), ProjectID: "p", Name: "fixed", Source: "package main"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO sys_gofunctions(id, project_id, name, source, exports_json, created_at, updated_at, archived_at)
		VALUES(?, 'p', 'badjson', '', '{', ?, ?, NULL)`, uuid.NewString(), time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetGoFunction(ctx, "p", "badjson"); err == nil {
		t.Fatal("bad exports json")
	}

	if nullString("") != nil || nullString("x") == nil {
		t.Fatal("nullString")
	}
	if nullInt64IfZero(0) != nil || nullInt64IfZero(3) == nil {
		t.Fatal("nullInt64IfZero")
	}
}
