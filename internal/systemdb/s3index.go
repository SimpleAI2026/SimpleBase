package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

// S3Object 是 sys_s3_objects 一行。
type S3Object struct {
	ID           string
	ProjectID    string
	Key          string
	Size         int64
	ETag         string
	ContentType  string
	LastModified time.Time
	CreatedAt    time.Time
	DeletedAt    *time.Time
}

// UpsertS3Object 上传成功后写入索引。
func (s *Store) UpsertS3Object(ctx context.Context, projectID, key string, size int64, etag, contentType string, lastMod time.Time) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	now := time.Now().UTC()
	if lastMod.IsZero() {
		lastMod = now
	}
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM sys_s3_objects WHERE project_id = ? AND object_key = ?`,
		projectID, key).Scan(&id)
	if err == nil {
		_, err = s.db.ExecContext(ctx,
			`UPDATE sys_s3_objects SET size=?, etag=?, content_type=?, last_modified=?, deleted_at=NULL WHERE id=?`,
			size, etag, contentType, lastMod.UTC(), id)
		if err != nil {
			return err
		}
		s.notifyWrite(ctx)
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO sys_s3_objects(id, project_id, object_key, size, etag, content_type, last_modified, created_at, deleted_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		uuid.NewString(), projectID, key, size, etag, contentType, lastMod.UTC(), now)
	if err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// SoftDeleteS3Object 删除成功后软删索引行。
func (s *Store) SoftDeleteS3Object(ctx context.Context, projectID, key string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE sys_s3_objects SET deleted_at = ? WHERE project_id = ? AND object_key = ? AND deleted_at IS NULL`,
		time.Now().UTC(), projectID, key)
	if err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// ListS3Objects 从索引表列出未删除对象。
func (s *Store) ListS3Objects(ctx context.Context, projectID, prefix string, limit int) ([]S3Object, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	q := `SELECT id, project_id, object_key, size, etag, content_type, last_modified, created_at, deleted_at
	      FROM sys_s3_objects WHERE project_id = ? AND deleted_at IS NULL`
	args := []any{projectID}
	if prefix != "" {
		q += " AND object_key LIKE ?"
		args = append(args, prefix+"%")
	}
	q += " ORDER BY object_key ASC LIMIT ?"
	args = append(args, int64(limit))
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]S3Object, 0)
	for rows.Next() {
		var o S3Object
		var lastMod sql.NullTime
		var deleted sql.NullTime
		if err := rows.Scan(&o.ID, &o.ProjectID, &o.Key, &o.Size, &o.ETag, &o.ContentType, &lastMod, &o.CreatedAt, &deleted); err != nil {
			return nil, err
		}
		if lastMod.Valid {
			o.LastModified = lastMod.Time
		}
		if deleted.Valid {
			t := deleted.Time
			o.DeletedAt = &t
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// GetS3Object 按项目内相对 key 取未删除对象。
func (s *Store) GetS3Object(ctx context.Context, projectID, key string) (S3Object, error) {
	if s == nil || s.db == nil {
		return S3Object{}, ErrUnavailable
	}
	var o S3Object
	var lastMod sql.NullTime
	var deleted sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, object_key, size, etag, content_type, last_modified, created_at, deleted_at
		 FROM sys_s3_objects WHERE project_id = ? AND object_key = ? AND deleted_at IS NULL`,
		projectID, key).Scan(&o.ID, &o.ProjectID, &o.Key, &o.Size, &o.ETag, &o.ContentType, &lastMod, &o.CreatedAt, &deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return S3Object{}, sql.ErrNoRows
	}
	if err != nil {
		return S3Object{}, err
	}
	if lastMod.Valid {
		o.LastModified = lastMod.Time
	}
	if deleted.Valid {
		t := deleted.Time
		o.DeletedAt = &t
	}
	return o, nil
}

// RefreshS3Index 用平面 A List 对账并更新表。
func (s *Store) RefreshS3Index(ctx context.Context, projectID string, listed []objectstore.FileObject) (inserted, removed int, err error) {
	if s == nil || s.db == nil {
		return 0, 0, ErrUnavailable
	}
	runID := uuid.NewString()
	started := time.Now().UTC()
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO sys_s3_sync_runs(id, project_id, status, listed, inserted, removed, error, started_at, finished_at)
		 VALUES(?, ?, 'running', ?, 0, 0, '', ?, NULL)`,
		runID, projectID, int64(len(listed)), started)

	live := map[string]objectstore.FileObject{}
	for _, o := range listed {
		key := o.Key
		if i := strings.Index(key, "/"); i >= 0 && strings.HasPrefix(key, projectID+"/") {
			key = key[len(projectID)+1:]
		}
		live[key] = o
	}

	existing, err := s.ListS3Objects(ctx, projectID, "", 10000)
	if err != nil {
		return 0, 0, err
	}
	have := map[string]S3Object{}
	for _, o := range existing {
		have[o.Key] = o
	}
	for key, fo := range live {
		if _, ok := have[key]; !ok {
			if err := s.UpsertS3Object(ctx, projectID, key, fo.Size, "", "", fo.LastModified); err != nil {
				return inserted, removed, err
			}
			inserted++
		} else {
			_ = s.UpsertS3Object(ctx, projectID, key, fo.Size, "", "", fo.LastModified)
		}
	}
	now := time.Now().UTC()
	for key := range have {
		if _, ok := live[key]; !ok {
			_, _ = s.db.ExecContext(ctx,
				`UPDATE sys_s3_objects SET deleted_at = ? WHERE project_id = ? AND object_key = ? AND deleted_at IS NULL`,
				now, projectID, key)
			removed++
		}
	}
	_, _ = s.db.ExecContext(ctx,
		`UPDATE sys_s3_sync_runs SET status='ok', listed=?, inserted=?, removed=?, finished_at=? WHERE id=?`,
		int64(len(listed)), int64(inserted), int64(removed), time.Now().UTC(), runID)
	s.notifyWrite(ctx)
	return inserted, removed, nil
}
