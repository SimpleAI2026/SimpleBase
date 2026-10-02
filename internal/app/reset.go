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

// ResetOptions 控制 simplebased reset（ducklake-duckdb-catalog-plan §5）。
type ResetOptions struct {
	// Confirm 必须等于 cfg.Instance.ID，防止误清其他环境。
	Confirm string
	// LocalOnly 只清本地缓存（远端已由运维清空的场景）。
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
	RemoteByGroup  map[string]int // tenants / catalog / files / other
	LeaseDatabases int            // 观察到的持租库数量（判活用）
}

// resetLockTTL 仅供排障展示锁创建时的预期窗口；无条件删除原语不可安全自动接管。
const resetLockTTL = 30 * time.Minute

type resetLock struct {
	Owner     string `json:"owner"`
	ExpiresAt string `json:"expires_at"`
}

// Reset 清空本实例的全部数据（本地缓存 + {root}/{env}/ 下全部对象）。不可恢复。
// 不启动 HTTP 服务，也不打开任何数据库。可重复执行：中途失败后重跑会从剩余对象继续删。
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

func needRemoteReset(cfg config.Config, opts ResetOptions) bool {
	return !cfg.DevMode && !opts.LocalOnly && cfg.S3.Bucket != ""
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
	if !cfg.DevMode && os.Getenv("SIMPLEBASE_ALLOW_RESET") != "1" {
		return sum, errors.New("reset: refusing to run with dev_mode=false unless SIMPLEBASE_ALLOW_RESET=1 is set")
	}
	cacheDir, err := safeResetDir(cfg.Database.CacheDir)
	if err != nil {
		return sum, err
	}

	var kb objectstore.KeyBuilder
	var base string
	if store != nil {
		// 前缀两段都必须非空：否则 base 退化为 bucket 根或单段，可能误删其他环境。
		if strings.Trim(cfg.S3.Prefix, "/") == "" || strings.Trim(cfg.Instance.ID, "/") == "" {
			return sum, errors.New("reset: s3.prefix and instance.id must both be non-empty for remote reset")
		}
		kb = objectstore.KeyBuilder{RootPrefix: cfg.S3.Prefix, Environment: cfg.Instance.ID}
		base = strings.TrimSuffix(kb.CatalogPrefix(), "catalog") // "{root}/{env}/"
	}

	// —— 列出本地目标 ——
	entries, err := os.ReadDir(cacheDir)
	if err != nil && !os.IsNotExist(err) {
		return sum, fmt.Errorf("reset: read cache dir: %w", err)
	}
	for _, e := range entries {
		sum.LocalPaths = append(sum.LocalPaths, filepath.Join(cacheDir, e.Name()))
	}

	if opts.DryRun {
		fmt.Fprintf(out, "[dry-run] instance.id=%s\n", cfg.Instance.ID)
		fmt.Fprintf(out, "[dry-run] local: %d paths under %s\n", len(sum.LocalPaths), cacheDir)
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
			fmt.Fprintln(out, "[dry-run] remote: skipped (dev_mode / --local-only / no bucket)")
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
	fmt.Fprintf(out, "reset: removed %d local paths under %s\n", len(sum.LocalPaths), cacheDir)

	if store != nil {
		if err := store.Delete(ctx, kb.ResetLockKey()); err != nil {
			return sum, fmt.Errorf("reset: delete reset.lock: %w", err)
		}
		fmt.Fprintf(out, "reset: removed %d remote objects under s3://%s/%s %v\n",
			sum.RemoteObjects, cfg.S3.Bucket, base, sum.RemoteByGroup)
	}
	fmt.Fprintln(out, "reset: done. next start will write a fresh instance-format marker and rebuild the system database.")
	return sum, nil
}

// safeResetDir 路径安全检查：拒绝空路径、根目录与家目录。
func safeResetDir(dir string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", errors.New("reset: database.cache_dir is empty")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("reset: resolve cache dir: %w", err)
	}
	abs = filepath.Clean(abs)
	if abs == string(filepath.Separator) || abs == filepath.VolumeName(abs)+string(filepath.Separator) {
		return "", fmt.Errorf("reset: refusing to wipe root directory %s", abs)
	}
	if home, err := os.UserHomeDir(); err == nil && filepath.Clean(home) == abs {
		return "", fmt.Errorf("reset: refusing to wipe home directory %s", abs)
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

// leaseKeyRe 匹配 tenants/{t}/databases/{db}/catalog/lease/{epoch}.json。
var leaseKeyRe = regexp.MustCompile(`/tenants/([^/]+)/databases/([^/]+)/catalog/lease/(\d+)\.json$`)

// leaseEpochs 返回每个库观察到的最大租约 epoch。
func leaseEpochs(keys []string) map[string]int64 {
	out := map[string]int64{}
	for _, k := range keys {
		m := leaseKeyRe.FindStringSubmatch(k)
		if m == nil {
			continue
		}
		e, err := strconv.ParseInt(m[3], 10, 64)
		if err != nil {
			continue
		}
		id := m[1] + "/" + m[2]
		if e > out[id] {
			out[id] = e
		}
	}
	return out
}

// checkNoLiveLease 观察式判活（计划 §5.2 v2.1）：两次 List 之间任一库最大 epoch
// 增加 → 有存活实例在续约 → 拒绝。任一 List 失败同样拒绝。
func checkNoLiveLease(ctx context.Context, store objectstore.BlobStore, base string, wait time.Duration, out io.Writer) error {
	tenants := base + "tenants/"
	keys0, err := listAll(ctx, store, tenants)
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
	keys1, err := listAll(ctx, store, tenants)
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

// deleteRemote 删除 {base}/ 下全部对象：先 tenants/ 与 files/ 等数据，再实例标记与其他
// catalog/ 对象（CAS 探针），reset.lock 最后由调用方删除。任一批失败即停止。
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
		case k == formatKey || groupOf(base, k) == "catalog":
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

func groupOf(base, key string) string {
	rest := strings.TrimPrefix(key, base)
	if i := strings.IndexByte(rest, '/'); i > 0 {
		switch g := rest[:i]; g {
		case "tenants", "catalog", "files":
			return g
		}
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
