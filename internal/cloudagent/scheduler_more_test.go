package cloudagent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

type schedHooks struct {
	*fakeScheduleStore
	listErr         error
	insertErr       error
	createThreadErr error
	updateErr       error
	createRunErr    error
	appendErr       error
	claimErr        error
	listMsgErr      error
}

func (s *schedHooks) ListDueAgentSchedules(ctx context.Context, now time.Time, limit int) ([]systemdb.AgentSchedule, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.fakeScheduleStore.ListDueAgentSchedules(ctx, now, limit)
}

func (s *schedHooks) InsertAgentScheduleRun(ctx context.Context, r systemdb.AgentScheduleRun) error {
	if s.insertErr != nil {
		return s.insertErr
	}
	return s.fakeScheduleStore.InsertAgentScheduleRun(ctx, r)
}

func (s *schedHooks) CreateAgentThread(ctx context.Context, t systemdb.AgentThread) (systemdb.AgentThread, error) {
	if s.createThreadErr != nil {
		return systemdb.AgentThread{}, s.createThreadErr
	}
	return s.fakeScheduleStore.CreateAgentThread(ctx, t)
}

func (s *schedHooks) UpdateAgentSchedule(ctx context.Context, sc systemdb.AgentSchedule) (systemdb.AgentSchedule, error) {
	if s.updateErr != nil {
		return systemdb.AgentSchedule{}, s.updateErr
	}
	return s.fakeScheduleStore.UpdateAgentSchedule(ctx, sc)
}

func (s *schedHooks) CreateAgentRun(ctx context.Context, r systemdb.AgentRun) (systemdb.AgentRun, error) {
	if s.createRunErr != nil {
		return systemdb.AgentRun{}, s.createRunErr
	}
	return s.fakeScheduleStore.CreateAgentRun(ctx, r)
}

func (s *schedHooks) AppendAgentMessage(ctx context.Context, m systemdb.AgentMessage) (systemdb.AgentMessage, error) {
	if s.appendErr != nil {
		return systemdb.AgentMessage{}, s.appendErr
	}
	return s.fakeScheduleStore.AppendAgentMessage(ctx, m)
}

func (s *schedHooks) ClaimAgentSchedule(ctx context.Context, id string, expectNext, newNext, lastRun time.Time) (bool, error) {
	if s.claimErr != nil {
		return false, s.claimErr
	}
	return s.fakeScheduleStore.ClaimAgentSchedule(ctx, id, expectNext, newNext, lastRun)
}

func (s *schedHooks) ListAgentMessages(ctx context.Context, projectID, threadID string, limit int) ([]systemdb.AgentMessage, error) {
	if s.listMsgErr != nil {
		return nil, s.listMsgErr
	}
	return s.fakeScheduleStore.ListAgentMessages(ctx, projectID, threadID, limit)
}

func TestSchedulerStartStopAndTickErrors(t *testing.T) {
	var nilS *Scheduler
	nilS.Start(context.Background())
	nilS.Stop()

	s := NewScheduler(nil, nil, nil)
	s.Start(context.Background())
	s.Stop()

	store := newFakeScheduleStore()
	store.agents["a1"] = systemdb.CloudAgent{ID: "a1", ProjectID: "p1", Name: "A", Module: "database"}
	store.threads["t1"] = systemdb.AgentThread{ID: "t1", ProjectID: "p1"}
	sc := seedSchedule(store)
	_ = sc
	hooks := &schedHooks{fakeScheduleStore: store, listErr: errors.New("list-fail")}
	runner := &fakeRunner{}
	s = NewScheduler(hooks, runner, nil)
	s.tick = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)
	time.Sleep(8 * time.Millisecond)
	cancel()
	s.Stop()

	s.tick = 0
	ctx, cancel = context.WithCancel(context.Background())
	s.Start(ctx)
	cancel()
	s.Stop()

	s.tickOnce(context.Background())
	if len(store.logs) == 0 {
		t.Fatal("list due error should be logged")
	}

	hooks.listErr = nil
	s.tickOnce(context.Background())
	s.Stop()
	if len(runner.calls) == 0 {
		t.Fatal("due schedule should run")
	}
}

func TestSchedulerExecuteErrorBranches(t *testing.T) {
	base := func() (*schedHooks, *fakeRunner, *Scheduler) {
		store := newFakeScheduleStore()
		store.agents["a1"] = systemdb.CloudAgent{ID: "a1", ProjectID: "p1", Name: "A", Module: "database"}
		store.threads["t1"] = systemdb.AgentThread{ID: "t1", ProjectID: "p1"}
		sc := seedSchedule(store)
		_ = sc
		hooks := &schedHooks{fakeScheduleStore: store}
		runner := &fakeRunner{}
		return hooks, runner, NewScheduler(hooks, runner, nil)
	}

	t.Run("insert run fails", func(t *testing.T) {
		h, r, s := base()
		h.insertErr = errors.New("insert")
		s.execute(context.Background(), h.schedules["sc-1"], TriggerScheduled)
		if len(r.calls) != 0 {
			t.Fatal("should not run")
		}
	})
	t.Run("create run fails", func(t *testing.T) {
		h, _, s := base()
		h.createRunErr = errors.New("run")
		s.execute(context.Background(), h.schedules["sc-1"], TriggerScheduled)
		got, _ := h.lastSchedRun()
		if got.Status != systemdb.AgentRunFailed {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("append user fails", func(t *testing.T) {
		h, _, s := base()
		h.appendErr = errors.New("msg")
		s.execute(context.Background(), h.schedules["sc-1"], TriggerScheduled)
		got, _ := h.lastSchedRun()
		if got.Error != "append user message failed" {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("list messages error still runs", func(t *testing.T) {
		h, r, s := base()
		h.listMsgErr = errors.New("hist")
		s.execute(context.Background(), h.schedules["sc-1"], TriggerScheduled)
		if len(r.calls) != 1 {
			t.Fatal("should still start run")
		}
	})
	t.Run("runner failed and canceled", func(t *testing.T) {
		h, r, s := base()
		r.err = errors.New("boom")
		s.execute(context.Background(), h.schedules["sc-1"], TriggerScheduled)
		got, _ := h.lastSchedRun()
		if got.Status != systemdb.AgentRunFailed {
			t.Fatalf("%+v", got)
		}
		h2, r2, s2 := base()
		r2.err = errors.New("context canceled")
		s2.execute(context.Background(), h2.schedules["sc-1"], TriggerScheduled)
		got, _ = h2.lastSchedRun()
		if got.Status != systemdb.AgentRunCanceled {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("recreate thread fails", func(t *testing.T) {
		store := newFakeScheduleStore()
		store.agents["a1"] = systemdb.CloudAgent{ID: "a1", ProjectID: "p1", Name: "A"}
		sc := seedSchedule(store)
		h := &schedHooks{fakeScheduleStore: store, createThreadErr: errors.New("no thread")}
		s := NewScheduler(h, &fakeRunner{}, nil)
		s.execute(context.Background(), sc, TriggerManual)
		got, _ := h.lastSchedRun()
		if got.Error != "schedule thread unavailable" {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("recreate thread update fails", func(t *testing.T) {
		store := newFakeScheduleStore()
		store.agents["a1"] = systemdb.CloudAgent{ID: "a1", ProjectID: "p1", Name: "A"}
		sc := seedSchedule(store)
		h := &schedHooks{fakeScheduleStore: store, updateErr: errors.New("upd")}
		s := NewScheduler(h, &fakeRunner{}, nil)
		s.execute(context.Background(), sc, TriggerManual)
		s.Stop()
	})
	t.Run("disable schedule update fails", func(t *testing.T) {
		store := newFakeScheduleStore()
		sc := seedSchedule(store)
		h := &schedHooks{fakeScheduleStore: store, updateErr: errors.New("upd")}
		s := NewScheduler(h, &fakeRunner{}, nil)
		s.disableSchedule(context.Background(), sc)
	})
}

func TestSchedulerRunDueNextAfterAndClaimError(t *testing.T) {
	store := newFakeScheduleStore()
	sc := testSchedule()
	sc.CronExpr = "0 0 30 2 *"
	store.schedules[sc.ID] = sc
	h := &schedHooks{fakeScheduleStore: store}
	s := NewScheduler(h, &fakeRunner{}, nil)
	s.runDue(context.Background(), sc)
	if store.schedules[sc.ID].Enabled {
		t.Fatal("impossible cron should disable")
	}

	store = newFakeScheduleStore()
	store.agents["a1"] = systemdb.CloudAgent{ID: "a1", ProjectID: "p1", Name: "A"}
	sc = seedSchedule(store)
	h = &schedHooks{fakeScheduleStore: store, claimErr: errors.New("cas")}
	s = NewScheduler(h, &fakeRunner{}, nil)
	s.runDue(context.Background(), sc)
}

func TestSchedulerTriggerAndInFlightHelpers(t *testing.T) {
	var s *Scheduler
	if _, err := s.Trigger(context.Background(), testSchedule()); err == nil {
		t.Fatal("nil scheduler")
	}
	s = NewScheduler(nil, &fakeRunner{}, nil)
	if _, err := s.Trigger(context.Background(), testSchedule()); err == nil {
		t.Fatal("no store")
	}

	s = &Scheduler{inFlight: nil}
	if !s.claimInFlight("a") {
		t.Fatal("nil map")
	}
	if s.claimInFlight("a") {
		t.Fatal("duplicate")
	}
	s.inFlight = map[string]struct{}{"1": {}, "2": {}, "3": {}, "4": {}}
	if s.claimInFlight("5") {
		t.Fatal("concurrency cap")
	}
	s.releaseInFlight("1")
	if !s.claimInFlight("5") {
		t.Fatal("after release")
	}

	s = &Scheduler{}
	s.log("x", time.Now())
	if sanitizeErr(nil) != "" {
		t.Fatal("nil err")
	}
	long := strings.Repeat("e", 400)
	if got := sanitizeErr(errors.New(long)); len(got) != 300 {
		t.Fatalf("len=%d", len(got))
	}
}
