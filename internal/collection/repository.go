// 文件说明：repository.go — internal/collection 中 Repository、NewRepository 等符号的实现
//
// 用法
//
//	NewRepository(db *sql.DB, notify func(context.Context)) *Repository
//	(r *Repository) Get(ctx context.Context, projectID, name string) (*Collection ...
//	(r *Repository) Put(ctx context.Context, schema Collection) error
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/collection/repository.go
//	         │ 依赖
//	         ▼
//	[下游] internal/utils/sqlval
//	[同包] fieldtypes.go, model.go
//
// 主要符号
//
//	type Repository struct {…}
//	NewRepository(db *sql.DB, notify func(context.Context)) *Repository
//	(r *Repository) Get(ctx context.Context, projectID, name string) (*Collection ...
//	(r *Repository) Put(ctx context.Context, schema Collection) error
//	(r *Repository) Delete(ctx context.Context, projectID, name string) error
//	(r *Repository) Invalidate(projectID, name string)
//	... 另有 4 个顶层符号

package collection

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/linkxzhou/SimpleBase/internal/utils/sqlval"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Repository struct {
	db     *sql.DB
	notify func(context.Context)
	mu     sync.Mutex
	cache  map[string]cacheEntry
	now    func() time.Time
}

type cacheEntry struct {
	schema  *Collection
	expires time.Time
}

func NewRepository(db *sql.DB, notify func(context.Context)) *Repository {
	return &Repository{db: db, notify: notify, cache: map[string]cacheEntry{}, now: time.Now}
}

func (r *Repository) Get(ctx context.Context, projectID, name string) (*Collection, error) {
	key := projectID + "|" + name
	r.mu.Lock()
	if entry, ok := r.cache[key]; ok && r.now().Before(entry.expires) {
		r.mu.Unlock()
		return cloneSchema(entry.schema), nil
	}
	r.mu.Unlock()
	if r.db == nil {
		return nil, ErrUnavailable
	}
	var schema Collection
	var mode, status string
	err := r.db.QueryRowContext(ctx, `SELECT id, project_id, name, schema_mode, view_query, status, created_at, updated_at FROM sys_collections WHERE project_id = ? AND name = ? AND status = 'active'`, projectID, name).Scan(&schema.ID, &schema.ProjectID, &schema.Name, &mode, &schema.ViewQuery, &status, &schema.CreatedAt, &schema.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, unavailableError(err)
	}
	schema.SchemaMode, schema.Status = mode, status
	rows, err := r.db.QueryContext(ctx, `SELECT name, type, required, unique_flag, default_json, options_json, hidden, readonly, order_index FROM sys_collection_fields WHERE project_id = ? AND collection = ? ORDER BY order_index, name`, projectID, name)
	if err != nil {
		return nil, unavailableError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var f Field
		var required, unique, hidden, readonly int
		var defaultJSON, optionsJSON string
		if err := rows.Scan(&f.Name, &f.Type, &required, &unique, &defaultJSON, &optionsJSON, &hidden, &readonly, &f.OrderIndex); err != nil {
			return nil, err
		}
		f.Required, f.Unique, f.Hidden, f.Readonly = required != 0, unique != 0, hidden != 0, readonly != 0
		if defaultJSON != "" {
			if err := json.Unmarshal([]byte(defaultJSON), &f.Default); err != nil {
				return nil, err
			}
		}
		if optionsJSON != "" {
			if err := json.Unmarshal([]byte(optionsJSON), &f.Options); err != nil {
				return nil, err
			}
		}
		schema.Fields = append(schema.Fields, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.cache[key] = cacheEntry{schema: cloneSchema(&schema), expires: r.now().Add(30 * time.Second)}
	r.mu.Unlock()
	return &schema, nil
}

func (r *Repository) Put(ctx context.Context, schema Collection) error {
	if err := ValidateSchema(schema); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailableError(err)
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	if schema.ID == "" {
		schema.ID = uuid.NewString()
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM sys_collections WHERE project_id = ? AND name = ?`, schema.ProjectID, schema.Name); err != nil {
		return err
	}
	if schema.SchemaMode == "" {
		schema.SchemaMode = "json"
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO sys_collections(id, project_id, name, schema_mode, view_query, status, created_at, updated_at) VALUES(?, ?, ?, ?, ?, 'active', ?, ?)`, schema.ID, schema.ProjectID, schema.Name, schema.SchemaMode, strings.TrimSpace(schema.ViewQuery), now, now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM sys_collection_fields WHERE project_id = ? AND collection = ?`, schema.ProjectID, schema.Name); err != nil {
		return err
	}
	for i, f := range schema.Fields {
		defaultJSON := ""
		if f.Default != nil {
			body, _ := json.Marshal(f.Default)
			defaultJSON = string(body)
		}
		options, _ := json.Marshal(f.Options)
		if _, err = tx.ExecContext(ctx, `INSERT INTO sys_collection_fields(id, project_id, collection, name, type, required, unique_flag, default_json, options_json, hidden, readonly, order_index, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, uuid.NewString(), schema.ProjectID, schema.Name, f.Name, f.Type, sqlval.BoolInt(f.Required), sqlval.BoolInt(f.Unique), defaultJSON, string(options), sqlval.BoolInt(f.Hidden), sqlval.BoolInt(f.Readonly), i, now, now); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	r.Invalidate(schema.ProjectID, schema.Name)
	if r.notify != nil {
		r.notify(ctx)
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, projectID, name string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailableError(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM sys_collection_fields WHERE project_id = ? AND collection = ?`, projectID, name); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sys_collections WHERE project_id = ? AND name = ?`, projectID, name); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	r.Invalidate(projectID, name)
	if r.notify != nil {
		r.notify(ctx)
	}
	return nil
}

func (r *Repository) Invalidate(projectID, name string) {
	r.mu.Lock()
	delete(r.cache, projectID+"|"+name)
	r.mu.Unlock()
}

func (r *Repository) List(ctx context.Context, projectID string) ([]Collection, error) {
	if r.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := r.db.QueryContext(ctx, `SELECT name, schema_mode FROM sys_collections WHERE project_id = ? AND status = 'active'`, projectID)
	if err != nil {
		return nil, unavailableError(err)
	}
	defer rows.Close()
	out := []Collection{}
	for rows.Next() {
		var item Collection
		if err := rows.Scan(&item.Name, &item.SchemaMode); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func cloneSchema(in *Collection) *Collection {
	out := *in
	out.Fields = append([]Field(nil), in.Fields...)
	return &out
}

func unavailableError(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "does not exist") || strings.Contains(message, "no such table") {
		return ErrUnavailable
	}
	return err
}
