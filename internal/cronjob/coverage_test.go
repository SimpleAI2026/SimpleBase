package cronjob

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

type cronHooks struct {
	*fakeStore
	listErr   error
	insertErr error
	updateErr error
	getErr    error
	claimErr  error
}

func (c *cronHooks) ListDueCronJobs(ctx context.Context, now time.Time, limit int) ([]systemdb.CronJob, error) {
	if c.listErr != nil {
		return nil, c.listErr
	}
	return c.fakeStore.ListDueCronJobs(ctx, now, limit)
}

func (c *cronHooks) InsertCronJobRun(ctx context.Context, r systemdb.CronJobRun) (systemdb.CronJobRun, error) {
	if c.insertErr != nil {
		return systemdb.CronJobRun{}, c.insertErr
	}
	return c.fakeStore.InsertCronJobRun(ctx, r)
}

func (c *cronHooks) UpdateCronJob(ctx context.Context, j systemdb.CronJob) (systemdb.CronJob, error) {
	if c.updateErr != nil {
		return systemdb.CronJob{}, c.updateErr
	}
	return c.fakeStore.UpdateCronJob(ctx, j)
}

func (c *cronHooks) GetCronJob(ctx context.Context, projectID, id string) (systemdb.CronJob, error) {
	if c.getErr != nil {
		return systemdb.CronJob{}, c.getErr
	}
	return c.fakeStore.GetCronJob(ctx, projectID, id)
}

func (c *cronHooks) ClaimCronJob(ctx context.Context, id string, expectNext, newNext, lastRun time.Time) (bool, error) {
	if c.claimErr != nil {
		return false, c.claimErr
	}
	return c.fakeStore.ClaimCronJob(ctx, id, expectNext, newNext, lastRun)
}

func intervalJob(id string) systemdb.CronJob {
	now := time.Now().UTC()
	return systemdb.CronJob{
		ID: id, ProjectID: "p1", Name: id, ScheduleKind: systemdb.CronJobKindInterval,
		IntervalSeconds: 60, FuncFile: "f", FuncExport: "F",
		Enabled: true, NextRunAt: now.Add(-time.Minute), InputJSON: `{}`,
	}
}

func TestSchedulerStartTickAndHelpers(t *testing.T) {
	var nilS *Scheduler
	nilS.Start(context.Background())
	nilS.Stop()

	s := NewScheduler(nil, nil)
	s.Start(context.Background())
	s.Stop()

	fs := newFakeStore(intervalJob("job-1"))
	h := &cronHooks{fakeStore: fs, listErr: errors.New("list")}
	s = NewScheduler(h, &fakeRunner{})
	s.tick = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)
	time.Sleep(8 * time.Millisecond)
	cancel()
	s.Stop()
	if len(fs.logs) == 0 {
		t.Fatal("list error should log")
	}

	s.tick = 0
	ctx, cancel = context.WithCancel(context.Background())
	s.Start(ctx)
	cancel()
	s.Stop()

	h.listErr = nil
	s.tickOnce(context.Background())
	s.Stop()
	if (&fakeRunner{}).callCount() == 999 {
		t.Fatal("sanity")
	}

	h.claimErr = errors.New("cas")
	s.runDue(context.Background(), fs.job("job-1"))

	j := intervalJob("bad-int")
	j.IntervalSeconds = 0
	if _, err := s.nextRun(j, time.Now().UTC()); err == nil {
		t.Fatal("invalid interval")
	}
	if _, err := s.nextRun(systemdb.CronJob{ScheduleKind: systemdb.CronJobKindCron, CronExpr: "* * * * *"}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	s = &Scheduler{inFlight: nil}
	if !s.claimInFlight("a") || s.claimInFlight("a") {
		t.Fatal("inflight")
	}
	s.inFlight = map[string]struct{}{"1": {}, "2": {}, "3": {}, "4": {}}
	if s.claimInFlight("5") {
		t.Fatal("cap")
	}
	s.releaseInFlight("1")
	if !s.claimInFlight("5") {
		t.Fatal("released")
	}
	(&Scheduler{}).log("x", time.Now())
	if truncate("ab", 10) != "ab" || truncate("abcdef", 3) != "abc" {
		t.Fatal("truncate")
	}
}

func TestSchedulerExecuteBranches(t *testing.T) {
	t.Run("insert fails", func(t *testing.T) {
		fs := newFakeStore(intervalJob("j"))
		h := &cronHooks{fakeStore: fs, insertErr: errors.New("ins")}
		s := NewScheduler(h, &fakeRunner{})
		s.execute(context.Background(), fs.job("j"), TriggerScheduled)
		if len(fs.runList()) != 0 {
			t.Fatal("no run")
		}
	})
	t.Run("canceled", func(t *testing.T) {
		fs := newFakeStore(intervalJob("j"))
		s := NewScheduler(fs, &fakeRunner{err: errors.New("context canceled")})
		s.execute(context.Background(), fs.job("j"), TriggerManual)
		runs := fs.runList()
		if len(runs) != 1 || runs[0].Status != systemdb.CronJobRunCanceled {
			t.Fatalf("%+v", runs)
		}
	})
	t.Run("snapshot get fails uses old row", func(t *testing.T) {
		fs := newFakeStore(intervalJob("j"))
		h := &cronHooks{fakeStore: fs, getErr: errors.New("gone")}
		s := NewScheduler(h, &fakeRunner{})
		s.execute(context.Background(), fs.job("j"), TriggerScheduled)
		if fs.job("j").RunCount != 1 {
			t.Fatalf("%+v", fs.job("j"))
		}
	})
	t.Run("snapshot update fails", func(t *testing.T) {
		fs := newFakeStore(intervalJob("j"))
		h := &cronHooks{fakeStore: fs, updateErr: errors.New("upd")}
		s := NewScheduler(h, &fakeRunner{})
		s.execute(context.Background(), fs.job("j"), TriggerScheduled)
		if len(fs.logs) == 0 {
			t.Fatal("update snapshot error logged")
		}
	})
	t.Run("disable update fails", func(t *testing.T) {
		fs := newFakeStore(intervalJob("j"))
		h := &cronHooks{fakeStore: fs, updateErr: errors.New("upd")}
		s := NewScheduler(h, &fakeRunner{})
		s.disableJob(context.Background(), fs.job("j"), "bad cron")
		if !strings.Contains(strings.Join(fs.logs, "\n"), "update failed") {
			t.Fatalf("logs=%v", fs.logs)
		}
	})
	t.Run("disable success", func(t *testing.T) {
		j := intervalJob("j")
		fs := newFakeStore(j)
		s := NewScheduler(fs, &fakeRunner{})
		s.disableJob(context.Background(), j, "stop")
		if fs.job("j").Enabled {
			t.Fatal("disabled")
		}
	})
	t.Run("truncated response", func(t *testing.T) {
		j := intervalJob("j")
		fs := newFakeStore(j)
		s := NewScheduler(fs, &fakeRunner{response: []byte(strings.Repeat("z", runResponseLimit+10))})
		s.execute(context.Background(), j, TriggerManual)
		runs := fs.runList()
		if len(runs[0].ResponseJSON) != runResponseLimit {
			t.Fatalf("len=%d", len(runs[0].ResponseJSON))
		}
	})
}
