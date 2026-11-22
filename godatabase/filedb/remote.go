package filedb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/atomicfile"
	"github.com/linkxzhou/SimpleBase/godatabase/blob"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlx"
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

// projectState 是单个项目的同步状态（C8：六个 per-project map 合并为单结构，
// 锁内一次查表即拿到全部字段，也为后续加代次字段留位）。
type projectState struct {
	bound      *bindState
	gen        int64 // R2：写代次，单调递增
	flushedGen int64 // R2：已由成功快照覆盖到的代次
	last       int64 // 最后成功上传的 manifest seq
	epoch      int64 // 最后成功上传使用的 epoch
	lost       bool  // 失租标记
	restored   bool  // 冷启动已拉取远端标记
}

// FileSyncer 把项目数据库文件以不可变快照 + manifest 链持久化到对象存储。
type FileSyncer struct {
	blobs     blob.Store
	keys      blob.KeyBuilder
	cacheDir  string
	interval  time.Duration
	keep      int
	logger    godatabase.Logger
	stateRoot string

	mu       sync.Mutex
	projects map[string]*projectState
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
		interval:  interval,
		keep:      keep,
		logger:    logger,
		stateRoot: filepath.Join(cacheDir, "projects"),
		projects:  map[string]*projectState{},
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
	st := s.stateOfLocked(projectID)
	st.lost = true
	st.gen = 0
	st.flushedGen = 0
	s.mu.Unlock()
}

// SetWriterEpoch 记录租约 epoch，用于快照键与 manifest。
func (s *FileSyncer) SetWriterEpoch(projectID string, epoch int64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	state := s.stateOfLocked(projectID)
	if epoch > state.epoch {
		state.epoch = epoch
	}
	s.mu.Unlock()
}

func (s *FileSyncer) flushDirty(ctx context.Context) {
	s.mu.Lock()
	ids := make([]string, 0, len(s.projects))
	for id, st := range s.projects {
		if st.gen > st.flushedGen {
			ids = append(ids, id)
		}
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
	state := s.stateOfLocked(meta.ProjectID)
	if state.restored {
		state.restored = false
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
	s.stateOfLocked(meta.ProjectID).bound = &bindState{db: sqlDB, meta: meta, filePath: filePath, engine: engine}
	s.mu.Unlock()
}

func (s *FileSyncer) MarkDirty(projectID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	st := s.stateOfLocked(projectID)
	if !st.lost {
		st.gen++
	}
	s.mu.Unlock()
}

func (s *FileSyncer) LastSynced(projectID string) int64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stateOfLocked(projectID).last
}

func (s *FileSyncer) SyncLag(projectID string) int64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.stateOfLocked(projectID)
	return st.gen - st.flushedGen
}

func (s *FileSyncer) setLast(projectID string, seq int64) {
	s.mu.Lock()
	if seq > s.stateOfLocked(projectID).last {
		s.stateOfLocked(projectID).last = seq
	}
	s.mu.Unlock()
}

// Flush 生成一致性快照并发布 manifest。
func (s *FileSyncer) Flush(ctx context.Context, projectID string) error {
	if s == nil || s.blobs == nil {
		return nil
	}
	s.mu.Lock()
	st := s.stateOfLocked(projectID)
	if st.lost {
		s.mu.Unlock()
		return nil
	}
	state := st.bound
	g0 := st.gen
	last := st.last
	epoch := st.epoch
	flushed := st.flushedGen
	s.mu.Unlock()
	if state == nil || g0 == 0 || g0 <= flushed {
		// 未绑定、从未 MarkDirty、或本代次已同步过：无需上传
		return nil
	}
	if epoch <= 0 {
		epoch = 1
	}
	seq := last + 1
	snapshotPath, digest, err := s.snapshot(ctx, state, projectID, seq, epoch)
	if err != nil {
		return err
	}
	defer os.Remove(snapshotPath)
	if digest == "" {
		// SQLite 路径：VACUUM INTO 后统一计算摘要（引擎内生成，无需复制）。
		digest, err = fileSHA256(snapshotPath)
		if err != nil {
			return err
		}
	}
	info, err := os.Stat(snapshotPath)
	if err != nil {
		return fmt.Errorf("filedb: stat snapshot: %w", err)
	}
	ext := path.Ext(state.meta.DatabasePath)
	key, err := s.keys.ProjectDatabaseSnapshotKey(projectID, seq, epoch, ext)
	if err != nil {
		return err
	}
	// P1：优先流式上传（blob.PutFiler），未实现时退回整读上传。
	if pf, ok := s.blobs.(blob.PutFiler); ok {
		if err := pf.PutFile(ctx, key, snapshotPath, "application/octet-stream"); err != nil {
			return fmt.Errorf("filedb: upload snapshot: %w", err)
		}
	} else {
		data, err := os.ReadFile(snapshotPath)
		if err != nil {
			return fmt.Errorf("filedb: read snapshot: %w", err)
		}
		if err := s.blobs.PutBytes(ctx, key, data, "application/octet-stream"); err != nil {
			return fmt.Errorf("filedb: upload snapshot: %w", err)
		}
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
	// R2：快照覆盖截至 g0 的全部写入，成功后 flushedGen 无条件单调推进到
	// g0；期间的新写入使 gen > flushedGen，仍保持待同步，下一轮上传新 seq。
	pst := s.stateOfLocked(projectID)
	if g0 > pst.flushedGen {
		pst.flushedGen = g0
	}
	if seq > pst.last {
		pst.last = seq
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
	if st := s.projects[projectID]; st != nil {
		st.bound = nil
	}
	s.mu.Unlock()
	return err
}

// stateOfLocked 返回（必要时创建）项目的同步状态。调用方必须持有 s.mu。
func (s *FileSyncer) stateOfLocked(projectID string) *projectState {
	st := s.projects[projectID]
	if st == nil {
		st = &projectState{}
		s.projects[projectID] = st
	}
	return st
}

// snapshot 生成可独立打开的副本；DuckDB 先 CHECKPOINT 再复制主文件
// （复制时经 io.MultiWriter 同步计算 sha256，P1：消除二次全文件读取），
// SQLite 使用 VACUUM INTO 得到一致性副本。
// 返回快照路径与摘要；摘要为空串表示未计算（SQLite 路径后续统一计算）。
func (s *FileSyncer) snapshot(ctx context.Context, state *bindState, projectID string, seq, epoch int64) (string, string, error) {
	stagingDir := filepath.Join(s.stateRoot, projectID, "staging")
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return "", "", fmt.Errorf("filedb: create staging directory: %w", err)
	}
	ext := path.Ext(state.meta.DatabasePath)
	target := filepath.Join(stagingDir, fmt.Sprintf("%020d-%020d%s", seq, epoch, ext))
	_ = os.Remove(target)
	switch state.engine {
	case godatabase.EngineSQLite:
		if _, err := state.db.ExecContext(ctx, "VACUUM INTO "+sqlx.Literal(target)); err != nil {
			return "", "", fmt.Errorf("filedb: sqlite vacuum into: %w", err)
		}
		return target, "", nil
	case godatabase.EngineDuckDB:
		if _, err := state.db.ExecContext(ctx, "CHECKPOINT"); err != nil {
			return "", "", fmt.Errorf("filedb: duckdb checkpoint: %w", err)
		}
		digest, err := copyFileHashed(state.filePath, target)
		if err != nil {
			return "", "", err
		}
		return target, digest, nil
	default:
		return "", "", fmt.Errorf("filedb: unsupported engine %q", state.engine)
	}
}

// copyFileHashed 复制文件并同步计算 sha256（P1：一遍 IO 完成复制与摘要）。
func copyFileHashed(src, dst string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", fmt.Errorf("filedb: open snapshot source: %w", err)
	}
	defer in.Close()
	h := sha256.New()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", fmt.Errorf("filedb: create snapshot: %w", err)
	}
	if _, err := io.Copy(io.MultiWriter(out, h), in); err != nil {
		out.Close()
		os.Remove(tmp)
		return "", fmt.Errorf("filedb: copy snapshot: %w", err)
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(tmp)
		return "", err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
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

// latestManifest 优先从本地序号向后探测；冷缓存时通过列表定位最新序号。
func (s *FileSyncer) latestManifest(ctx context.Context, projectID string) (*Manifest, error) {
	prefix, err := s.keys.ProjectDatabasePrefix(projectID)
	if err != nil {
		return nil, err
	}
	manifestPrefix := prefix + "/manifest/"
	if local, localErr := s.readLocalState(projectID); localErr == nil && local != nil && local.Seq > 0 {
		best, err := s.manifestByAnchor(ctx, projectID, local.Seq)
		if err == nil {
			return best, nil
		}
	}
	return s.latestManifestByList(ctx, manifestPrefix)
}

func (s *FileSyncer) manifestByAnchor(ctx context.Context, projectID string, base int64) (*Manifest, error) {
	var best *Manifest
	for i := int64(1); i <= 32; i++ {
		key, err := s.keys.ProjectDatabaseManifestKey(projectID, base+i)
		if err != nil {
			return nil, err
		}
		manifest, err := s.readManifest(ctx, key)
		if err != nil {
			return nil, err
		}
		if manifest == nil {
			break
		}
		best = manifest
	}
	if best != nil {
		return best, nil
	}
	key, err := s.keys.ProjectDatabaseManifestKey(projectID, base)
	if err != nil {
		return nil, err
	}
	return s.readManifest(ctx, key)
}

func (s *FileSyncer) readManifest(ctx context.Context, key string) (*Manifest, error) {
	data, _, err := s.blobs.GetBytes(ctx, key)
	if err != nil {
		if errors.Is(err, blob.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("filedb: decode manifest: %w", err)
	}
	return &manifest, nil
}

func (s *FileSyncer) latestManifestByList(ctx context.Context, manifestPrefix string) (*Manifest, error) {
	var latestKey string
	var latestSeq int64
	cursor := ""
	for {
		keys, next, err := s.blobs.List(ctx, manifestPrefix, cursor, 1000)
		if err != nil {
			return nil, fmt.Errorf("filedb: list manifest: %w", err)
		}
		for _, key := range keys {
			seq, err := manifestSeq(key)
			if err == nil && (latestKey == "" || seq > latestSeq) {
				latestKey, latestSeq = key, seq
			}
		}
		if next == "" {
			break
		}
		cursor = next
	}
	if latestKey == "" {
		return nil, nil
	}
	manifest, err := s.readManifest(ctx, latestKey)
	if err != nil {
		return nil, fmt.Errorf("filedb: read manifest: %w", err)
	}
	if manifest == nil {
		return nil, fmt.Errorf("filedb: read manifest: %w", blob.ErrNotFound)
	}
	if manifest.Seq != latestSeq {
		return nil, fmt.Errorf("filedb: manifest seq mismatch")
	}
	return manifest, nil
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
