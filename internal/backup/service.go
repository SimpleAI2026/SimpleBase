// 文件说明：service.go — internal/backup 中 Catalog、Service 等符号的实现
//
// 用法
//
//	NewService(cfg config.BackupConfig, instance string, catalog Catalog, registry ...
//	(s *Service) Create(ctx context.Context, tenantID, projectID string) (Item, error)
//	(s *Service) List(ctx context.Context, projectID string) ([]Item, error)
//	同包调用：Create() → repository.go, scheduler.go；Service → restore.go, scheduler.go；List() → repository.go
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/backup/service.go
//	         │ 依赖
//	         ▼
//	[下游] internal/dbbridge, godatabase, godatabase/filedb, godatabase/registry 等
//	[同包] model.go, repository.go, restore.go, scheduler.go
//	[关键引用] Create() ← repository.go, scheduler.go
//
// 主要符号
//
//	type Catalog interface {…}
//	type Service struct {…}
//	NewService(cfg config.BackupConfig, instance string, catalog Catalog, registry ...
//	(s *Service) Create(ctx context.Context, tenantID, projectID string) (Item, error)
//	(s *Service) List(ctx context.Context, projectID string) ([]Item, error)
//	(s *Service) Delete(ctx context.Context, projectID, id string) error
//	... 另有 6 个顶层符号

package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/linkxzhou/SimpleBase/internal/dbbridge"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/filedb"
	"github.com/linkxzhou/SimpleBase/godatabase/registry"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

type Catalog interface {
	ResolveProjectStorage(context.Context, string, string) (catalog.Database, error)
}

type Service struct {
	cfg      config.BackupConfig
	instance string
	catalog  Catalog
	registry *registry.Registry
	factory  *filedb.Factory
	repo     *Repository
	blobs    objectstore.BlobStore
	keys     objectstore.KeyBuilder
	now      func() time.Time
}

func NewService(cfg config.BackupConfig, instance string, catalog Catalog, registry *registry.Registry, factory *filedb.Factory, repo *Repository, blobs objectstore.BlobStore, keys objectstore.KeyBuilder) *Service {
	return &Service{cfg: cfg, instance: instance, catalog: catalog, registry: registry, factory: factory, repo: repo, blobs: blobs, keys: keys, now: time.Now}
}

func (s *Service) Create(ctx context.Context, tenantID, projectID string) (Item, error) {
	if !s.cfg.Enabled {
		return Item{}, ErrDisabled
	}
	if s.catalog == nil || s.registry == nil || s.factory == nil || s.repo == nil {
		return Item{}, ErrUnavailable
	}
	if s.cfg.UploadToS3 && s.blobs == nil {
		return Item{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, s.cfg.SnapshotTimeout)
	defer cancel()
	meta, err := s.catalog.ResolveProjectStorage(ctx, tenantID, projectID)
	if err != nil {
		return Item{}, err
	}
	engine, err := catalog.DatabaseEngine(meta.DatabasePath)
	if err != nil {
		return Item{}, err
	}
	id := uuid.NewString()
	dir := filepath.Join(s.cfg.Dir, projectID)
	staging := filepath.Join(dir, id+".tmp")
	defer os.Remove(staging)
	lease, err := s.registry.Acquire(ctx, dbbridge.SpecOf(meta), godatabase.ReadWrite)
	if err != nil {
		return Item{}, err
	}
	source, err := s.factory.DatabaseFilePath(dbbridge.SpecOf(meta))
	var snapshotInfo godatabase.SnapshotInfo
	if err == nil {
		snapshotInfo, err = filedb.SnapshotFile(ctx, lease.Handle.Conn(), source, engine, staging)
	}
	lease.Release()
	if err != nil {
		return Item{}, err
	}
	ext := ".duckdb"
	if engine == catalog.EngineSQLite {
		ext = ".sqlite"
	}
	item := Item{ID: id, ProjectID: projectID, StorageKey: id + ext, SizeBytes: snapshotInfo.SizeBytes, SHA256: snapshotInfo.SHA256, Engine: engine, FormatVersion: snapshotInfo.FormatVer, Status: "ready", CreatedAt: snapshotInfo.CompletedAt}
	item.ExpiresAt = expiry(item.CreatedAt, s.cfg.KeepDays)
	final := s.localPath(item)
	if err := os.Rename(staging, final); err != nil {
		return Item{}, fmt.Errorf("backup: commit snapshot: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(final)
		}
	}()
	manifest, _ := json.Marshal(Manifest{Item: item, InstanceID: s.instance})
	if err := os.WriteFile(s.manifestPath(item), manifest, 0o600); err != nil {
		_ = os.Remove(s.manifestPath(item))
		return Item{}, fmt.Errorf("backup: write manifest: %w", err)
	}
	if s.cfg.UploadToS3 {
		data, err := os.ReadFile(final)
		if err != nil {
			return Item{}, fmt.Errorf("backup: read snapshot: %w", err)
		}
		key := s.remoteKey(item)
		if err := s.blobs.PutBytes(ctx, key, data, "application/octet-stream"); err != nil {
			return Item{}, fmt.Errorf("backup: upload snapshot: %w", err)
		}
		if err := s.blobs.PutBytes(ctx, key+".manifest.json", manifest, "application/json"); err != nil {
			_ = s.blobs.Delete(context.Background(), key)
			return Item{}, fmt.Errorf("backup: upload manifest: %w", err)
		}
	}
	if err := s.repo.Create(ctx, item); err != nil {
		return Item{}, err
	}
	committed = true
	if err := s.Prune(ctx, projectID); err != nil {
		return item, err
	}
	return item, nil
}

func (s *Service) List(ctx context.Context, projectID string) ([]Item, error) {
	if !s.cfg.Enabled {
		return nil, ErrDisabled
	}
	return s.repo.List(ctx, projectID)
}

func (s *Service) Delete(ctx context.Context, projectID, id string) error {
	if !s.cfg.Enabled {
		return ErrDisabled
	}
	item, err := s.repo.Get(ctx, projectID, id)
	if err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, projectID, id); err != nil {
		return err
	}
	_ = os.Remove(s.localPath(item))
	_ = os.Remove(s.manifestPath(item))
	if s.cfg.UploadToS3 && s.blobs != nil {
		_ = s.blobs.Delete(ctx, s.remoteKey(item))
		_ = s.blobs.Delete(ctx, s.remoteKey(item)+".manifest.json")
	}
	return nil
}

func (s *Service) OpenDownload(ctx context.Context, projectID, id string) (io.ReadCloser, Item, error) {
	item, err := s.repo.Get(ctx, projectID, id)
	if err != nil {
		return nil, Item{}, err
	}
	if err := s.ensureLocal(ctx, item); err != nil {
		return nil, Item{}, err
	}
	file, err := os.Open(s.localPath(item))
	return file, item, err
}

func (s *Service) Prune(ctx context.Context, projectID string) error {
	items, err := s.repo.List(ctx, projectID)
	if err != nil {
		return err
	}
	cutoff := s.now().Add(-time.Duration(s.cfg.KeepDays) * 24 * time.Hour)
	for i, item := range items {
		expired := s.cfg.KeepDays > 0 && item.CreatedAt.Before(cutoff)
		if i >= s.cfg.MaxPerProject || i >= s.cfg.KeepLast && expired {
			if err := s.Delete(ctx, projectID, item.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) ensureLocal(ctx context.Context, item Item) error {
	path := s.localPath(item)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if s.blobs == nil {
		return ErrNotFound
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("backup: prepare download: %w", err)
	}
	_, err := s.blobs.DownloadFile(ctx, s.remoteKey(item), path)
	return err
}

func (s *Service) localPath(item Item) string {
	return filepath.Join(s.cfg.Dir, item.ProjectID, item.StorageKey)
}
func (s *Service) manifestPath(item Item) string { return s.localPath(item) + ".manifest.json" }
func (s *Service) remoteKey(item Item) string {
	parts := []string{}
	if prefix := strings.Trim(s.keys.RootPrefix, "/"); prefix != "" {
		parts = append(parts, prefix)
	}
	parts = append(parts, "_backups", item.ProjectID, item.StorageKey)
	return strings.Join(parts, "/")
}
