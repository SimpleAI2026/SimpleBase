// 文件说明：dev_lock.go — .simplebase/lock.json 的读写与数据结构
//
// 用法
//
//	同包内由 dev.go 的 runDev 调用；与 skills/simplebase/scripts/sbdev.py 的对应步骤语义一致。
//
// 调用架构
//
//	[上游] dev.go（devCommand）
//	         ▼
//	[本文件] internal/sbcli/dev_lock.go
//	         ▼
//	[同包] dev.go（devError / DevAction / devTarget）、dev_workspace.go（devWorkspace）
//
// 主要符号
//
//	type devLock / loadDevLock / saveDevLock / devLockPath

package sbcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/linkxzhou/SimpleBase/godatabase/atomicfile"
)

type devLockDB struct {
	ID string `json:"id"`
}

type devLockFunc struct {
	SHA     string `json:"sha"`
	Version int64  `json:"version"`
}

type devLockCron struct {
	ID string `json:"id"`
}

// devLockEntry 字段顺序与 sbdev.py 的 sort_keys 输出一致，保证两端写出的 lock 可互相读取。
type devLockEntry struct {
	AppliedAt  string                       `json:"applied_at,omitempty"`
	Engine     string                       `json:"engine,omitempty"`
	Cron       map[string]devLockCron       `json:"cron,omitempty"`
	Databases  map[string]devLockDB         `json:"databases,omitempty"`
	Functions  map[string]devLockFunc       `json:"functions,omitempty"`
	Migrations map[string]map[string]string `json:"migrations,omitempty"`
	Sbdev      string                       `json:"sbdev,omitempty"`
}

type devLock struct {
	Targets map[string]*devLockEntry `json:"targets"`
	Version int                      `json:"version"`
}

func devLockPath(w *devWorkspace) string { return filepath.Join(w.root, devStateDir, devLockName) }

func loadDevLock(w *devWorkspace) (*devLock, error) {
	lock := &devLock{Targets: map[string]*devLockEntry{}, Version: 1}
	b, err := os.ReadFile(devLockPath(w))
	if errors.Is(err, os.ErrNotExist) {
		return lock, nil
	}
	if err != nil {
		return nil, &devError{code: "workspace_invalid", msg: err.Error(), exit: exitUsage}
	}
	if err := json.Unmarshal(b, lock); err != nil || lock.Targets == nil {
		return nil, &devError{code: "workspace_invalid", exit: exitUsage,
			msg: devLockPath(w) + " 损坏：修复或删除后重跑（删除会丢失迁移 checksum）"}
	}
	return lock, nil
}

func saveDevLock(w *devWorkspace, lock *devLock) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(lock); err != nil {
		return err
	}
	return atomicfile.WriteEnsureDir(devLockPath(w), buf.Bytes(), 0o644)
}
