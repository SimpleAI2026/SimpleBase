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

// authTablesCompactInterval 是认证热点表的小文件合并周期。
// planv5.0 §4 P2：refresh 轮转 UPDATE / 项目归属变更高频小事务使
// sys_user_sessions、sys_project_owners、sys_users 存活文件持续增长
//（2026-10-08 基线 18/19/9 个），而这三张表正是 5s Principal 缓存过期
// 波次必读的表；远端 COS 每文件一次往返，读放大直接落在认证延迟上。
// 比全库 merge（每小时）更频繁，且按表定向，避开日志/指标等大表的
// 合并内存开销。
const authTablesCompactInterval = 15 * time.Minute

// authCompactTables 待定向合并的认证热点表（固定集合，低基数）。
var authCompactTables = []string{"sys_user_sessions", "sys_project_owners", "sys_users"}

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

	// liveFiles 抽象按表统计存活文件数（生产查 ducklake 元数据；
	// 测试可注入）。返回 <0 表示统计失败，调用方按「跳过」处理。
	liveFiles func(ctx context.Context, table string) int

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
		r.liveFiles = r.liveFilesViaCatalog
	}
	return r
}

// liveFilesViaCatalog 查 ducklake 元数据统计表的存活数据文件数。
// 查询失败返回 -1（调用方按跳过处理，不阻塞其他表）。
func (r *MaintenanceRunner) liveFilesViaCatalog(ctx context.Context, table string) int {
	if r.store == nil || r.store.db == nil {
		return -1
	}
	var live int
	err := r.store.db.QueryRowContext(ctx,
		`SELECT count(*) FROM __ducklake_metadata_lake.ducklake_data_file
		 WHERE end_snapshot IS NULL AND table_id =
		   (SELECT table_id FROM __ducklake_metadata_lake.ducklake_table WHERE table_name = ?)`,
		table).Scan(&live)
	if err != nil {
		return -1
	}
	return live
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
	authTimer := time.NewTimer(MaintenanceInitialDelay + authTablesCompactInterval)
	defer timer.Stop()
	defer authTimer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			r.runOnce(ctx)
			timer.Reset(r.interval())
		case <-authTimer.C:
			r.compactAuthTables(ctx)
			authTimer.Reset(authTablesCompactInterval)
		}
	}
}

// compactAuthTables 对认证热点表执行定向小文件合并（planv5.0 §4 P2.2）。
// 仅 writer 执行；先按表统计存活文件数，超过阈值才发起 merge，
// 空表/干净表直接跳过，不产生无效写事务与快照。合并是数据写入，
// 与既有全库 merge 走同一执行通道（catalog 同步与冲突检测照常生效）。
func (r *MaintenanceRunner) compactAuthTables(ctx context.Context) {
	if r.writable != nil && !r.writable() {
		return
	}
	if r.exec == nil || r.liveFiles == nil {
		return
	}
	const minLiveFiles = 4 // 低于该文件数收益小于合并成本，跳过
	for _, table := range authCompactTables {
		if ctx.Err() != nil {
			return
		}
		live := r.liveFiles(ctx, table)
		if live < minLiveFiles {
			continue
		}
		start := time.Now()
		_, err := r.exec(ctx, "CALL ducklake_merge_adjacent_files(?, ?)", r.lake, table)
		if r.logger != nil {
			fields := []zap.Field{
				zap.String("step", "merge_auth_table"),
				zap.String("table", table),
				zap.Int("live_files", live),
				zap.Duration("duration", time.Since(start)),
			}
			if err != nil {
				r.logger.Warn("systemdb maintenance step failed", append(fields, zap.Error(err))...)
			} else {
				r.logger.Info("systemdb maintenance step done", fields...)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(maintenanceStepYield):
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
