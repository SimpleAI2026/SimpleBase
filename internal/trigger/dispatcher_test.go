package trigger

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/functionrun"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

type fakeStore struct{ triggers []systemdb.FunctionTrigger }

func (s fakeStore) ListFunctionTriggers(context.Context, string) ([]systemdb.FunctionTrigger, error) {
	return s.triggers, nil
}

type fakeRunner struct {
	calls   atomic.Int64
	started chan struct{}
	release chan struct{}
}

func (r *fakeRunner) Run(ctx context.Context, request functionrun.Request) (functionrun.Result, error) {
	call := r.calls.Add(1)
	if r.started != nil && call == 1 {
		close(r.started)
	}
	if r.release != nil && call == 1 {
		select {
		case <-r.release:
		case <-ctx.Done():
		}
	}
	return functionrun.Result{}, errors.New("failed")
}

func TestDispatcherRetriesThreeTimes(t *testing.T) {
	runner := &fakeRunner{}
	dispatcher := NewDispatcher(fakeStore{}, runner, 4, 1)
	dispatcher.retryBase = time.Millisecond
	trigger := systemdb.FunctionTrigger{ID: "t1", ProjectID: "p1", Kind: "http", FuncFile: "handler", FuncExport: "Handle", Enabled: true}
	if err := dispatcher.Invoke(trigger, map[string]any{"value": 1}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for runner.calls.Load() < 4 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	dispatcher.Close()
	if got := runner.calls.Load(); got != 4 {
		t.Fatalf("calls=%d, want initial attempt plus three retries", got)
	}
}

func TestDispatcherQueueFullAndObjectPrefix(t *testing.T) {
	runner := &fakeRunner{started: make(chan struct{}), release: make(chan struct{})}
	dispatcher := NewDispatcher(fakeStore{}, runner, 1, 1)
	trigger := systemdb.FunctionTrigger{ID: "t1", ProjectID: "p1", Kind: "http", FuncFile: "handler", FuncExport: "Handle", Enabled: true}
	if err := dispatcher.Invoke(trigger, nil); err != nil {
		t.Fatal(err)
	}
	<-runner.started
	if err := dispatcher.Invoke(trigger, nil); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Invoke(trigger, nil); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("queue error=%v", err)
	}
	close(runner.release)
	dispatcher.Close()

	success := &fakeRunner{}
	store := fakeStore{triggers: []systemdb.FunctionTrigger{{ProjectID: "p1", Kind: "object_event", Target: "images/", Events: []string{"upload"}, FuncFile: "handler", FuncExport: "Handle", Enabled: true}}}
	dispatcher = NewDispatcher(store, success, 4, 1)
	dispatcher.PublishObject(Event{Kind: "object_event", ProjectID: "p1", Target: "images/a.png", Action: "upload"})
	dispatcher.PublishObject(Event{Kind: "object_event", ProjectID: "p1", Target: "docs/a.txt", Action: "upload"})
	deadline := time.Now().Add(time.Second)
	for success.calls.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	dispatcher.Close()
	if success.calls.Load() != 1 {
		t.Fatalf("prefix calls=%d", success.calls.Load())
	}
}

func TestTriggerFilterValidation(t *testing.T) {
	for _, filter := range []string{"", `status="open"`, `priority=2`, `active=true`} {
		if !ValidateFilter(filter) {
			t.Fatalf("rejected %q", filter)
		}
	}
	for _, filter := range []string{"status", `status=`, `a.b=1`} {
		if ValidateFilter(filter) {
			t.Fatalf("accepted %q", filter)
		}
	}
	if !matchesFilter(`status="open"`, map[string]any{"status": "open"}) || matchesFilter(`status="open"`, map[string]any{"status": "closed"}) {
		t.Fatal("unexpected filter result")
	}
}
