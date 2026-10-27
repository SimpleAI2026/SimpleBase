// 文件说明：model.go — internal/backup 中 ErrDisabled、ErrNotFound 等符号的实现
//
// 用法
//
//	type Item（导出类型，同包与上层均可使用）
//	type Manifest（导出类型，同包与上层均可使用）
//	同包调用：Item → repository.go, restore.go 等；ErrNotFound → repository.go, service.go；ErrUnavailable → repository.go, service.go
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/backup/model.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] repository.go, restore.go, scheduler.go, service.go
//	[关键引用] Item ← repository.go, restore.go, service.go
//
// 主要符号
//
//	var ErrDisabled
//	var ErrNotFound
//	var ErrProjectBusy
//	var ErrSnapshotMismatch
//	var ErrUnavailable
//	type Item struct {…}
//	... 另有 1 个顶层符号

package backup

import (
	"errors"
	"time"
)

var (
	ErrDisabled         = errors.New("backup_disabled")
	ErrNotFound         = errors.New("backup_not_found")
	ErrProjectBusy      = errors.New("project_busy")
	ErrSnapshotMismatch = errors.New("snapshot_mismatch")
	ErrUnavailable      = errors.New("backup_unavailable")
)

type Item struct {
	ID            string     `json:"id"`
	ProjectID     string     `json:"project_id"`
	StorageKey    string     `json:"-"`
	SizeBytes     int64      `json:"size_bytes"`
	SHA256        string     `json:"sha256"`
	Engine        string     `json:"engine"`
	FormatVersion int        `json:"format_version"`
	Status        string     `json:"status"`
	Error         string     `json:"error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
}

type Manifest struct {
	Item
	InstanceID string `json:"instance_id,omitempty"`
}
