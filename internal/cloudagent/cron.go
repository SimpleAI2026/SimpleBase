package cloudagent

import "github.com/linkxzhou/SimpleBase/internal/crontab"

// CronSpec 兼容别名：实现已迁移至 internal/crontab（ui-cronjob-plan §5.1）。
type CronSpec = crontab.CronSpec

// ParseCron 兼容薄封装。
func ParseCron(expr string) (CronSpec, error) { return crontab.ParseCron(expr) }
