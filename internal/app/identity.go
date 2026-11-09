// 文件说明：identity.go — internal/app 的内部实现（storageIdentity、storageIdentityPath 等）
//
// 用法
//
//	type storageIdentity（包内使用）
//	同包调用：checkStorageIdentity() → app.go
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api/perfbench
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/app/identity.go
//	         │ 依赖
//	         ▼
//	[下游] internal/config, internal/utils/atomicfile
//	[同包] app.go, catalog_engine.go, cron_runner.go, project_purger.go, reset.go 等 6 个
//	[关键引用] checkStorageIdentity() ← app.go
//
// 主要符号
//
//	type storageIdentity struct {…}
//	storageIdentityPath(dataRoot string) string
//	const storageSystemDir
//	checkStorageIdentity(cfg config.Config) error
//	normalizeMode(mode string) string

package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/utils/atomicfile"
)

// storageIdentity 记录实例首次启动时的持久层身份。
//
// 持久层换了（storage.database/files 模式、bucket、prefix 任一变化），
// 现有数据就"消失"了——它不是被删了，而是被指到了另一个位置。
// 启动时检测到不一致即拒绝，提示先执行 ./build.sh reset。
type storageIdentity struct {
	Database    string `json:"storage_database"`
	Files       string `json:"storage_files"`
	Bucket      string `json:"bucket"`
	Prefix      string `json:"prefix"`
	FirstSeenAt string `json:"first_seen_at"`
}

// storageIdentityPath 返回身份文件路径：{data_dir}/_system/identity.json。
// 放在持久根下，随数据一起搬迁；缓存目录可被整体删除，不能放那里。
func storageIdentityPath(dataRoot string) string {
	return filepath.Join(dataRoot, storageSystemDir, "identity.json")
}

// storageSystemDir 是持久根下平台级对象的目录名。
const storageSystemDir = "_system"

// checkStorageIdentity 启动检测：与首次启动不一致时返回错误，否则写入。
// 唯一记录的例外是"首次启动"——文件不存在时直接写入当前身份。
func checkStorageIdentity(cfg config.Config) error {
	path := storageIdentityPath(cfg.Storage.DataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("app: storage identity dir: %w", err)
	}

	want := storageIdentity{
		Database: normalizeMode(cfg.Storage.Database),
		Files:    normalizeMode(cfg.Storage.Files),
		Bucket:   cfg.S3.Bucket,
		Prefix:   strings.Trim(cfg.S3.Prefix, "/"),
	}

	data, err := os.ReadFile(path)
	if err == nil && len(data) > 0 {
		var prev storageIdentity
		if jsonErr := json.Unmarshal(data, &prev); jsonErr == nil {
			if prev.Database != want.Database || prev.Files != want.Files ||
				prev.Bucket != want.Bucket || prev.Prefix != want.Prefix {
				return fmt.Errorf(
					"app: storage identity changed since first start "+
						"(was storage.database=%q storage.files=%q bucket=%q prefix=%q, now database=%q files=%q bucket=%q prefix=%q). "+
						"Pointing at a different storage location hides all existing data. "+
						"Switching storage modes is not migrated: run `./build.sh reset` to start empty, "+
						"or restore the previous storage settings",
					prev.Database, prev.Files, prev.Bucket, prev.Prefix,
					want.Database, want.Files, want.Bucket, want.Prefix)
			}
			return nil
		}
		// 损坏的身份文件：覆盖重写（保守继续）。
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("app: read storage identity: %w", err)
	}

	want.FirstSeenAt = time.Now().UTC().Format(time.RFC3339Nano)
	body, err := json.Marshal(want)
	if err != nil {
		return err
	}
	if err := atomicfile.Write(path, body, 0o644); err != nil {
		return fmt.Errorf("app: write storage identity: %w", err)
	}
	return nil
}

// normalizeMode 归一化存储模式：空值按默认 local 处理。
func normalizeMode(mode string) string {
	m := strings.ToLower(strings.TrimSpace(mode))
	if m == "" {
		return config.StorageModeLocal
	}
	return m
}
