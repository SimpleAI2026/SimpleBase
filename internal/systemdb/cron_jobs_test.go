package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestCronJobCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pid := "proj-1"

	next := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	created, err := s.CreateCronJob(ctx, CronJob{
		ProjectID: pid, Name: "nightly-refresh", Description: "每晚汇总",
		ScheduleKind: CronJobKindCron, CronExpr: "0 2 * * *",
		FuncFile: "hello", FuncExport: "Hello", InputJSON: `{"name":"cron"}`,
		Enabled: true, NextRunAt: next,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || !created.Enabled || created.NextRunAt.IsZero() {
		t.Fatalf("unexpected created row: %+v", created)
	}

	// 项目惯例：唯一性由 handler 先查后写保证（同 gofunctions），store 不建唯一索引；
	// 同名第二条可插入（GetByName 只取最早一条），此处仅验证插入成功。
	dup, err := s.CreateCronJob(ctx, CronJob{ProjectID: pid, Name: "nightly-refresh",
		ScheduleKind: CronJobKindInterval, IntervalSeconds: 600, FuncFile: "hello", FuncExport: "Hello"})
	if err != nil {
		t.Fatalf("store allows duplicates (handler guards): %v", err)
	}
	_ = s.ArchiveCronJob(ctx, pid, dup.ID)

	got, err := s.GetCronJob(ctx, pid, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "nightly-refresh" || got.CronExpr != "0 2 * * *" || got.InputJSON != `{"name":"cron"}` {
		t.Fatalf("unexpected get row: %+v", got)
	}

	byName, err := s.GetCronJobByName(ctx, pid, "nightly-refresh")
	if err != nil || byName.ID != created.ID {
		t.Fatalf("get by name: %+v err=%v", byName, err)
	}

	list, err := s.ListCronJobs(ctx, pid)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %+v err=%v", list, err)
	}

	// interval 模式更新：cron 字段清空、interval 写入
	got.ScheduleKind = CronJobKindInterval
	got.CronExpr = ""
	got.IntervalSeconds = 600
	updated, err := s.UpdateCronJob(ctx, got)
	if err != nil {
		t.Fatal(err)
	}
	if updated.CronExpr != "" || updated.IntervalSeconds != 600 {
		t.Fatalf("unexpected updated row: %+v", updated)
	}

	// 软删后同名可复建
	if err := s.ArchiveCronJob(ctx, pid, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetCronJob(ctx, pid, created.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("want ErrNoRows after archive, got %v", err)
	}
	if _, err := s.CreateCronJob(ctx, CronJob{ProjectID: pid, Name: "nightly-refresh",
		ScheduleKind: CronJobKindCron, CronExpr: "0 3 * * *", FuncFile: "hello", FuncExport: "Hello"}); err != nil {
		t.Fatalf("recreate after archive: %v", err)
	}
}

func TestCronJobDueAndClaim(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pid := "proj-1"
	now := time.Now().UTC().Truncate(time.Second)
	due := now.Add(-time.Minute)
	future := now.Add(time.Hour)

	mk := func(name string, next time.Time, enabled bool) CronJob {
		j, err := s.CreateCronJob(ctx, CronJob{
			ProjectID: pid, Name: name, ScheduleKind: CronJobKindInterval, IntervalSeconds: 600,
			FuncFile: "hello", FuncExport: "Hello", Enabled: enabled, NextRunAt: next,
		})
		if err != nil {
			t.Fatal(err)
		}
		return j
	}
	a := mk("due-a", due, true)
	mk("future-b", future, true)
	mk("disabled-c", due, false)

	dueList, err := s.ListDueCronJobs(ctx, now, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(dueList) != 1 || dueList[0].ID != a.ID {
		t.Fatalf("want only due-a, got %+v", dueList)
	}

	// CAS 认领成功
	newNext := now.Add(10 * time.Minute)
	ok, err := s.ClaimCronJob(ctx, a.ID, a.NextRunAt, newNext, now)
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	got, _ := s.GetCronJob(ctx, pid, a.ID)
	if !got.LastRunAt.Equal(now) || !got.NextRunAt.Equal(newNext) {
		t.Fatalf("after claim: last=%v next=%v", got.LastRunAt, got.NextRunAt)
	}

	// 旧 expectNext 再认领 → false
	ok, err = s.ClaimCronJob(ctx, a.ID, a.NextRunAt, newNext.Add(10*time.Minute), now)
	if err != nil || ok {
		t.Fatalf("stale claim should fail: ok=%v err=%v", ok, err)
	}
}

func TestCronJobRuns(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pid := "proj-1"
	j, err := s.CreateCronJob(ctx, CronJob{
		ProjectID: pid, Name: "j1", ScheduleKind: CronJobKindInterval, IntervalSeconds: 60,
		FuncFile: "hello", FuncExport: "Hello", Enabled: true, NextRunAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	started := time.Now().UTC()
	inserted, err := s.InsertCronJobRun(ctx, CronJobRun{
		JobID: j.ID, ProjectID: pid, Trigger: "scheduled", Status: CronJobRunRunning, StartedAt: started,
	})
	if err != nil {
		t.Fatal(err)
	}
	if inserted.ID == "" {
		t.Fatal("inserted run should have ID")
	}
	runs, err := s.ListCronJobRuns(ctx, pid, j.ID, 20)
	if err != nil || len(runs) != 1 || runs[0].Status != CronJobRunRunning {
		t.Fatalf("runs: %+v err=%v", runs, err)
	}

	fin := started.Add(time.Second * 2)
	if err := s.UpdateCronJobRunStatus(ctx, pid, runs[0].ID, CronJobRunCompleted, "", 2031, `{"ok":true}`, &fin); err != nil {
		t.Fatal(err)
	}
	runs, _ = s.ListCronJobRuns(ctx, pid, j.ID, 20)
	if runs[0].Status != CronJobRunCompleted || runs[0].DurationMs != 2031 || runs[0].ResponseJSON != `{"ok":true}` {
		t.Fatalf("updated run: %+v", runs[0])
	}
	if runs[0].FinishedAt.IsZero() || runs[0].StartedAt.IsZero() {
		t.Fatalf("timestamps missing: %+v", runs[0])
	}
}
