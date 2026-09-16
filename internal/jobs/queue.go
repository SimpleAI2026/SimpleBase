// Package jobs 实现后台任务队列与 worker（plan7.md）。
//
// 设计：
//   - Queue 基于 catalog.Repository.JobQueue，持久化于 catalog 系统数据库。
//   - 单实例 worker 以轮询方式 Claim 任务；任务必须幂等，使用 operation ID 与状态机防止重复执行。
//   - 失败指数退避，有最大重试次数与 dead_letter 终态。
//   - 任务处理器（backup/restore/verify_recovery 等）实现 Handler 接口。
//     删库已改为 API 同步清理，不再使用 delete_database handler。
package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"go.uber.org/zap"
)

// Handler 处理一类后台任务。实现必须幂等：任意中断可安全重试。
type Handler interface {
	// Type 返回此 handler 处理的 JobType。
	Type() catalog.JobType
	// Execute 执行任务。返回 error 表示失败（将进入 Retry 流程）。
	Execute(ctx context.Context, job catalog.Job) error
}

// Queue 是任务队列的接口抽象，catalog.Repository 自动满足。
type Queue interface {
	Enqueue(ctx context.Context, job catalog.Job) error
	Claim(ctx context.Context, workerID string, now time.Time) (catalog.Job, error)
	Complete(ctx context.Context, id string) error
	Retry(ctx context.Context, id string, lastErr string, next time.Time) error
}

// EnqueueInput 是提交任务的简化输入。
type EnqueueInput struct {
	OperationID string
	DatabaseID  string
	ProjectID   string
	Type        catalog.JobType
	PayloadJSON string
	// RunAfter 延迟执行时间；零值表示立即。延迟用于删除保留期。
	RunAfter    time.Time
	MaxAttempts int
}

// Enqueuer 提供提交任务的便捷方法，自动生成 ID 与默认值。
type Enqueuer struct {
	queue Queue
}

// NewEnqueuer 构造 Enqueuer。
func NewEnqueuer(q Queue) *Enqueuer {
	return &Enqueuer{queue: q}
}

// Enqueue 提交一个 pending 任务。返回生成的 job ID。
func (e *Enqueuer) Enqueue(ctx context.Context, in EnqueueInput) (string, error) {
	if in.DatabaseID == "" {
		return "", errors.New("jobs: database_id required")
	}
	if in.Type == "" {
		return "", errors.New("jobs: job type required")
	}
	id := uuid.NewString()
	maxAttempts := in.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	runAfter := in.RunAfter
	if runAfter.IsZero() {
		runAfter = time.Now().UTC()
	}
	job := catalog.Job{
		ID:          id,
		OperationID: in.OperationID,
		DatabaseID:  in.DatabaseID,
		ProjectID:   in.ProjectID,
		Type:        in.Type,
		Status:      catalog.JobStatusPending,
		MaxAttempts: maxAttempts,
		RunAfter:    runAfter,
		PayloadJSON: in.PayloadJSON,
	}
	if err := e.queue.Enqueue(ctx, job); err != nil {
		return "", fmt.Errorf("jobs: enqueue %s: %w", in.Type, err)
	}
	return id, nil
}

// Worker 是单实例后台任务执行器。
type Worker struct {
	queue    Queue
	handlers map[catalog.JobType]Handler
	logger   observability.Logger
	metrics  JobMetrics

	workerID  string
	pollEvery time.Duration
	baseDelay time.Duration // 指数退避基准
	maxDelay  time.Duration

	stop chan struct{}
	done chan struct{}
}

// JobMetrics 是 observability.Metrics 的最小子集。
type JobMetrics interface {
	IncJobTotal(jobType string, outcome string)
	ObserveJobDuration(jobType string, seconds float64)
}

// Options 配置 Worker。
type Options struct {
	WorkerID  string
	PollEvery time.Duration // <=0 默认 2s
	BaseDelay time.Duration // <=0 默认 10s
	MaxDelay  time.Duration // <=0 默认 10min
	Logger    observability.Logger
	Metrics   JobMetrics
}

// NewWorker 构造 Worker。handlers 至少包含一个 Handler。
func NewWorker(q Queue, handlers []Handler, opts Options) (*Worker, error) {
	if q == nil {
		return nil, errors.New("jobs: queue is required")
	}
	if len(handlers) == 0 {
		return nil, errors.New("jobs: at least one handler required")
	}
	pe := opts.PollEvery
	if pe <= 0 {
		pe = 2 * time.Second
	}
	bd := opts.BaseDelay
	if bd <= 0 {
		bd = 10 * time.Second
	}
	md := opts.MaxDelay
	if md <= 0 {
		md = 10 * time.Minute
	}
	hm := make(map[catalog.JobType]Handler, len(handlers))
	for _, h := range handlers {
		if _, dup := hm[h.Type()]; dup {
			return nil, fmt.Errorf("jobs: duplicate handler for type %s", h.Type())
		}
		hm[h.Type()] = h
	}
	wid := opts.WorkerID
	if wid == "" {
		wid = "worker-" + uuid.NewString()[:8]
	}
	return &Worker{
		queue:     q,
		handlers:  hm,
		logger:    opts.Logger,
		metrics:   opts.Metrics,
		workerID:  wid,
		pollEvery: pe,
		baseDelay: bd,
		maxDelay:  md,
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}, nil
}

// Start 启动 worker 轮询循环。阻塞直到 Stop 被调用。可在 goroutine 中运行。
func (w *Worker) Start(ctx context.Context) {
	defer close(w.done)
	ticker := time.NewTicker(w.pollEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stop:
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

// Stop 停止 worker。等待当前任务完成后返回。
func (w *Worker) Stop() {
	select {
	case <-w.stop:
		// already closed
	default:
		close(w.stop)
	}
	<-w.done
}

// runOnce 尝试领取并执行一个任务。
func (w *Worker) runOnce(ctx context.Context) {
	job, err := w.queue.Claim(ctx, w.workerID, time.Now().UTC())
	if err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			return // 无任务
		}
		if w.logger != nil {
			w.logger.Warn("jobs: claim failed", zap.String("err", err.Error()))
		}
		return
	}
	handler, ok := w.handlers[job.Type]
	if !ok {
		// 无对应 handler：标记失败重试，避免永久卡住。
		w.fail(ctx, job, fmt.Errorf("jobs: no handler for type %s", job.Type))
		return
	}

	start := time.Now()
	err = handler.Execute(ctx, job)
	elapsed := time.Since(start)
	if w.metrics != nil {
		secs := elapsed.Seconds()
		outcome := "success"
		if err != nil {
			outcome = "error"
		}
		w.metrics.ObserveJobDuration(string(job.Type), secs)
		w.metrics.IncJobTotal(string(job.Type), outcome)
	}
	if err != nil {
		w.fail(ctx, job, err)
		return
	}
	if cerr := w.queue.Complete(ctx, job.ID); cerr != nil {
		if w.logger != nil {
			w.logger.Error("jobs: complete failed",
				zap.String("job_id", job.ID),
				zap.String("err", cerr.Error()))
		}
	}
}

// fail 记录失败并安排重试（指数退避）。
func (w *Worker) fail(ctx context.Context, job catalog.Job, err error) {
	delay := w.backoff(job.Attempt)
	next := time.Now().UTC().Add(delay)
	if rerr := w.queue.Retry(ctx, job.ID, err.Error(), next); rerr != nil {
		if w.logger != nil {
			w.logger.Error("jobs: retry failed",
				zap.String("job_id", job.ID),
				zap.String("err", rerr.Error()))
		}
	}
	if w.logger != nil {
		w.logger.Warn("jobs: execution failed",
			zap.String("job_id", job.ID),
			zap.String("type", string(job.Type)),
			zap.Int("attempt", job.Attempt),
			zap.Duration("retry_after", delay),
			zap.String("err", err.Error()))
	}
}

// backoff 计算指数退避延迟：base * 2^(attempt-1)，上限 maxDelay。
func (w *Worker) backoff(attempt int) time.Duration {
	if attempt <= 1 {
		return w.baseDelay
	}
	d := w.baseDelay
	for i := 1; i < attempt && d < w.maxDelay; i++ {
		d *= 2
	}
	if d > w.maxDelay {
		d = w.maxDelay
	}
	return d
}
