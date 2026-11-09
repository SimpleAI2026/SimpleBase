// 文件说明：local_catalog.go — godatabase/ducklake 中 EnsureLocalCatalog：系统库本地 catalog 的下载/校验/修复
// 由 catalog_syncer.go 按职责拆分而来（C6，纯移动，不改逻辑）。

package ducklake

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/blob"
	"github.com/linkxzhou/SimpleBase/godatabase/internal/hashx"
)

// EnsureLocalCatalog 冷启动：把远端最新 catalog 拉到本地（multi-instance-consistency-plan §4.3）。
//
// v3 行为（ducklake-duckdb-catalog-plan §3/§4.4）：
//  1. 读远端 manifest 序列，取最大 seq 的 snapshot_key 下载；
//     manifest 的 catalog_engine 与配置不一致 → ErrCatalogEngineMismatch，不下载；
//  2. 无 manifest = 新库；
//  3. 本地 local-state.json.snapshot_id > 远端水位 → 不下载（本地领先，
//     Warn + 计数），返回 localAhead=true，调用方应触发一次 Sync 推上去；
//  4. state 损坏/缺失但 catalog 文件存在 → 保守视为 0，允许下载；
//  5. 下载后按 manifest.size/sha256 校验，通过后才 rename 覆盖；
//     覆盖前删除本地残留 .wal（DuckDB WAL 与新 catalog 不匹配会损坏）。
func EnsureLocalCatalog(ctx context.Context, store blob.Store, remote RemoteStorage, cacheDir string, meta godatabase.Spec, engine string) (downloaded bool, err error) {
	if store == nil || !remote.Enabled {
		return false, nil
	}
	engine = NormalizeCatalogEngine(engine)

	layout := layoutForEngine(cacheDir, meta.ID, engine)
	if err := layout.ensure(); err != nil {
		return false, err
	}

	// 本地水位先读：SyncedSeq 作为远端发现锚点（O(1) 探测），
	// 并用于下方本地领先比较。仅当本地 catalog 文件确实存在时才信任锚点。
	local, hasState := ReadLocalState(cacheDir, meta.ID)
	anchor := int64(0)
	if hasState && local.SyncedSeq > 0 {
		if _, statErr := os.Stat(layout.CatalogFile); statErr == nil {
			anchor = local.SyncedSeq
		}
	}

	// 远端最新水位：manifest 序列。
	latest, err := ReadLatestManifest(ctx, store, remote, meta.ProjectID, meta.ID, anchor)
	if err != nil && IsManifestChainBroken(err) && anchor > 0 {
		// 锚点失效（远端被重置/回退）：退化到全链发现再判一次；
		// 全链发现仍断链则 fail closed（§3.1：不得退回旧镜像）。
		latest, err = ReadLatestManifest(ctx, store, remote, meta.ProjectID, meta.ID, 0)
	}
	if err != nil {
		return false, err
	}
	if latest == nil {
		return false, nil // 新库：远端无任何 manifest
	}
	// §4.4 防御：manifest 引擎与配置不一致 → 拒绝下载。
	if NormalizeCatalogEngine(latest.CatalogEngine) != engine {
		return false, fmt.Errorf("%w: remote manifest seq %d was written with engine %q, config requires %q (project=%s db=%s)",
			ErrCatalogEngineMismatch, latest.Seq, latest.CatalogEngine, engine, meta.ProjectID, meta.ID)
	}

	remoteSnap := latest.SnapshotID
	remoteKey := latest.SnapshotKey
	// §4.4 防御：snapshot_key 后缀必须与配置引擎一致（防手工篡改/残留）。
	if remoteKey != "" && !strings.HasSuffix(remoteKey, catalogExtFor(engine)) {
		return false, fmt.Errorf("%w: remote snapshot key %q suffix does not match engine %q (project=%s db=%s)",
			ErrCatalogEngineMismatch, remoteKey, engine, meta.ProjectID, meta.ID)
	}

	if _, statErr := os.Stat(layout.CatalogFile); statErr == nil && hasState && local.SnapshotID > 0 {
		if local.SnapshotID > remoteSnap && remoteSnap > 0 {
			// 本地领先远端（Sync 失败路径，§3.8）：不覆盖。
			return false, errLocalAhead{dbID: meta.ID, local: local.SnapshotID, remote: remoteSnap}
		}
		if local.SnapshotID == remoteSnap && remoteSnap > 0 {
			// 已是最新：跳过下载。
			return false, nil
		}
	}

	if remoteKey == "" {
		return false, nil
	}
	// staging 下载成功后校验 + rename 覆盖（避免半写文件）。
	tmp := layout.CatalogFile + ".dl.tmp"
	if _, err := store.DownloadFile(ctx, remoteKey, tmp); err != nil {
		_ = os.Remove(tmp)
		return false, fmt.Errorf("ducklake: download catalog: %w", err)
	}
	data, err := os.ReadFile(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return false, fmt.Errorf("ducklake: read downloaded catalog: %w", err)
	}
	if err := verifySnapshotBody(data, latest); err != nil {
		_ = os.Remove(tmp)
		return false, err
	}
	// 覆盖前清理本地残留 WAL（与旧 catalog 配套，不清理会导致新 catalog + 旧 WAL）。
	if wal := layout.WALFile(engine); wal != "" {
		if err := os.Remove(wal); err != nil && !os.IsNotExist(err) {
			_ = os.Remove(tmp)
			return false, fmt.Errorf("ducklake: remove stale catalog WAL: %w", err)
		}
	}
	if err := os.Rename(tmp, layout.CatalogFile); err != nil {
		_ = os.Remove(tmp)
		return false, fmt.Errorf("ducklake: rename downloaded catalog: %w", err)
	}

	// 同步本地水位：远端快照已知时记录，供下次 Open 比较与 syncOnce 推进。
	if remoteSnap > 0 {
		st := local
		if !hasState {
			st = LocalState{}
		}
		st.SnapshotID = remoteSnap
		st.SyncedSnapshotID = remoteSnap
		if latest != nil {
			st.SyncedSeq = latest.Seq
		}
		if err := SaveLocalState(cacheDir, meta.ID, st); err != nil {
			// 水位文件写失败不阻塞打开（下次 syncOnce 会重写）。
			_ = err
		}
	}
	return true, nil
}

// verifySnapshotBody 校验下载的快照与 manifest 记录一致（size + sha256）。
func verifySnapshotBody(data []byte, m *Manifest) error {
	if m == nil || m.Size <= 0 || len(m.SHA256) != 64 {
		return fmt.Errorf("ducklake: catalog manifest missing required size or sha256")
	}
	if int64(len(data)) != m.Size {
		return fmt.Errorf("ducklake: downloaded catalog size %d != manifest %d (seq %d)", len(data), m.Size, m.Seq)
	}
	if hashx.Sum(data) != m.SHA256 {
		return fmt.Errorf("ducklake: downloaded catalog sha256 mismatch (seq %d)", m.Seq)
	}
	return nil
}

// errLocalAhead 表示本地 catalog 水位领先远端（§3.8 路径被拦截）。
type errLocalAhead struct {
	dbID          string
	local, remote int64
}

func (e errLocalAhead) Error() string {
	return fmt.Sprintf("ducklake: local catalog snapshot %d is ahead of remote %d for %s (skip download)",
		e.local, e.remote, e.dbID)
}

// IsLocalAhead 判定 EnsureLocalCatalog 返回的错误是否为「本地领先」。
func IsLocalAhead(err error) bool {
	var la errLocalAhead
	return errors.As(err, &la)
}
