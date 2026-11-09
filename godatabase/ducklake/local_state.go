// 文件说明：local_state.go — godatabase/ducklake 中 LocalState、ReadLocalState 等符号的实现
//
// 用法
//
//	ReadLocalState(cacheDir, databaseID string) (LocalState, bool)  // ReadLocalState 读取水位文件（catalog 工作副本位于
//	SaveLocalState(cacheDir, databaseID string, st LocalState) error  // SaveLocalState 原子写入水位文件（catalog 工作副本位于
//	同包调用：ReadLocalState() → catalog_syncer.go, factory.go；SaveLocalState() → catalog_syncer.go, factory.go；LocalState → catalog_syncer.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/app, internal/systemdb
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/ducklake/local_state.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase/internal/atomicfile
//	[同包] catalog_syncer.go, errors.go, extension.go, factory.go, gated_conn.go 等 14 个
//	[关键引用] ReadLocalState() ← catalog_syncer.go, factory.go
//
// 主要符号
//
//	type LocalState struct {…}
//	ReadLocalState(cacheDir, databaseID string) (LocalState, bool)
//	SaveLocalState(cacheDir, databaseID string, st LocalState) error
//	... 另有 3 个顶层符号

package ducklake

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase/internal/atomicfile"
)

// LocalState 是本地 catalog 副本的水位文件（multi-instance-consistency-plan §4.3）。
// 存于 {cache_dir}/{db-id}/catalog/local-state.json，由 syncOnce 成功后写入、
// AfterWrite 推进 snapshot_id。EnsureLocalCatalog 用它与远端 manifest 比较，
// 避免「本地领先时被远端旧版本覆盖」（§3.8 回滚覆盖）。
//
// 损坏/缺失时保守视为 0（允许下载），与 v1 语义一致。
type LocalState struct {
	SnapshotID       int64 `json:"snapshot_id"`
	SyncedSnapshotID int64 `json:"synced_snapshot_id"`
	SyncedSeq        int64 `json:"synced_manifest_seq"`
	// PrunedSeq 是快照清理进度（v2.0 §4-P1）：[1, PrunedSeq] 区间的过期
	// 快照已处理，下轮从 PrunedSeq+1 继续，避免每次从 seq 1 重扫。
	PrunedSeq int64  `json:"pruned_seq,omitempty"`
	ETag      string `json:"etag,omitempty"`
	UpdatedAt string `json:"updated_at"`
}

func localStatePath(layout Layout) string {
	return filepath.Join(filepath.Dir(layout.CatalogFile), "local-state.json")
}

// ReadLocalState 读取水位文件（catalog 工作副本位于 {cacheDir}/{databaseID} 时使用）。
// 缺失或损坏返回零值（ok=false）。
func ReadLocalState(cacheDir, databaseID string) (LocalState, bool) {
	return readLocalStateAt(localStatePath(layoutForEngine(cacheDir, databaseID, EngineDuckDB)))
}

// SaveLocalState 原子写入水位文件（catalog 工作副本位于 {cacheDir}/{databaseID} 时使用）。
func SaveLocalState(cacheDir, databaseID string, st LocalState) error {
	return saveLocalStateAt(localStatePath(layoutForEngine(cacheDir, databaseID, EngineDuckDB)), st)
}

func readLocalStateAt(path string) (LocalState, bool) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return LocalState{}, false
	}
	var st LocalState
	if err := json.Unmarshal(data, &st); err != nil {
		return LocalState{}, false
	}
	return st, true
}

// saveLocalStateAt 原子写入水位文件（tmp + rename）。
func saveLocalStateAt(path string, st LocalState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("ducklake: local-state dir: %w", err)
	}
	st.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	body, err := json.Marshal(st)
	if err != nil {
		return err
	}
	if err := atomicfile.Write(path, body, 0o644); err != nil {
		return fmt.Errorf("ducklake: write local-state: %w", err)
	}
	return nil
}
