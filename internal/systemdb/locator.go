// 文件说明：locator.go — internal/systemdb 中 Locator、LoadLocator 等符号的实现
//
// 用法
//
//	LoadLocator(dir string) (Locator, bool, error)  // LoadLocator 读取 locator.json
//	SaveLocator(dir string, loc Locator) error  // SaveLocator 原子写入 locator.json
//	同包调用：Locator → bootstrap.go, store.go；LoadLocator() → bootstrap.go；SaveLocator() → bootstrap.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/locator.go
//	         │ 依赖
//	         ▼
//	[下游] internal/utils/atomicfile
//	[同包] agent_schedules.go, agents.go, async_flush.go, bootstrap.go, cron_jobs.go 等 22 个
//	[关键引用] Locator ← bootstrap.go, store.go
//
// 主要符号
//
//	type Locator struct {…}
//	LoadLocator(dir string) (Locator, bool, error)
//	SaveLocator(dir string, loc Locator) error
//	... 另有 2 个顶层符号

package systemdb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/utils/atomicfile"
)

const locatorFileName = "locator.json"

// Locator 持久化系统库身份（chicken-egg：在 CatalogService 可用之前落盘）。
type Locator struct {
	DatabaseID string    `json:"database_id"`
	TenantID   string    `json:"tenant_id"`
	Name       string    `json:"name"`
	CreatedAt  time.Time `json:"created_at"`
}

func locatorPath(dir string) string {
	return filepath.Join(dir, locatorFileName)
}

// LoadLocator 读取 locator.json；文件不存在返回 (Locator{}, false, nil)。
func LoadLocator(dir string) (Locator, bool, error) {
	data, err := os.ReadFile(locatorPath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return Locator{}, false, nil
		}
		return Locator{}, false, fmt.Errorf("systemdb: read locator: %w", err)
	}
	var loc Locator
	if err := json.Unmarshal(data, &loc); err != nil {
		return Locator{}, false, fmt.Errorf("systemdb: parse locator: %w", err)
	}
	if loc.DatabaseID == "" || loc.TenantID == "" {
		return Locator{}, false, fmt.Errorf("systemdb: locator missing database_id or tenant_id")
	}
	return loc, true, nil
}

// SaveLocator 原子写入 locator.json。
func SaveLocator(dir string, loc Locator) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("systemdb: create locator dir: %w", err)
	}
	data, err := json.MarshalIndent(loc, "", "  ")
	if err != nil {
		return fmt.Errorf("systemdb: marshal locator: %w", err)
	}
	if err := atomicfile.Write(locatorPath(dir), data, 0o644); err != nil {
		return fmt.Errorf("systemdb: write locator: %w", err)
	}
	return nil
}
