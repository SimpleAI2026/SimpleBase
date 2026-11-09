// 文件说明：backup.go — internal/config 中 BackupConfig 等符号的实现
//
// 用法
//
//	type BackupConfig（导出类型，同包与上层均可使用）
//	同包调用：BackupConfig → config.go；validateBackup() → config.go
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api, internal/api/perfbench 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/config/backup.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] config.go, env.go, http_boundary.go, yaml.go
//	[关键引用] BackupConfig ← config.go
//
// 主要符号
//
//	type BackupConfig struct {…}
//	... 另有 1 个顶层符号

package config

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// MissedPolicy 取值（backup.missed_policy）：停机错过 cron 窗口后的处理方式。
const (
	// MissedPolicySkip 错过即跳过，等下一个 cron 窗口。
	MissedPolicySkip = "miss"
	// MissedPolicyCatchup 错过后启动时立即补跑一次（默认）。
	MissedPolicyCatchup = "catchup"
)

type BackupConfig struct {
	Enabled         bool          `yaml:"enabled"`
	Dir             string        `yaml:"dir"`
	UploadToS3      bool          `yaml:"upload_to_s3"`
	Schedule        string        `yaml:"schedule"`
	KeepLast        int           `yaml:"keep_last"`
	KeepDays        int           `yaml:"keep_days"`
	MaxPerProject   int           `yaml:"max_per_project"`
	SnapshotTimeout time.Duration `yaml:"snapshot_timeout"`
	// MissedPolicy miss | catchup；空 = catchup（production-readiness-plan §8.3.2）。
	MissedPolicy string `yaml:"missed_policy"`
}

// EffectiveMissedPolicy 归一化补跑策略；空视为 catchup。
func (c BackupConfig) EffectiveMissedPolicy() string {
	switch strings.ToLower(strings.TrimSpace(c.MissedPolicy)) {
	case MissedPolicySkip:
		return MissedPolicySkip
	default:
		return MissedPolicyCatchup
	}
}

func validateBackup(c BackupConfig) error {
	if c == (BackupConfig{}) {
		return nil
	}
	if c.Dir == "" {
		return errors.New("backup.dir is required")
	}
	if c.KeepLast < 1 || c.MaxPerProject < 1 || c.KeepLast > c.MaxPerProject {
		return errors.New("backup.keep_last must be between 1 and backup.max_per_project")
	}
	if c.KeepDays < 0 || c.SnapshotTimeout <= 0 {
		return errors.New("backup retention and snapshot_timeout are invalid")
	}
	switch strings.ToLower(strings.TrimSpace(c.MissedPolicy)) {
	case "", MissedPolicySkip, MissedPolicyCatchup:
	default:
		return fmt.Errorf("backup.missed_policy must be miss or catchup (got %q)", c.MissedPolicy)
	}
	return nil
}
