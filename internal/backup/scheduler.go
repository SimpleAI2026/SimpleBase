// 文件说明：scheduler.go — internal/backup 中 Scheduler、NewScheduler 等符号的实现
//
// 用法
//
//	NewScheduler(service *Service, repo *Repository, expression string) (*Scheduler ...
//	(s *Scheduler) Start(ctx context.Context)
//	(s *Scheduler) RunOnce(ctx context.Context) error
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/backup/scheduler.go
//	         │ 依赖
//	         ▼
//	[下游] internal/utils/crontab
//	[同包] model.go, repository.go, restore.go, service.go
//
// 主要符号
//
//	type Scheduler struct {…}
//	NewScheduler(service *Service, repo *Repository, expression string) (*Scheduler ...
//	(s *Scheduler) Start(ctx context.Context)
//	(s *Scheduler) RunOnce(ctx context.Context) error

package backup

import (
	"context"
	"errors"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/utils/crontab"
)

type Scheduler struct {
	service *Service
	repo    *Repository
	spec    crontab.CronSpec
}

func NewScheduler(service *Service, repo *Repository, expression string) (*Scheduler, error) {
	spec, err := crontab.ParseCron(expression)
	if err != nil {
		return nil, err
	}
	return &Scheduler{service: service, repo: repo, spec: spec}, nil
}

func (s *Scheduler) Start(ctx context.Context) {
	go func() {
		for {
			next, err := s.spec.NextAfter(time.Now())
			if err != nil {
				return
			}
			timer := time.NewTimer(time.Until(next))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				_ = s.RunOnce(ctx)
			}
		}
	}()
}

func (s *Scheduler) RunOnce(ctx context.Context) error {
	projects, err := s.repo.ListProjects(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, project := range projects {
		if _, err := s.service.Create(ctx, project.TenantID, project.ProjectID); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
