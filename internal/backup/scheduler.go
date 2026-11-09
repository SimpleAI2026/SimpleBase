// 文件说明：scheduler.go — 备份 cron 调度（§8.3.2 改造：停机错过窗口 catchup、单飞、错误上报）
//
// 用法
//
//	NewScheduler(service, repo, expression, opts) (*Scheduler, error)
//	(s *Scheduler) Start(ctx)   // 首轮按 missed_policy 补跑，其后按 cron 窗口
//	(s *Scheduler) RunOnce(ctx) // 手动触发（与 cron 共用单飞）
package backup

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/utils/crontab"
)

// Reporter 是调度器向上报告备份结果的通道（§8.3.2：错误必须可见）。
// app 层接线：告警信号（alerts.Bus）+ 审计（sys_audit_events）+ 日志。
type Reporter interface {
	// BackupSucceeded 全部项目成功；lastSuccessAt 供 readiness 检查「上次成功 < 48h」。
	BackupSucceeded(at time.Time, projects int)
	// BackupFailed 部分或全部失败；err 为聚合错误。
	BackupFailed(at time.Time, err error)
}

// SchedulerOptions 是调度器行为开关（§8.3.2）。
type SchedulerOptions struct {
	// MissedPolicy miss | catchup；空 = catchup。
	MissedPolicy string
	// Reporter 可为 nil（仅日志语义退化为无上报）。
	Reporter Reporter
	// Now 可注入时钟（测试）。
	Now func() time.Time
}

type Scheduler struct {
	service *Service
	// repo 只用到 ListProjects（§8.3.2 改为接口以便测试注入）。
	repo projectLister
	spec crontab.CronSpec
	opts SchedulerOptions

	// 单飞：cron 触发与手动 trigger 共用；进行中时后续触发直接跳过并计数。
	mu      sync.Mutex
	running bool
	skipped int64
}

// projectLister 是调度器所需的最小仓库接口。
type projectLister interface {
	ListProjects(ctx context.Context) ([]ProjectTarget, error)
}

// lastBackupReader 提供最近一次备份时刻（catchup 判定用）；可选能力。
type lastBackupReader interface {
	// LastBackupAt 返回全部项目最近一次备份的最大时刻；无任何备份返回零值。
	LastBackupAt(ctx context.Context) (time.Time, error)
}

func NewScheduler(service *Service, repo projectLister, expression string, opts SchedulerOptions) (*Scheduler, error) {
	spec, err := crontab.ParseCron(expression)
	if err != nil {
		return nil, err
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Scheduler{service: service, repo: repo, spec: spec, opts: opts}, nil
}

// Start 启动调度循环。missed_policy=catchup（默认）时：
// 以「最近一次备份时刻」为锚——若它早于上一个 cron 窗口（即停机错过了窗口），
// 启动即补跑一次；无任何备份记录时视为全新部署，等首个窗口。
func (s *Scheduler) Start(ctx context.Context) {
	// §8.3.2 catchup：启动时判定是否错过窗口。
	if s.opts.MissedPolicy != config.MissedPolicySkip {
		if s.missedWindow(ctx) {
			go func() {
				_ = s.runOnceGuarded(ctx)
			}()
		}
	}
	go func() {
		for {
			next, err := s.spec.NextAfter(s.opts.Now())
			if err != nil {
				return
			}
			timer := time.NewTimer(time.Until(next))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				_ = s.runOnceGuarded(ctx)
			}
		}
	}()
}

// missedWindow 判定启动时是否错过了最近的 cron 窗口（§8.3.2）：
// 最近一次备份时刻 早于 上一个 cron 窗口时刻 → 该窗口已错过 → 补跑。
// 仓库不支持 LastBackupAt（可选能力）或查询失败时保守返回 false。
func (s *Scheduler) missedWindow(ctx context.Context) bool {
	reader, ok := s.repo.(lastBackupReader)
	if !ok {
		return false
	}
	last, err := reader.LastBackupAt(ctx)
	if err != nil || last.IsZero() {
		return false
	}
	prev, err := s.spec.PrevBefore(s.opts.Now())
	if err != nil {
		return false
	}
	return last.Before(prev)
}

// runOnceGuarded 单飞 + 结果上报（§8.3.2）。
func (s *Scheduler) runOnceGuarded(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.skipped++
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	started := s.opts.Now()
	projects, err := s.RunOnceDetailed(ctx)
	if s.opts.Reporter == nil {
		return err
	}
	if err != nil {
		s.opts.Reporter.BackupFailed(started, err)
		return err
	}
	s.opts.Reporter.BackupSucceeded(started, projects)
	return nil
}

// Skipped 返回单飞跳过计数（观测用）。
func (s *Scheduler) Skipped() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.skipped
}

func (s *Scheduler) RunOnce(ctx context.Context) error {
	_, err := s.RunOnceDetailed(ctx)
	return err
}

// RunOnceDetailed 执行一轮全项目备份，返回成功的项目数。
func (s *Scheduler) RunOnceDetailed(ctx context.Context) (int, error) {
	projects, err := s.repo.ListProjects(ctx)
	if err != nil {
		return 0, err
	}
	var errs []error
	succeeded := 0
	for _, project := range projects {
		if _, err := s.service.Create(ctx, project.TenantID, project.ProjectID); err != nil {
			errs = append(errs, err)
			continue
		}
		succeeded++
	}
	return succeeded, errors.Join(errs...)
}
