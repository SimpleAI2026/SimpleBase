// 文件说明：remote.go — godatabase/filedb 中 Manifest、FileSyncer 等符号的实现
//
// 用法
//
//	NewFileSyncer(blobs blob.Store, keys blob.KeyBuilder, cacheDir string, interval ...
//	(s *FileSyncer) Start(ctx context.Context)  // Start 启动后台周期性 Flush（幂等）
//	(s *FileSyncer) Close() error  // Close 停止后台循环
//	同包调用：snapshot() → factory.go, path.go 等；Close() → factory.go, snapshot.go；Restore() → factory.go, syncer.go
//
// 调用架构
//
//	[上游] internal/app, internal/backup
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/filedb/remote.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase, godatabase/blob, godatabase/internal/atomicfile
//	[同包] factory.go, path.go, restore.go, restore_remote.go, snapshot.go 等 6 个
//	[关键引用] snapshot() ← factory.go, path.go, snapshot.go
//
// 主要符号
//
//	type Manifest struct {…}
//	type FileSyncer struct {…}
//	NewFileSyncer(blobs blob.Store, keys blob.KeyBuilder, cacheDir string, interval ...
//	(s *FileSyncer) Start(ctx context.Context)
//	(s *FileSyncer) Close() error
//	(s *FileSyncer) CloseNoFlush(projectID string)
//	... 另有 26 个顶层符号

package filedb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/godatabase/blob"
	"github.com/linkxzhou/SimpleBase/godatabase/internal/atomicfile"
)

const fileManifestFormatVersion = 1

// Manifest 记录项目数据库文件的一次不可变快照。
type Manifest struct {
	FormatVersion int       `json:"format_version"`
	ProjectID     string    `json:"project_id"`
	Engine        string    `json:"engine"`
	DatabasePath  string    `json:"database_path"`
	Seq           int64     `json:"seq"`
	Epoch         int64     `json:"epoch"`
	SnapshotKey   string    `json:"snapshot_key"`
	Size          int64     `json:"size"`
	SHA256        string    `json:"sha256"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type localState struct {
	Seq          int64     `json:"seq"`
	Epoch        int64     `json:"epoch"`
	DatabasePath string    `json:"database_path"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type bindState struct {
	db       *sql.DB
	meta     godatabase.Spec
	filePath string
	engine   string
}

// FileSyncer 把项目数据库文件以不可变快照 + manifest 链持久化到对象存储。
type FileSyncer struct {
	blobs     blob.Store
	keys      blob.KeyBuilder
	cacheDir  string
	remote    bool
	interval  time.Duration
	keep      int
	logger    godatabase.Logger
	stateRoot string

	mu       sync.Mutex
	bound    map[string]*bindState
	dirty    map[string]bool
	last     map[string]int64
	epoch    map[string]int64
	lost     map[string]bool
	restored map[string]bool
	closed   bool
	stopCh   chan struct{}
	doneCh   chan struct{}
	started  bool
}

// NewFileSyncer 构造远端同步器；interval <= 0 时取 15s，keep <= 0 时保留 10 版。
func NewFileSyncer(blobs blob.Store, keys blob.KeyBuilder, cacheDir string, interval time.Duration, keep int, logger godatabase.Logger) *FileSyncer {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	if keep <= 0 {
		keep = 10
	}
	return &FileSyncer{
		blobs:     blobs,
		keys:      keys,
		cacheDir:  cacheDir,
		remote:    true,
		interval:  interval,
		keep:      keep,
		logger:    logger,
		stateRoot: filepath.Join(cacheDir, "projects"),
		bound:     map[string]*bindState{},
		dirty:     map[string]bool{},
		last:      map[string]int64{},
		epoch:     map[string]int64{},
		lost:      map[string]bool{},
		restored:  map[string]bool{},
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
}

// Start 启动后台周期性 Flush（幂等）。
func (s *FileSyncer) Start(ctx context.Context) {
	if s == nil || s.blobs == nil {
		return
	}
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()
	go func() {
		defer close(s.doneCh)
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stopCh:
				return
			case <-ticker.C:
				s.flushDirty(ctx)
			}
		}
	}()
}

// Close 停止后台循环；不再发起新的 PUT。
func (s *FileSyncer) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	started := s.started
	s.mu.Unlock()
	if started {
		close(s.stopCh)
		<-s.doneCh
	}
	return nil
}

// CloseNoFlush 标记项目失租：后续 Flush/Unbind 不再上传。
func (s *FileSyncer) CloseNoFlush(projectID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.lost[projectID] = true
	delete(s.dirty, projectID)
	s.mu.Unlock()
}

// SetWriterEpoch 记录租约 epoch，用于快照键与 manifest。
func (s *FileSyncer) SetWriterEpoch(projectID string, epoch int64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if epoch > s.epoch[projectID] {
		s.epoch[projectID] = epoch
	}
	s.mu.Unlock()
}

func (s *FileSyncer) flushDirty(ctx context.Context) {
	s.mu.Lock()
	ids := make([]string, 0, len(s.dirty))
	for id := range s.dirty {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	sort.Strings(ids)
	for _, id := range ids {
		if err := s.Flush(ctx, id); err != nil && s.logger != nil {
			s.logger.Warn("filedb: flush project database failed", zap.String("project_id", id), zap.Error(err))
		}
	}
}

// Restore 冷启动恢复：远端 manifest 更新时下载并覆盖本地工作副本。
func (s *FileSyncer) Restore(ctx context.Context, meta godatabase.Spec, filePath string) error {
	if s == nil || s.blobs == nil {
		return nil
	}
	s.mu.Lock()
	if s.restored[meta.ProjectID] {
		delete(s.restored, meta.ProjectID)
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	manifest, err := s.latestManifest(ctx, meta.ProjectID)
	if err != nil {
		return err
	}
	if manifest == nil {
		return nil
	}
	if manifest.DatabasePath != meta.DatabasePath {
		return fmt.Errorf("filedb: remote manifest database path mismatch")
	}
	local, err := s.readLocalState(meta.ProjectID)
	if err != nil {
		return err
	}
	if local != nil && local.Seq >= manifest.Seq {
		s.setLast(meta.ProjectID, local.Seq)
		return nil
	}
	if err := s.downloadSnapshot(ctx, meta, manifest, filePath); err != nil {
		return err
	}
	s.setLast(meta.ProjectID, manifest.Seq)
	if err := s.writeLocalState(meta.ProjectID, localState{
		Seq: manifest.Seq, Epoch: manifest.Epoch, DatabasePath: manifest.DatabasePath, UpdatedAt: time.Now().UTC(),
	}); err != nil {
		return err
	}
	return nil
}

func (s *FileSyncer) Bind(meta godatabase.Spec, sqlDB *sql.DB, filePath string) {
	if s == nil {
		return
	}
	engine, _ := godatabase.EngineFromPath(meta.DatabasePath)
	s.mu.Lock()
	s.bound[meta.ProjectID] = &bindState{db: sqlDB, meta: meta, filePath: filePath, engine: engine}
	s.mu.Unlock()
}

func (s *FileSyncer) MarkDirty(projectID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if !s.lost[projectID] {
		s.dirty[projectID] = true
	}
	s.mu.Unlock()
}

func (s *FileSyncer) LastSynced(projectID string) int64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last[projectID]
}

func (s *FileSyncer) SyncLag(projectID string) int64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dirty[projectID] {
		return 1
	}
	return 0
}

func (s *FileSyncer) setLast(projectID string, seq int64) {
	s.mu.Lock()
	if seq > s.last[projectID] {
		s.last[projectID] = seq
	}
	s.mu.Unlock()
}

// Flush 生成一致性快照并发布 manifest。
func (s *FileSyncer) Flush(ctx context.Context, projectID string) error {
	if s == nil || s.blobs == nil {
		return nil
	}
	s.mu.Lock()
	if s.lost[projectID] {
		s.mu.Unlock()
		return nil
	}
	state, ok := s.bound[projectID]
	dirty := s.dirty[projectID]
	last := s.last[projectID]
	epoch := s.epoch[projectID]
	s.mu.Unlock()
	if !ok || !dirty {
		return nil
	}
	if epoch <= 0 {
		epoch = 1
	}
	seq := last + 1
	snapshotPath, err := s.snapshot(ctx, state, projectID, seq, epoch)
	if err != nil {
		return err
	}
	defer os.Remove(snapshotPath)
	info, err := os.Stat(snapshotPath)
	if err != nil {
		return fmt.Errorf("filedb: stat snapshot: %w", err)
	}
	digest, err := fileSHA256(snapshotPath)
	if err != nil {
		return err
	}
	ext := path.Ext(state.meta.DatabasePath)
	key, err := s.keys.ProjectDatabaseSnapshotKey(projectID, seq, epoch, ext)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		return fmt.Errorf("filedb: read snapshot: %w", err)
	}
	if err := s.blobs.PutBytes(ctx, key, data, "application/octet-stream"); err != nil {
		return fmt.Errorf("filedb: upload snapshot: %w", err)
	}
	manifest := Manifest{
		FormatVersion: fileManifestFormatVersion,
		ProjectID:     projectID,
		Engine:        state.engine,
		DatabasePath:  state.meta.DatabasePath,
		Seq:           seq,
		Epoch:         epoch,
		SnapshotKey:   key,
		Size:          info.Size(),
		SHA256:        digest,
		UpdatedAt:     time.Now().UTC(),
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	manifestKey, err := s.keys.ProjectDatabaseManifestKey(projectID, seq)
	if err != nil {
		return err
	}
	if _, err := s.blobs.PutIfAbsent(ctx, manifestKey, body, "application/json"); err != nil {
		if errors.Is(err, blob.ErrPreconditionFailed) {
			s.CloseNoFlush(projectID)
			return fmt.Errorf("filedb: manifest conflict at seq %d (split brain): %w", seq, err)
		}
		return fmt.Errorf("filedb: publish manifest: %w", err)
	}
	s.mu.Lock()
	s.dirty[projectID] = false
	if seq > s.last[projectID] {
		s.last[projectID] = seq
	}
	s.mu.Unlock()
	if err := s.writeLocalState(projectID, localState{
		Seq: seq, Epoch: epoch, DatabasePath: state.meta.DatabasePath, UpdatedAt: time.Now().UTC(),
	}); err != nil && s.logger != nil {
		s.logger.Warn("filedb: write local state failed", zap.String("project_id", projectID), zap.Error(err))
	}
	if err := s.prune(ctx, projectID, seq); err != nil && s.logger != nil {
		s.logger.Warn("filedb: prune old snapshots failed", zap.String("project_id", projectID), zap.Error(err))
	}
	return nil
}

// Unbind 关闭前刷新并解绑。
func (s *FileSyncer) Unbind(ctx context.Context, projectID string) error {
	if s == nil {
		return nil
	}
	err := s.Flush(ctx, projectID)
	s.mu.Lock()
	delete(s.bound, projectID)
	s.mu.Unlock()
	return err
}

// snapshot 生成可独立打开的副本；DuckDB 先 CHECKPOINT 再复制主文件，
// SQLite 使用 VACUUM INTO 得到一致性副本。
func (s *FileSyncer) snapshot(ctx context.Context, state *bindState, projectID string, seq, epoch int64) (string, error) {
	stagingDir := filepath.Join(s.stateRoot, projectID, "staging")
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return "", fmt.Errorf("filedb: create staging directory: %w", err)
	}
	ext := path.Ext(state.meta.DatabasePath)
	target := filepath.Join(stagingDir, fmt.Sprintf("%020d-%020d%s", seq, epoch, ext))
	_ = os.Remove(target)
	switch state.engine {
	case godatabase.EngineSQLite:
		if _, err := state.db.ExecContext(ctx, "VACUUM INTO "+sqlQuote(target)); err != nil {
			return "", fmt.Errorf("filedb: sqlite vacuum into: %w", err)
		}
	case godatabase.EngineDuckDB:
		if _, err := state.db.ExecContext(ctx, "CHECKPOINT"); err != nil {
			return "", fmt.Errorf("filedb: duckdb checkpoint: %w", err)
		}
		if err := copyFile(state.filePath, target); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("filedb: unsupported engine %q", state.engine)
	}
	return target, nil
}

func sqlQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("filedb: open snapshot source: %w", err)
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("filedb: create snapshot: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return fmt.Errorf("filedb: copy snapshot: %w", err)
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func fileSHA256(name string) (string, error) {
	f, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (s *FileSyncer) latestManifest(ctx context.Context, projectID string) (*Manifest, error) {
	prefix, err := s.keys.ProjectDatabasePrefix(projectID)
	if err != nil {
		return nil, err
	}
	var best *Manifest
	cursor := ""
	for {
		keys, next, err := s.blobs.List(ctx, prefix+"/manifest/", cursor, 1000)
		if err != nil {
			return nil, fmt.Errorf("filedb: list manifest: %w", err)
		}
		for _, key := range keys {
			seq, err := manifestSeq(key)
			if err != nil {
				continue
			}
			if best != nil && seq <= best.Seq {
				continue
			}
			data, _, err := s.blobs.GetBytes(ctx, key)
			if err != nil {
				return nil, fmt.Errorf("filedb: read manifest: %w", err)
			}
			var m Manifest
			if err := json.Unmarshal(data, &m); err != nil {
				return nil, fmt.Errorf("filedb: decode manifest: %w", err)
			}
			if m.Seq != seq {
				return nil, fmt.Errorf("filedb: manifest seq mismatch")
			}
			best = &m
		}
		if next == "" {
			break
		}
		cursor = next
	}
	return best, nil
}

func manifestSeq(key string) (int64, error) {
	base := path.Base(key)
	if !strings.HasSuffix(base, ".json") {
		return 0, fmt.Errorf("filedb: not a manifest key")
	}
	return strconv.ParseInt(strings.TrimSuffix(base, ".json"), 10, 64)
}

func (s *FileSyncer) downloadSnapshot(ctx context.Context, meta godatabase.Spec, m *Manifest, filePath string) error {
	if m.SnapshotKey == "" || m.SHA256 == "" {
		return fmt.Errorf("filedb: remote manifest missing snapshot metadata")
	}
	tmp := filePath + ".dl.tmp"
	if _, err := s.blobs.DownloadFile(ctx, m.SnapshotKey, tmp); err != nil {
		return fmt.Errorf("filedb: download snapshot: %w", err)
	}
	digest, err := fileSHA256(tmp)
	if err != nil {
		os.Remove(tmp)
		return err
	}
	if digest != m.SHA256 {
		os.Remove(tmp)
		return fmt.Errorf("filedb: snapshot checksum mismatch for project %s", meta.ProjectID)
	}
	engine, _ := godatabase.EngineFromPath(meta.DatabasePath)
	for _, name := range sidecarFiles(filePath, engine) {
		if name == filePath {
			continue
		}
		_ = os.Remove(name)
	}
	if err := os.Rename(tmp, filePath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("filedb: install downloaded snapshot: %w", err)
	}
	return nil
}

func (s *FileSyncer) localStatePath(projectID string) string {
	return filepath.Join(s.stateRoot, projectID, "local-state.json")
}

func (s *FileSyncer) readLocalState(projectID string) (*localState, error) {
	data, err := os.ReadFile(s.localStatePath(projectID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var st localState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("filedb: decode local state: %w", err)
	}
	return &st, nil
}

func (s *FileSyncer) writeLocalState(projectID string, st localState) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	dir := filepath.Join(s.stateRoot, projectID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return atomicfile.Write(s.localStatePath(projectID), data, 0o644)
}

func (s *FileSyncer) prune(ctx context.Context, projectID string, latest int64) error {
	if s.keep <= 0 || latest <= int64(s.keep) {
		return nil
	}
	prefix, err := s.keys.ProjectDatabasePrefix(projectID)
	if err != nil {
		return err
	}
	keys, _, err := s.blobs.List(ctx, prefix+"/snapshots/", "", 1000)
	if err != nil {
		return err
	}
	cutoff := latest - int64(s.keep)
	var obsolete []string
	for _, key := range keys {
		seq, err := snapshotSeq(key)
		if err != nil || seq > cutoff {
			continue
		}
		obsolete = append(obsolete, key)
	}
	if len(obsolete) == 0 {
		return nil
	}
	return s.blobs.DeleteMany(ctx, obsolete)
}

func snapshotSeq(key string) (int64, error) {
	base := path.Base(key)
	if i := strings.LastIndex(base, "-"); i > 0 {
		base = base[:i]
	}
	return strconv.ParseInt(base, 10, 64)
}

// PurgeProject 删除项目数据库快照、manifest 与租约对象。
func (s *FileSyncer) PurgeProject(ctx context.Context, projectID string) error {
	if s == nil || s.blobs == nil {
		return nil
	}
	prefix, err := s.keys.ProjectDatabasePrefix(projectID)
	if err != nil {
		return err
	}
	cursor := ""
	for {
		keys, next, err := s.blobs.List(ctx, prefix+"/", cursor, 1000)
		if err != nil {
			return fmt.Errorf("filedb: list project database objects: %w", err)
		}
		if len(keys) > 0 {
			if err := s.blobs.DeleteMany(ctx, keys); err != nil {
				return err
			}
		}
		if next == "" {
			break
		}
		cursor = next
	}
	if err := os.RemoveAll(filepath.Join(s.stateRoot, projectID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
