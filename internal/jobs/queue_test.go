package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/observability"
)

// fakeQueue 是内存版 Queue，用于测试 worker。
type fakeQueue struct {
	mu        sync.Mutex
	jobs      map[string]catalog.Job
	claimOrder []string
}

func newFakeQueue() *fakeQueue {
	return &fakeQueue{jobs: map[string]catalog.Job{}}
}

func (q *fakeQueue) Enqueue(ctx context.Context, job catalog.Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs[job.ID] = job
	q.claimOrder = append(q.claimOrder, job.ID)
	return nil
}

func (q *fakeQueue) Claim(ctx context.Context, workerID string, now time.Time) (catalog.Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	// 每次从头扫描，找到最早的 pending 任务（支持重试后重新领取）。
	for _, id := range q.claimOrder {
		j := q.jobs[id]
		if j.Status == catalog.JobStatusPending && (j.RunAfter.IsZero() || !now.Before(j.RunAfter)) {
			j.Status = catalog.JobStatusRunning
			j.Attempt++
			q.jobs[id] = j
			return j, nil
		}
	}
	return catalog.Job{}, catalog.ErrNotFound
}

func (q *fakeQueue) Complete(ctx context.Context, id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	j, ok := q.jobs[id]
	if !ok {
		return catalog.ErrNotFound
	}
	j.Status = catalog.JobStatusCompleted
	q.jobs[id] = j
	return nil
}

func (q *fakeQueue) Retry(ctx context.Context, id string, lastErr string, next time.Time) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	j, ok := q.jobs[id]
	if !ok {
		return catalog.ErrNotFound
	}
	if j.Attempt >= j.MaxAttempts {
		j.Status = catalog.JobStatusDeadLetter
	} else {
		j.Status = catalog.JobStatusPending
		j.LastError = lastErr
		j.RunAfter = next
	}
	q.jobs[id] = j
	return nil
}

// fakeHandler 是测试用 Handler。
type fakeHandler struct {
	typ     catalog.JobType
	execErr error
	calls   int
	mu      sync.Mutex
}

func (h *fakeHandler) Type() catalog.JobType { return h.typ }
func (h *fakeHandler) Execute(ctx context.Context, job catalog.Job) error {
	h.mu.Lock()
	h.calls++
	h.mu.Unlock()
	return h.execErr
}

func (h *fakeHandler) callCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls
}

func TestEnqueuerEnqueue(t *testing.T) {
	q := newFakeQueue()
	e := NewEnqueuer(q)
	id, err := e.Enqueue(context.Background(), EnqueueInput{
		DatabaseID: "db-1",
		ProjectID:  "proj-1",
		Type:       catalog.JobTypeDeleteDatabase,
		PayloadJSON: `{"storage_prefix":"test"}`,
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if id == "" {
		t.Fatal("empty job id")
	}
	job, ok := q.jobs[id]
	if !ok {
		t.Fatal("job not stored")
	}
	if job.Type != catalog.JobTypeDeleteDatabase {
		t.Fatalf("type mismatch: %s", job.Type)
	}
	if job.Status != catalog.JobStatusPending {
		t.Fatalf("status mismatch: %s", job.Status)
	}
}

func TestEnqueuerRejectsMissingFields(t *testing.T) {
	q := newFakeQueue()
	e := NewEnqueuer(q)
	_, err := e.Enqueue(context.Background(), EnqueueInput{ProjectID: "p1"})
	if err == nil {
		t.Fatal("expected error for missing database_id")
	}
	_, err = e.Enqueue(context.Background(), EnqueueInput{DatabaseID: "d1"})
	if err == nil {
		t.Fatal("expected error for missing type")
	}
}

func TestWorkerCompletesJob(t *testing.T) {
	q := newFakeQueue()
	e := NewEnqueuer(q)
	id, _ := e.Enqueue(context.Background(), EnqueueInput{
		DatabaseID: "db-1",
		Type:       catalog.JobTypeDeleteDatabase,
	})
	h := &fakeHandler{typ: catalog.JobTypeDeleteDatabase}
	w, err := NewWorker(q, []Handler{h}, Options{
		Logger:    observability.NewLogger("error", "console", nil),
		PollEvery: 10 * time.Millisecond,
		BaseDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewWorker: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go w.Start(ctx)
	// 等待任务完成。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		q.mu.Lock()
		j := q.jobs[id]
		q.mu.Unlock()
		if j.Status == catalog.JobStatusCompleted {
			cancel()
			w.Stop()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	w.Stop()
	t.Fatalf("job not completed, status: %s", q.jobs[id].Status)
}

func TestWorkerRetriesOnFailure(t *testing.T) {
	q := newFakeQueue()
	e := NewEnqueuer(q)
	id, _ := e.Enqueue(context.Background(), EnqueueInput{
		DatabaseID:  "db-1",
		Type:        catalog.JobTypeDeleteDatabase,
		MaxAttempts: 3,
	})
	// 第一次失败，后续成功。
	h := &fakeHandler{typ: catalog.JobTypeDeleteDatabase, execErr: errors.New("transient")}
	w, _ := NewWorker(q, []Handler{h}, Options{
		Logger:    observability.NewLogger("error", "console", nil),
		PollEvery: 10 * time.Millisecond,
		BaseDelay: 5 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	go w.Start(ctx)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if h.callCount() >= 1 {
			h.mu.Lock()
			h.execErr = nil
			h.mu.Unlock()
		}
		q.mu.Lock()
		j := q.jobs[id]
		q.mu.Unlock()
		if j.Status == catalog.JobStatusCompleted {
			cancel()
			w.Stop()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	w.Stop()
	t.Fatalf("job not completed after retry, status: %s", q.jobs[id].Status)
}

func TestWorkerDeadLetterAfterMaxAttempts(t *testing.T) {
	q := newFakeQueue()
	e := NewEnqueuer(q)
	id, _ := e.Enqueue(context.Background(), EnqueueInput{
		DatabaseID:  "db-1",
		Type:        catalog.JobTypeDeleteDatabase,
		MaxAttempts: 2,
	})
	h := &fakeHandler{typ: catalog.JobTypeDeleteDatabase, execErr: errors.New("permanent")}
	w, _ := NewWorker(q, []Handler{h}, Options{
		Logger:    observability.NewLogger("error", "console", nil),
		PollEvery: 10 * time.Millisecond,
		BaseDelay: 1 * time.Millisecond,
		MaxDelay:  5 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	go w.Start(ctx)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		q.mu.Lock()
		j := q.jobs[id]
		q.mu.Unlock()
		if j.Status == catalog.JobStatusDeadLetter {
			cancel()
			w.Stop()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	w.Stop()
	t.Fatalf("job not dead-lettered, status: %s", q.jobs[id].Status)
}
