// 文件说明：restore.go — internal/backup 中 Restore 等符号的实现
//
// 用法
//
//	(s *Service) Restore(ctx context.Context, tenantID, projectID, id string) error
//	type restorePreparer（包内使用）
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/backup/restore.go
//	         │ 依赖
//	         ▼
//	[下游] internal/dbbridge, godatabase, godatabase/filedb, internal/catalog
//	[同包] model.go, repository.go, scheduler.go, service.go
//
// 主要符号
//
//	(s *Service) Restore(ctx context.Context, tenantID, projectID, id string) error
//	... 另有 3 个顶层符号

package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/linkxzhou/SimpleBase/internal/dbbridge"
	"io"
	"os"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/filedb"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

type restorePreparer interface {
	PrepareRestore(string) error
	MarkDirty(string)
}

func (s *Service) Restore(ctx context.Context, tenantID, projectID, id string) error {
	if !s.cfg.Enabled {
		return ErrDisabled
	}
	item, err := s.repo.Get(ctx, projectID, id)
	if err != nil {
		return err
	}
	meta, err := s.catalog.ResolveProjectStorage(ctx, tenantID, projectID)
	if err != nil {
		return err
	}
	engine, err := catalog.DatabaseEngine(meta.DatabasePath)
	if err != nil || engine != item.Engine || item.FormatVersion != 1 || item.Status != "ready" {
		return ErrSnapshotMismatch
	}
	if err := s.ensureLocal(ctx, item); err != nil {
		return err
	}
	if err := verifyBackupFile(s.localPath(item), item); err != nil {
		return err
	}
	if err := filedb.ValidateSnapshot(ctx, s.localPath(item), engine); err != nil {
		return ErrSnapshotMismatch
	}
	if err := s.registry.CloseDatabase(ctx, projectID); err != nil {
		return ErrProjectBusy
	}
	target, err := s.factory.DatabaseFilePath(dbbridge.SpecOf(meta))
	if err != nil {
		return err
	}
	staging := target + ".restore-tmp"
	old := target + ".restore-old"
	_ = os.Remove(staging)
	_ = os.Remove(old)
	defer os.Remove(staging)
	if err := copyBackupFile(s.localPath(item), staging); err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, old); err != nil {
			return fmt.Errorf("backup: preserve current database: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("backup: inspect current database: %w", err)
	}
	if err := os.Rename(staging, target); err != nil {
		_ = os.Rename(old, target)
		return fmt.Errorf("backup: install restored database: %w", err)
	}
	rollback := func() {
		_ = os.Remove(target)
		_ = os.Rename(old, target)
	}
	if syncer, ok := s.factory.Syncer.(restorePreparer); ok {
		if err := syncer.PrepareRestore(projectID); err != nil {
			rollback()
			return fmt.Errorf("backup: prepare remote restore: %w", err)
		}
		syncer.MarkDirty(projectID)
	}
	lease, err := s.registry.Acquire(ctx, dbbridge.SpecOf(meta), godatabase.ReadWrite)
	if err != nil {
		rollback()
		return err
	}
	lease.Release()
	if err := s.registry.CloseDatabase(ctx, projectID); err != nil {
		rollback()
		return err
	}
	if err := os.Remove(old); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("backup: clean previous database: %w", err)
	}
	return nil
}

func verifyBackupFile(path string, item Item) error {
	file, err := os.Open(path)
	if err != nil {
		return ErrSnapshotMismatch
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || stat.Size() != item.SizeBytes {
		return ErrSnapshotMismatch
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return ErrSnapshotMismatch
	}
	if hex.EncodeToString(hash.Sum(nil)) != item.SHA256 {
		return ErrSnapshotMismatch
	}
	return nil
}

func copyBackupFile(source, target string) (err error) {
	in, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("backup: open snapshot: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("backup: create restore staging: %w", err)
	}
	defer func() {
		if closeErr := out.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return fmt.Errorf("backup: copy restore staging: %w", err)
	}
	return out.Sync()
}
