package ducklake

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
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

// ReadLocalState 读取水位文件；缺失或损坏返回零值（ok=false）。
func ReadLocalState(cacheDir, databaseID string) (LocalState, bool) {
	layout := layoutFor(cacheDir, databaseID)
	return readLocalStateAt(localStatePath(layout))
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

// SaveLocalState 原子写入水位文件（tmp + rename）。
func SaveLocalState(cacheDir, databaseID string, st LocalState) error {
	layout := layoutFor(cacheDir, databaseID)
	path := localStatePath(layout)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("ducklake: local-state dir: %w", err)
	}
	st.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	body, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return fmt.Errorf("ducklake: write local-state tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("ducklake: rename local-state: %w", err)
	}
	return nil
}
