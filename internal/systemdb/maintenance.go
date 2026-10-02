// maintenance.go 系统库周期维护（perf §1 P1-C）。
//
// 步骤：ducklake_merge_adjacent_files → ducklake_expire_snapshots → ducklake_cleanup_old_files。
// 约束（multi-instance-consistency-plan §3.2）：
//   - 绝不做 ducklake_delete_orphaned_files——基于单边 catalog 视图删除有丢数据风险；
//   - 仅在实例可写且持有系统库写租约（启用租约时）的 writer 上执行，否则直接跳过；
//   - cleanup_old_files 首轮 dry_run（只记录将删除的对象），短期开关
//     SIMPLEBASE_SYSTEMDB_CLEANUP_DELETE=1 才真实删除，验证一轮后移除该开关；
//   - 全部走系统库唯一连接，步骤间让出，避免长时间占住连接。
package systemdb

import (
	"context"
	"database/sql"
	"os"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/database/ducklake"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"go.uber.org/zap"
)

// MaintenanceInitialDelay 是启动后首次维护的延迟（避开启动高峰）。
const MaintenanceInitialDelay = time.Minute

// maintenanceStepYield 是相邻维护步骤间的让出时长。
const maintenanceStepYield = 200 * time.Millisecond

// MaintenanceRunner 周期执行系统库维护。
type MaintenanceRunner struct {
	store    *Store
	opts     ducklake.MaintenanceOptions
	writable func() bool // 可写且（启用租约时）持有系统库写权
	logger   observability.Logger
	lake     string
	dryRun   bool

	// exec 抽象 SQL 执行（生产为 store.db；测试可注入 fake）。
	exec func(ctx context.Context, query string, args ...any) (sql.Result, error)

	cancel context.CancelFunc
	done   chan struct{}
}

// NewMaintenanceRunner 装配系统库维护循环；writable 为 nil 表示不做写权检查。
func NewMaintenanceRunner(store *Store, opts ducklake.MaintenanceOptions, writable func() bool, logger observability.Logger) *MaintenanceRunner {
	r := &MaintenanceRunner{
		store:    store,
		opts:     opts,
		writable: writable,
		logger:   logger,
		lake:     ducklake.DefaultLakeAlias,
		// 首轮 dry-run：只记录不删除；SIMPLEBASE_SYSTEMDB_CLEANUP_DELETE=1 放开真实删除。
		dryRun: os.Getenv("SIMPLEBASE_SYSTEMDB_CLEANUP_DELETE") != "1",
	}
	if store != nil && store.db != nil {
		r.exec = store.db.ExecContext
	}
	return r
}

// Start 启动维护循环（幂等；无连接时不启动）。
func (r *MaintenanceRunner) Start() {
	if r == nil || r.exec == nil || r.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.done = make(chan struct{})
	go r.loop(ctx)
}

// Close 停止维护循环（io.Closer，供 app 反向释放）。
func (r *MaintenanceRunner) Close() error {
	if r == nil || r.cancel == nil {
		return nil
	}
	r.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	select {
	case <-r.done:
	case <-ctx.Done():
	}
	return nil
}

func (r *MaintenanceRunner) interval() time.Duration {
	if r.opts.CheckpointInterval > 0 {
		return r.opts.CheckpointInterval
	}
	return time.Hour
}

func (r *MaintenanceRunner) loop(ctx context.Context) {
	defer close(r.done)
	timer := time.NewTimer(MaintenanceInitialDelay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			r.runOnce(ctx)
			timer.Reset(r.interval())
		}
	}
}

// runOnce 执行一轮维护；任一步失败只记录不中断后续步骤。
func (r *MaintenanceRunner) runOnce(ctx context.Context) {
	if r.writable != nil && !r.writable() {
		if r.logger != nil {
			r.logger.Debug("systemdb maintenance skipped: not writer")
		}
		return
	}
	now := time.Now().UTC()
	expireBefore := now.Add(-r.opts.ExpireOlderThan)
	deleteBefore := now.Add(-r.opts.DeleteOlderThan)
	steps := []struct {
		name  string
		query string
		args  []any
	}{
		{"merge_adjacent_files", "CALL ducklake_merge_adjacent_files(?)", []any{r.lake}},
		{"expire_snapshots", "CALL ducklake_expire_snapshots(?, older_than => ?)", []any{r.lake, expireBefore}},
		{"cleanup_old_files", "CALL ducklake_cleanup_old_files(?, older_than => ?, dry_run => ?)", []any{r.lake, deleteBefore, r.dryRun}},
	}
	for _, st := range steps {
		if ctx.Err() != nil {
			return
		}
		start := time.Now()
		_, err := r.exec(ctx, st.query, st.args...)
		if r.logger != nil {
			fields := []zap.Field{
				zap.String("step", st.name),
				zap.Duration("duration", time.Since(start)),
			}
			if st.name == "cleanup_old_files" {
				fields = append(fields, zap.Bool("dry_run", r.dryRun))
			}
			if err != nil {
				r.logger.Warn("systemdb maintenance step failed", append(fields, zap.Error(err))...)
			} else {
				r.logger.Info("systemdb maintenance step done", fields...)
			}
		}
		// 步间让出系统库唯一连接，降低对在线请求的影响。
		select {
		case <-ctx.Done():
			return
		case <-time.After(maintenanceStepYield):
		}
	}
}
