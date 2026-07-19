package catalog

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/uglyer/go-sqlite3"
)

func newTestRepoForJobs(t *testing.T) Repository {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := ApplyMigrations(context.Background(), db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return NewSQLiteRepository(db)
}

func TestJobQueueEnqueueClaimComplete(t *testing.T) {
	repo := newTestRepoForJobs(t)
	ctx := context.Background()
	jobID := uuid.NewString()
	job := Job{
		ID:         jobID,
		DatabaseID: "db-1",
		ProjectID:  "proj-1",
		Type:       JobTypeDeleteDatabase,
		Status:     JobStatusPending,
		MaxAttempts: 3,
		RunAfter:    time.Now().UTC(),
	}
	if err := repo.Enqueue(ctx, job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Claim
	claimed, err := repo.Claim(ctx, "worker-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if claimed.ID != jobID {
		t.Fatalf("claimed wrong job: %s", claimed.ID)
	}
	if claimed.Status != JobStatusRunning {
		t.Fatalf("status not running: %s", claimed.Status)
	}
	if claimed.Attempt != 1 {
		t.Fatalf("attempt not 1: %d", claimed.Attempt)
	}

	// Complete
	if err := repo.Complete(ctx, jobID); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	got, err := repo.GetJob(ctx, jobID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.Status != JobStatusCompleted {
		t.Fatalf("status not completed: %s", got.Status)
	}
}

func TestJobQueueRetryThenDeadLetter(t *testing.T) {
	repo := newTestRepoForJobs(t)
	ctx := context.Background()
	jobID := uuid.NewString()
	job := Job{
		ID:          jobID,
		DatabaseID:  "db-1",
		ProjectID:   "proj-1",
		Type:        JobTypeBackup,
		Status:      JobStatusPending,
		MaxAttempts: 2,
		RunAfter:    time.Now().UTC(),
	}
	repo.Enqueue(ctx, job)

	// 第一次 Claim + Retry
	claimed, _ := repo.Claim(ctx, "w1", time.Now().UTC())
	if err := repo.Retry(ctx, claimed.ID, "fail-1", time.Now().UTC().Add(time.Second)); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	got, _ := repo.GetJob(ctx, jobID)
	if got.Status != JobStatusPending {
		t.Fatalf("expected pending after retry, got %s", got.Status)
	}

	// 第二次 Claim + Retry → 应 dead_letter（attempt=2 >= max=2）
	claimed2, _ := repo.Claim(ctx, "w1", time.Now().UTC().Add(time.Second))
	if claimed2.Attempt != 2 {
		t.Fatalf("expected attempt 2, got %d", claimed2.Attempt)
	}
	if err := repo.Retry(ctx, claimed2.ID, "fail-2", time.Now().UTC()); err != nil {
		t.Fatalf("Retry 2: %v", err)
	}
	got2, _ := repo.GetJob(ctx, jobID)
	if got2.Status != JobStatusDeadLetter {
		t.Fatalf("expected dead_letter, got %s", got2.Status)
	}
}

func TestJobQueueListJobs(t *testing.T) {
	repo := newTestRepoForJobs(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		repo.Enqueue(ctx, Job{
			ID:         uuid.NewString(),
			DatabaseID: "db-1",
			ProjectID:  "proj-1",
			Type:       JobTypeDeleteDatabase,
			MaxAttempts: 3,
			RunAfter:    time.Now().UTC(),
		})
	}
	// 不同 databaseID
	repo.Enqueue(ctx, Job{
		ID:         uuid.NewString(),
		DatabaseID: "db-2",
		ProjectID:  "proj-1",
		Type:       JobTypeDeleteDatabase,
		MaxAttempts: 3,
		RunAfter:    time.Now().UTC(),
	})
	jobs, err := repo.ListJobs(ctx, "db-1", 10)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}
}
