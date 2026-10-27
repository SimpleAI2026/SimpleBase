// 文件说明：instance_format.go — godatabase/ducklake 中 InstanceFormatVersion、InstanceFormat 等符号的实现
//
// 用法
//
//	LocalInstanceFormatPath(dataRoot string) string  // LocalInstanceFormatPath 返回本地标记路径（持久根下，随数据一起搬迁）
//	EnsureInstanceFormat(ctx context.Context, in FormatCheckInput) (InstanceFormat ...  // EnsureInstanceFormat 启动兼容性校验（§4.2）：
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/app, internal/systemdb
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/ducklake/instance_format.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase/blob, godatabase/internal/atomicfile
//	[同包] catalog_syncer.go, errors.go, extension.go, factory.go, gated_conn.go 等 14 个
//
// 主要符号
//
//	const InstanceFormatVersion
//	type InstanceFormat struct {…}
//	type FormatCheckInput struct {…}
//	LocalInstanceFormatPath(dataRoot string) string
//	EnsureInstanceFormat(ctx context.Context, in FormatCheckInput) (InstanceFormat ...
//	DetectLocalData(cacheDir string) (bool, string)
//	... 另有 11 个顶层符号

package ducklake

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase/blob"
	"github.com/linkxzhou/SimpleBase/godatabase/internal/atomicfile"
)

// InstanceFormatVersion 是实例级格式标记的格式版本（ducklake-duckdb-catalog-plan §4.1）。
// 以后提升 DuckDB storage version 等不兼容变更时递增，按「不兼容 → 报错 → reset」处理。
//
// v2：用户库改为「每项目一个 DuckDB/SQLite 单文件」（planv6.2）。v1 的
// {project}/databases/{id}/catalog 布局不再被读取，旧标记直接拒绝启动。
const InstanceFormatVersion = 2

// instanceFormatFileName 是本地标记文件名：{cache_dir}/instance-format.json。
const instanceFormatFileName = "instance-format.json"

// InstanceFormat 是实例级 catalog 格式标记。
type InstanceFormat struct {
	CatalogEngine string `json:"catalog_engine"`
	FormatVersion int    `json:"format_version"`
	CreatedAt     string `json:"created_at"`
	CreatedBy     string `json:"created_by"`
}

// FormatCheckInput 是 EnsureInstanceFormat 的入参。
type FormatCheckInput struct {
	// Engine 是配置的 catalog 引擎。
	Engine string
	// DataRoot 是本地持久根（storage.data_dir）：本地标记与本地数据探测的根。
	DataRoot string
	// Store/Keys 仅在远端启用时设置；Store 为 nil 表示数据库走本地盘。
	Store blob.Store
	Keys  blob.KeyBuilder
	// CreatedBy 写入标记的 created_by（{instance_id}/{boot_id}）。
	CreatedBy string
	// Bucket 仅用于报错信息定位远端标记。
	Bucket string
}

// LocalInstanceFormatPath 返回本地标记路径（持久根下，随数据一起搬迁）。
func LocalInstanceFormatPath(dataRoot string) string {
	return filepath.Join(dataRoot, instanceFormatFileName)
}

// EnsureInstanceFormat 启动兼容性校验（§4.2）：
//
//	读远端标记（Store != nil 时）/ 本地标记
//	├─ 两者都不存在：
//	│    ├─ 探测到已有数据 → ErrCatalogEngineUnknown
//	│    └─ 完全干净 → 写标记（远端 PutIfAbsent；冲突 → 重读后比较）→ 通过
//	├─ 标记.catalog_engine == 配置 → 通过（本地缺标记时补写）
//	└─ 不一致 / 远端与本地互相矛盾 → ErrCatalogEngineMismatch
//
// 任何远端读/List 错误都 fail closed，绝不在无法确认时写标记。
func EnsureInstanceFormat(ctx context.Context, in FormatCheckInput) (InstanceFormat, error) {
	engine := NormalizeCatalogEngine(in.Engine)
	if !IsValidCatalogEngine(engine) {
		return InstanceFormat{}, fmt.Errorf("ducklake: unsupported catalog engine %q", in.Engine)
	}
	if in.DataRoot == "" {
		return InstanceFormat{}, errors.New("ducklake: cache dir is required for instance format check")
	}

	local, hasLocal, err := readLocalFormat(in.DataRoot)
	if err != nil {
		return InstanceFormat{}, err
	}
	var remote InstanceFormat
	hasRemote := false
	if in.Store != nil {
		remote, hasRemote, err = readRemoteFormat(ctx, in.Store, in.Keys)
		if err != nil {
			return InstanceFormat{}, err
		}
	}

	// 远端与本地互相矛盾 → 拒绝（不猜哪一方是对的）。
	if hasRemote && hasLocal && NormalizeCatalogEngine(remote.CatalogEngine) != NormalizeCatalogEngine(local.CatalogEngine) {
		return InstanceFormat{}, mismatchErr(in, engine, fmt.Sprintf("remote marker says %q but local marker (%s) says %q",
			remote.CatalogEngine, LocalInstanceFormatPath(in.DataRoot), local.CatalogEngine))
	}

	switch {
	case hasRemote:
		if err := compareFormat(in, engine, remote, "remote"); err != nil {
			return InstanceFormat{}, err
		}
		if !hasLocal {
			if err := writeLocalFormat(in.DataRoot, remote); err != nil {
				return InstanceFormat{}, err
			}
		}
		return remote, nil
	case hasLocal:
		if err := compareFormat(in, engine, local, "local"); err != nil {
			return InstanceFormat{}, err
		}
		if in.Store != nil {
			// 远端缺标记但本地有：远端被清空（或首次开启远端）。远端必须干净才补写，
			// 否则无法确认远端已有数据的引擎。
			if err := requireRemoteClean(ctx, in); err != nil {
				return InstanceFormat{}, err
			}
			got, err := putRemoteFormat(ctx, in, local)
			if err != nil {
				return InstanceFormat{}, err
			}
			if err := compareFormat(in, engine, got, "remote"); err != nil {
				return InstanceFormat{}, err
			}
		}
		return local, nil
	}

	// 两者都不存在：只有完全干净才写新标记。
	if found, where := DetectLocalData(in.DataRoot); found {
		return InstanceFormat{}, unknownErr(in, "local cache contains existing data at "+where)
	}
	if in.Store != nil {
		if err := requireRemoteClean(ctx, in); err != nil {
			return InstanceFormat{}, err
		}
	}
	fresh := InstanceFormat{
		CatalogEngine: engine,
		FormatVersion: InstanceFormatVersion,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		CreatedBy:     in.CreatedBy,
	}
	if in.Store != nil {
		got, err := putRemoteFormat(ctx, in, fresh)
		if err != nil {
			return InstanceFormat{}, err
		}
		// 并发首启：另一个实例抢先写入 → 以远端为准再比较（§7-6）。
		if err := compareFormat(in, engine, got, "remote"); err != nil {
			return InstanceFormat{}, err
		}
		fresh = got
	}
	if err := writeLocalFormat(in.DataRoot, fresh); err != nil {
		return InstanceFormat{}, err
	}
	return fresh, nil
}

// compareFormat 比较标记与配置。
func compareFormat(in FormatCheckInput, engine string, f InstanceFormat, source string) error {
	if f.CatalogEngine == "" {
		return unknownErr(in, source+" marker has no catalog_engine")
	}
	if NormalizeCatalogEngine(f.CatalogEngine) != engine {
		return mismatchErr(in, engine, fmt.Sprintf("existing data uses %q (%s marker)", f.CatalogEngine, source))
	}
	if f.FormatVersion != InstanceFormatVersion {
		return mismatchErr(in, engine, fmt.Sprintf("%s marker format_version=%d, this build requires %d", source, f.FormatVersion, InstanceFormatVersion))
	}
	return nil
}

// DetectLocalData 探测本地缓存是否已有库数据（§4.3）。返回首个命中路径。
// 同时识别 v2 项目单文件（{project}/database.{duckdb,sqlite}）与 v1 旧布局。
func DetectLocalData(cacheDir string) (bool, string) {
	if _, err := os.Stat(filepath.Join(cacheDir, "system", "locator.json")); err == nil {
		return true, filepath.Join(cacheDir, "system", "locator.json")
	}
	if entries, err := os.ReadDir(cacheDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			for _, name := range []string{"database.duckdb", "database.sqlite"} {
				p := filepath.Join(cacheDir, e.Name(), name)
				if _, err := os.Stat(p); err == nil {
					return true, p
				}
			}
			p := filepath.Join(cacheDir, e.Name(), "databases")
			if info, err := os.Stat(p); err == nil && info.IsDir() {
				return true, p
			}
		}
	}
	roots := []string{
		cacheDir,
		filepath.Join(cacheDir, "system", "dbs"),
		filepath.Join(cacheDir, "dev", "dbs"),
	}
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			for _, name := range []string{"catalog.sqlite", "catalog.ducklake"} {
				p := filepath.Join(root, e.Name(), "catalog", name)
				if _, err := os.Stat(p); err == nil {
					return true, p
				}
			}
		}
	}
	return false, ""
}

// requireRemoteClean 要求存储根下没有任何项目数据（§4.3）。List 任何错误都 fail closed：
// 无法区分"前缀为空"与"寻址/路由错误"（如 bucket 级 COS endpoint + path-style 寻址下
// ListObjectsV2 被 COS 路由为 GetObject，恒 404 NoSuchKey），绝不能当成空前缀。
// _system/ 下的平台标记（实例格式标记、reset 锁、CAS 探针）不算已有数据。
func requireRemoteClean(ctx context.Context, in FormatCheckInput) error {
	prefix := in.Keys.RootListingPrefix()
	keys, _, err := in.Store.List(ctx, prefix, "", 100)
	if err != nil {
		return fmt.Errorf("ducklake: cannot verify remote prefix %s is empty (list failed, refusing to write instance-format marker): %w", prefix, err)
	}
	sysPrefix := blob.SystemPrefix + "/"
	for _, k := range keys {
		rest := strings.TrimPrefix(strings.TrimPrefix(k, prefix), "/")
		if rest == "" || strings.HasPrefix(rest, sysPrefix) {
			continue
		}
		return unknownErr(in, "remote prefix "+prefix+" contains existing objects (e.g. "+k+")")
	}
	return nil
}

func readLocalFormat(cacheDir string) (InstanceFormat, bool, error) {
	data, err := os.ReadFile(LocalInstanceFormatPath(cacheDir))
	if err != nil {
		if os.IsNotExist(err) {
			return InstanceFormat{}, false, nil
		}
		return InstanceFormat{}, false, fmt.Errorf("ducklake: read local instance format: %w", err)
	}
	var f InstanceFormat
	if err := json.Unmarshal(data, &f); err != nil {
		// 损坏的本地标记不能静默覆盖：可能掩盖引擎变更。
		return InstanceFormat{}, false, fmt.Errorf("ducklake: local instance format %s is corrupted: %w", LocalInstanceFormatPath(cacheDir), err)
	}
	return f, true, nil
}

func writeLocalFormat(cacheDir string, f InstanceFormat) error {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("ducklake: instance format dir: %w", err)
	}
	body, err := json.Marshal(f)
	if err != nil {
		return err
	}
	path := LocalInstanceFormatPath(cacheDir)
	if err := atomicfile.Write(path, body, 0o644); err != nil {
		return fmt.Errorf("ducklake: write local instance format: %w", err)
	}
	return nil
}

func readRemoteFormat(ctx context.Context, store blob.Store, kb blob.KeyBuilder) (InstanceFormat, bool, error) {
	data, _, err := store.GetBytes(ctx, kb.InstanceFormatKey())
	if err != nil {
		if errors.Is(err, blob.ErrNotFound) {
			return InstanceFormat{}, false, nil
		}
		return InstanceFormat{}, false, fmt.Errorf("ducklake: read remote instance format: %w", err)
	}
	var f InstanceFormat
	if err := json.Unmarshal(data, &f); err != nil {
		return InstanceFormat{}, false, fmt.Errorf("ducklake: remote instance format is corrupted: %w", err)
	}
	return f, true, nil
}

// putRemoteFormat PutIfAbsent 写远端标记；冲突时重读并返回远端现值。
func putRemoteFormat(ctx context.Context, in FormatCheckInput, f InstanceFormat) (InstanceFormat, error) {
	body, err := json.Marshal(f)
	if err != nil {
		return InstanceFormat{}, err
	}
	if _, err := in.Store.PutIfAbsent(ctx, in.Keys.InstanceFormatKey(), body, "application/json"); err != nil {
		if !errors.Is(err, blob.ErrPreconditionFailed) {
			return InstanceFormat{}, fmt.Errorf("ducklake: write remote instance format: %w", err)
		}
		got, ok, rerr := readRemoteFormat(ctx, in.Store, in.Keys)
		if rerr != nil {
			return InstanceFormat{}, rerr
		}
		if !ok {
			return InstanceFormat{}, errors.New("ducklake: remote instance format conflict but marker not readable")
		}
		return got, nil
	}
	return f, nil
}

func markerLocation(in FormatCheckInput) string {
	if in.Store == nil {
		return LocalInstanceFormatPath(in.DataRoot)
	}
	if in.Bucket != "" {
		return "s3://" + in.Bucket + "/" + in.Keys.InstanceFormatKey()
	}
	return in.Keys.InstanceFormatKey()
}

func resetHint(in FormatCheckInput) string {
	return "Switching engines is not supported and requires wiping ALL databases:\n" +
		"  ./build.sh reset\n" +
		"or set database.ducklake.catalog_engine back to the engine of the existing data."
}

func mismatchErr(in FormatCheckInput, engine, detail string) error {
	return fmt.Errorf("%w: config database.ducklake.catalog_engine=%s, %s (marker %s).\n%s",
		ErrCatalogEngineMismatch, engine, detail, markerLocation(in), resetHint(in))
}

func unknownErr(in FormatCheckInput, detail string) error {
	return fmt.Errorf("%w: %s (marker %s missing).\n%s",
		ErrCatalogEngineUnknown, detail, markerLocation(in), resetHint(in))
}
