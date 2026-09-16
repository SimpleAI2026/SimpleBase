package systemdb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
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
	tmp := locatorPath(dir) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("systemdb: write locator tmp: %w", err)
	}
	if err := os.Rename(tmp, locatorPath(dir)); err != nil {
		return fmt.Errorf("systemdb: rename locator: %w", err)
	}
	return nil
}
