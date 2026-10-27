// 文件说明：reset.go — internal/app 中 ResetOptions、ResetSummary 等符号的实现
//
// 用法
//
//	Reset(ctx context.Context, cfg config.Config, opts ResetOptions) (ResetSummary ...  // Reset 清空本实例的全部数据（本地 data_dir + cache_dir
//	type ResetOptions（导出类型，同包与上层均可使用）
//	type ResetSummary（导出类型，同包与上层均可使用）
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api/perfbench
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/app/reset.go
//	         │ 依赖
//	         ▼
//	[下游] internal/config, internal/objectstore
//	[同包] app.go, catalog_engine.go, cron_runner.go, identity.go, project_purger.go 等 6 个
//
// 主要符号
//
//	type ResetOptions struct {…}
//	type ResetSummary struct {…}
//	Reset(ctx context.Context, cfg config.Config, opts ResetOptions) (ResetSummary ...
//	... 另有 15 个顶层符号

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

// ResetOptions 控制 simplebase reset（ducklake-duckdb-catalog-plan §5）。
type ResetOptions struct {
	// Confirm 必须等于 cfg.Instance.ID，防止误清其他环境。
	Confirm string
	// LocalOnly 只清本地 data_dir 与 cache_dir（远端已由运维清空的场景）。
	LocalOnly bool
	// DryRun 只列出将删除的内容，不做任何修改。
	DryRun bool
	// Out 接收进度与摘要输出；nil 时丢弃。
	Out io.Writer
	// LeaseWait 是租约判活的观察窗口；<=0 时取 lease.ttl + lease.grace。测试用。
	LeaseWait time.Duration
}

// ResetSummary 是 reset 的执行摘要。
type ResetSummary struct {
	LocalPaths     []string
	RemoteObjects  int            // 已删除（dry-run 时为将删除）的远端对象数
	RemoteByGroup  map[string]int // system（_system/ 下平台对象）/ project（项目数据）/ other
	LeaseDatabases int            // 观察到的持租库数量（判活用）
}

// resetLockTTL 仅供排障展示锁创建时的预期窗口；无条件删除原语不可安全自动接管。
const resetLockTTL = 30 * time.Minute

type resetLock struct {
	Owner     string `json:"owner"`
	ExpiresAt string `json:"expires_at"`
}

// Reset 清空本实例的全部数据（本地 data_dir + cache_dir；远端为 {prefix}/ 下全部对象）。
// 不可恢复。不启动 HTTP 服务，也不打开任何数据库。
// 可重复执行：中途失败后重跑会从剩余对象继续删。
func Reset(ctx context.Context, cfg config.Config, opts ResetOptions) (ResetSummary, error) {
	var store objectstore.BlobStore
	if needRemoteReset(cfg, opts) {
		c, err := objectstore.NewClient(ctx, objectstore.Config{
			Endpoint:       cfg.S3.Endpoint,
			Region:         cfg.S3.Region,
			Bucket:         cfg.S3.Bucket,
			Prefix:         cfg.S3.Prefix,
			AccessKey:      cfg.S3.AccessKey,
			SecretKey:      cfg.S3.SecretKey,
			KMSKeyID:       cfg.S3.KMSKeyID,
			ForcePathStyle: cfg.S3.ForcePathStyle,
		}, nil, nil)
		if err != nil {
			return ResetSummary{}, fmt.Errorf("reset: create objectstore client: %w", err)
		}
		bs, ok := c.(objectstore.BlobStore)
		if !ok {
			return ResetSummary{}, errors.New("reset: objectstore client does not support blob operations")
		}
		store = bs
	}
	return resetWith(ctx, cfg, opts, store)
}

// needRemoteReset 仅当持久层真的落在对象存储、且调用方未要求只清本地时，才连远端。
func needRemoteReset(cfg config.Config, opts ResetOptions) bool {
	if opts.LocalOnly || cfg.S3.Bucket == "" {
		return false
	}
	return cfg.IsDBRemote() || cfg.IsFilesRemote()
}

// resetWith 是 Reset 的可测实现；store 为 nil 表示不处理远端。
func resetWith(ctx context.Context, cfg config.Config, opts ResetOptions, store objectstore.BlobStore) (ResetSummary, error) {
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	sum := ResetSummary{RemoteByGroup: map[string]int{}}

	if opts.Confirm == "" || opts.Confirm != cfg.Instance.ID {
		return sum, fmt.Errorf("reset: --confirm must equal instance.id (%q); got %q", cfg.Instance.ID, opts.Confirm)
	}
	// 删除远端对象不可逆，要求显式环境变量二次确认；
	// 纯本地清理（data_dir + cache_dir）只要求 --confirm。
	if store != nil && os.Getenv("SIMPLEBASE_ALLOW_RESET") != "1" {
		return sum, errors.New("reset: refusing to wipe remote objects unless SIMPLEBASE_ALLOW_RESET=1 is set")
	}
	localDirs, err := resetLocalDirs(cfg)
	if err != nil {
		return sum, err
	}

	var kb objectstore.KeyBuilder
	var base string
	if store != nil {
		kb = objectstore.KeyBuilder{RootPrefix: cfg.S3.Prefix}
		base = kb.RootListingPrefix() // "{prefix}/"；prefix 为空时是整个桶
	}

	// —— 列出本地目标 ——
	for _, dir := range localDirs {
		entries, rerr := os.ReadDir(dir)
		if rerr != nil && !os.IsNotExist(rerr) {
			return sum, fmt.Errorf("reset: read %s: %w", dir, rerr)
		}
		for _, e := range entries {
			sum.LocalPaths = append(sum.LocalPaths, filepath.Join(dir, e.Name()))
		}
	}

	if opts.DryRun {
		fmt.Fprintf(out, "[dry-run] instance.id=%s\n", cfg.Instance.ID)
		fmt.Fprintf(out, "[dry-run] local: %d paths under %s\n", len(sum.LocalPaths), strings.Join(localDirs, ", "))
		for _, p := range sum.LocalPaths {
			fmt.Fprintf(out, "  - %s\n", p)
		}
		if store != nil {
			keys, err := listAll(ctx, store, base)
			if err != nil {
				return sum, fmt.Errorf("reset: list remote prefix: %w", err)
			}
			for _, k := range keys {
				sum.RemoteByGroup[groupOf(base, k)]++
			}
			sum.RemoteObjects = len(keys)
			sum.LeaseDatabases = len(leaseEpochs(keys))
			fmt.Fprintf(out, "[dry-run] remote: s3://%s/%s  %d objects %v (databases with lease: %d)\n",
				cfg.S3.Bucket, base, sum.RemoteObjects, sum.RemoteByGroup, sum.LeaseDatabases)
		} else {
			fmt.Fprintln(out, "[dry-run] remote: skipped (local-only / storage is local / no bucket)")
		}
		return sum, nil
	}

	// —— 远端：锁 → 判活 → 删除 ——
	if store != nil {
		owner := cfg.Instance.ID + "/" + bootID
		if err := acquireResetLock(ctx, store, kb, owner); err != nil {
			return sum, err
		}
		if err := checkNoLiveLease(ctx, store, base, resetLeaseWait(cfg, opts), out); err != nil {
			_ = store.Delete(ctx, kb.ResetLockKey())
			return sum, err
		}
		n, err := deleteRemote(ctx, store, base, kb, sum.RemoteByGroup, out)
		sum.RemoteObjects = n
		if err != nil {
			_ = store.Delete(ctx, kb.ResetLockKey())
			return sum, err
		}
	}

	// —— 本地 ——
	for _, p := range sum.LocalPaths {
		if err := os.RemoveAll(p); err != nil {
			return sum, fmt.Errorf("reset: remove %s: %w", p, err)
		}
	}
	fmt.Fprintf(out, "reset: removed %d local paths under %s\n", len(sum.LocalPaths), strings.Join(localDirs, ", "))

	if store != nil {
		if err := store.Delete(ctx, kb.ResetLockKey()); err != nil {
			return sum, fmt.Errorf("reset: delete reset.lock: %w", err)
		}
		fmt.Fprintf(out, "reset: removed %d remote objects under s3://%s/%s %v\n",
			sum.RemoteObjects, cfg.S3.Bucket, base, sum.RemoteByGroup)
	}
	fmt.Fprintln(out, "reset: done. next start writes a fresh instance-format marker and rebuilds the system database.")
	return sum, nil
}

// resetLocalDirs 返回需要清空的本地目录（已做安全校验）。
// 本地模式下 data_dir 是唯一副本，必须与可丢弃的 cache_dir 一起清，
// 否则换了存储模式后会读到上一次的数据。
func resetLocalDirs(cfg config.Config) ([]string, error) {
	cacheDir, err := safeResetDir(cfg.Database.CacheDir, "database.cache_dir")
	if err != nil {
		return nil, err
	}
	dirs := []string{cacheDir}
	if strings.TrimSpace(cfg.Storage.DataDir) != "" {
		dataDir, derr := safeResetDir(cfg.Storage.DataDir, "storage.data_dir")
		if derr != nil {
			return nil, derr
		}
		if dataDir != cacheDir {
			dirs = append(dirs, dataDir)
		}
	}
	return dirs, nil
}

// safeResetDir 路径安全检查：拒绝空路径、根目录与家目录，以及包含配置/密钥的目录。
func safeResetDir(dir, field string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", fmt.Errorf("reset: %s is empty", field)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("reset: resolve %s: %w", field, err)
	}
	abs = filepath.Clean(abs)
	if abs == string(filepath.Separator) || abs == filepath.VolumeName(abs)+string(filepath.Separator) {
		return "", fmt.Errorf("reset: refusing to wipe root directory %s", abs)
	}
	if home, err := os.UserHomeDir(); err == nil && filepath.Clean(home) == abs {
		return "", fmt.Errorf("reset: refusing to wipe home directory %s", abs)
	}
	for _, name := range []string{"config.yaml", ".env"} {
		if _, err := os.Stat(filepath.Join(abs, name)); err == nil {
			return "", fmt.Errorf("reset: refusing to wipe %s (contains %s)", abs, name)
		}
	}
	return abs, nil
}

func resetLeaseWait(cfg config.Config, opts ResetOptions) time.Duration {
	if opts.LeaseWait > 0 {
		return opts.LeaseWait
	}
	ttl, grace := cfg.Instance.Lease.TTL, cfg.Instance.Lease.Grace
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	if grace <= 0 {
		grace = 10 * time.Second
	}
	return ttl + grace
}

// acquireResetLock 仅条件创建 reset.lock。BlobStore 不支持条件删除或续期，
// 因此锁过期也不得自动接管：删除旧锁后条件写会与另一 reset 产生竞态。
func acquireResetLock(ctx context.Context, store objectstore.BlobStore, kb objectstore.KeyBuilder, owner string) error {
	body, _ := json.Marshal(resetLock{Owner: owner, ExpiresAt: time.Now().Add(resetLockTTL).UTC().Format(time.RFC3339Nano)})
	if _, err := store.PutIfAbsent(ctx, kb.ResetLockKey(), body, "application/json"); err != nil {
		if errors.Is(err, objectstore.ErrPreconditionFailed) {
			return errors.New("reset: reset.lock already exists; stop all instances and verify whether another reset is running before manually clearing a stale lock")
		}
		return fmt.Errorf("reset: write reset.lock: %w", err)
	}
	return nil
}

// leaseKeyRe 匹配系统库 per-database 租约 {project}/databases/{db}/catalog/lease/{epoch}.json。
var leaseKeyRe = regexp.MustCompile(`/([^/]+)/databases/([^/]+)/catalog/lease/(\d+)\.json$`)

// projectLeaseKeyRe 匹配项目单文件租约 {project}/database/lease/{epoch}.json。
var projectLeaseKeyRe = regexp.MustCompile(`(?:^|/)([^/]+)/database/lease/(\d+)\.json$`)

// leaseEpochs 返回每把租约观察到的最大 epoch。
// key 为 "{project}/{db}"（系统库）或 "{project}"（项目文件）。
func leaseEpochs(keys []string) map[string]int64 {
	out := map[string]int64{}
	for _, k := range keys {
		var id string
		var epochStr string
		if m := leaseKeyRe.FindStringSubmatch(k); m != nil {
			id, epochStr = m[1]+"/"+m[2], m[3]
		} else if m := projectLeaseKeyRe.FindStringSubmatch(k); m != nil {
			id, epochStr = m[1], m[2]
		} else {
			continue
		}
		e, err := strconv.ParseInt(epochStr, 10, 64)
		if err != nil {
			continue
		}
		if e > out[id] {
			out[id] = e
		}
	}
	return out
}

// checkNoLiveLease 观察式判活（计划 §5.2 v2.1）：两次 List 之间任一库最大 epoch
// 增加 → 有存活实例在续约 → 拒绝。任一 List 失败同样拒绝。
func checkNoLiveLease(ctx context.Context, store objectstore.BlobStore, base string, wait time.Duration, out io.Writer) error {
	keys0, err := listAll(ctx, store, base)
	if err != nil {
		return fmt.Errorf("reset: list leases: %w", err)
	}
	obs0 := leaseEpochs(keys0)
	fmt.Fprintf(out, "reset: %d databases have lease records; observing %s for live writers...\n", len(obs0), wait)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
	}
	keys1, err := listAll(ctx, store, base)
	if err != nil {
		return fmt.Errorf("reset: list leases: %w", err)
	}
	obs1 := leaseEpochs(keys1)
	var live []string
	for db, e := range obs1 {
		if e > obs0[db] {
			live = append(live, db)
		}
	}
	if len(live) > 0 {
		sort.Strings(live)
		return fmt.Errorf("reset: live write lease detected for %d database(s) (e.g. %s); stop ALL instances first", len(live), live[0])
	}
	return nil
}

// deleteRemote 删除 {base} 下全部对象：先项目数据，再 _system/ 下的实例标记与
// CAS 探针，reset.lock 最后由调用方删除。任一批失败即停止。
// 注意：s3.prefix 为空时 base 是整个桶，请确保桶由本实例独占。
func deleteRemote(ctx context.Context, store objectstore.BlobStore, base string, kb objectstore.KeyBuilder, groups map[string]int, out io.Writer) (int, error) {
	keys, err := listAll(ctx, store, base)
	if err != nil {
		return 0, fmt.Errorf("reset: list remote prefix: %w", err)
	}
	lockKey := kb.ResetLockKey()
	formatKey := kb.InstanceFormatKey()
	var data, tail []string
	for _, k := range keys {
		switch {
		case k == lockKey:
			continue
		case k == formatKey || groupOf(base, k) == "system":
			tail = append(tail, k)
		default:
			data = append(data, k)
		}
	}
	total := 0
	for _, batch := range [][]string{data, tail} {
		for len(batch) > 0 {
			n := len(batch)
			if n > 1000 {
				n = 1000
			}
			if err := store.DeleteMany(ctx, batch[:n]); err != nil {
				return total, fmt.Errorf("reset: delete batch (%d deleted so far): %w", total, err)
			}
			for _, k := range batch[:n] {
				groups[groupOf(base, k)]++
			}
			total += n
			fmt.Fprintf(out, "reset: deleted %d remote objects\n", total)
			batch = batch[n:]
		}
	}
	return total, nil
}

// groupOf 把 key 归到 "system"（_system/ 下的平台对象）或 "project"（项目数据）。
func groupOf(base, key string) string {
	rest := strings.TrimPrefix(key, base)
	rest = strings.TrimPrefix(rest, "/")
	if i := strings.IndexByte(rest, '/'); i > 0 {
		if rest[:i] == objectstore.SystemPrefix {
			return "system"
		}
	}
	if rest != "" {
		return "project"
	}
	return "other"
}

func listAll(ctx context.Context, store objectstore.BlobStore, prefix string) ([]string, error) {
	var all []string
	cursor := ""
	for {
		keys, next, err := store.List(ctx, prefix, cursor, 1000)
		if err != nil {
			return nil, err
		}
		all = append(all, keys...)
		if next == "" {
			return all, nil
		}
		cursor = next
	}
}
