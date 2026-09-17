package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// sqlRepository 基于 database/sql 访问系统 DuckLake（sys_* 表）。
// 不依赖 PRIMARY KEY / 序列；唯一性由应用层检查。测试可用同一 SQL 跑在 SQLite 上。
type sqlRepository struct {
	db         *sql.DB
	afterWrite func(context.Context)
}

// NewSQLRepository 构造 Repository。db 必须已应用系统迁移。
func NewSQLRepository(db *sql.DB, afterWrite ...func(context.Context)) Repository {
	r := &sqlRepository{db: db}
	if len(afterWrite) > 0 {
		r.afterWrite = afterWrite[0]
	}
	return r
}

func (r *sqlRepository) touch(ctx context.Context) {
	if r.afterWrite != nil {
		r.afterWrite(ctx)
	}
}

func (r *sqlRepository) CreateTenant(ctx context.Context, t Tenant) error {
	var existing string
	err := r.db.QueryRowContext(ctx, `SELECT id FROM sys_tenants WHERE id = ?`, t.ID).Scan(&existing)
	if err == nil {
		return fmt.Errorf("%w: tenant %s", ErrAlreadyExists, t.ID)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO sys_tenants(id, name, created_at) VALUES(?, ?, ?)`,
		t.ID, t.Name, t.CreatedAt.UTC())
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: tenant %s", ErrAlreadyExists, t.ID)
		}
		return err
	}
	r.touch(ctx)
	return nil
}

func (r *sqlRepository) CreateProject(ctx context.Context, p Project) error {
	var existing string
	err := r.db.QueryRowContext(ctx, `SELECT id FROM sys_projects WHERE id = ?`, p.ID).Scan(&existing)
	if err == nil {
		return fmt.Errorf("%w: project %s", ErrAlreadyExists, p.ID)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	err = r.db.QueryRowContext(ctx,
		`SELECT id FROM sys_projects WHERE tenant_id = ? AND name = ?`, p.TenantID, p.Name).Scan(&existing)
	if err == nil {
		return fmt.Errorf("%w: project name %q in tenant %s", ErrAlreadyExists, p.Name, p.TenantID)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO sys_projects(id, tenant_id, name, created_at) VALUES(?, ?, ?, ?)`,
		p.ID, p.TenantID, p.Name, p.CreatedAt.UTC())
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: project name %q in tenant %s", ErrAlreadyExists, p.Name, p.TenantID)
		}
		return err
	}
	r.touch(ctx)
	return nil
}

func (r *sqlRepository) ListProjectsByTenant(ctx context.Context, tenantID string) ([]Project, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, tenant_id, name, created_at FROM sys_projects WHERE tenant_id = ? ORDER BY created_at ASC, id ASC`,
		tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Project, 0)
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.TenantID, &p.Name, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *sqlRepository) CreateDatabase(ctx context.Context, d Database) error {
	if d.Kind == "" {
		d.Kind = DatabaseKindUser
	}
	var existing string
	err := r.db.QueryRowContext(ctx, `SELECT id FROM sys_databases WHERE id = ?`, d.ID).Scan(&existing)
	if err == nil {
		return fmt.Errorf("%w: database %s", ErrAlreadyExists, d.ID)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if d.Kind != DatabaseKindSystem {
		err = r.db.QueryRowContext(ctx,
			`SELECT id FROM sys_databases WHERE project_id = ? AND name = ? AND deleted_at IS NULL`,
			d.ProjectID, d.Name).Scan(&existing)
		if err == nil {
			return fmt.Errorf("%w: database name %q in project %s", ErrAlreadyExists, d.Name, d.ProjectID)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO sys_databases(id, tenant_id, project_id, name, kind, status, storage_prefix, format_version, deleted_at, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.TenantID, d.ProjectID, d.Name, d.Kind, string(d.Status), d.StoragePrefix,
		int64(d.FormatVersion), nullTime(d.DeletedAt), d.CreatedAt.UTC(), d.UpdatedAt.UTC())
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: database name %q in project %s", ErrAlreadyExists, d.Name, d.ProjectID)
		}
		return err
	}
	r.touch(ctx)
	return nil
}

func (r *sqlRepository) GetDatabase(ctx context.Context, projectID, databaseID string) (Database, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, project_id, name, kind, status, storage_prefix, format_version, deleted_at, created_at, updated_at
		 FROM sys_databases WHERE id = ? AND project_id = ?`,
		databaseID, projectID)
	d, err := scanDatabase(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Database{}, fmt.Errorf("%w: database %s in project %s", ErrNotFound, databaseID, projectID)
		}
		return Database{}, err
	}
	return d, nil
}

func (r *sqlRepository) ListDatabases(ctx context.Context, projectID string, page Page) ([]Database, string, error) {
	limit := page.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, tenant_id, project_id, name, kind, status, storage_prefix, format_version, deleted_at, created_at, updated_at
		 FROM sys_databases WHERE project_id = ? AND deleted_at IS NULL AND kind = ?
		 ORDER BY created_at ASC, id ASC LIMIT ?`,
		projectID, DatabaseKindUser, int64(limit+1))
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	out := make([]Database, 0, limit)
	for rows.Next() {
		d, err := scanDatabase(rows)
		if err != nil {
			return nil, "", err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	nextCursor := ""
	if len(out) > limit {
		nextCursor = out[limit-1].ID
		out = out[:limit]
	}
	return out, nextCursor, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanDatabase(s scanner) (Database, error) {
	var d Database
	var status string
	var formatVersion int64
	var deletedAt sql.NullTime
	err := s.Scan(
		&d.ID, &d.TenantID, &d.ProjectID, &d.Name, &d.Kind, &status, &d.StoragePrefix,
		&formatVersion, &deletedAt, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return Database{}, err
	}
	if d.Kind == "" {
		d.Kind = DatabaseKindUser
	}
	d.Status = DatabaseStatus(status)
	d.FormatVersion = int(formatVersion)
	if deletedAt.Valid {
		t := deletedAt.Time
		d.DeletedAt = &t
	}
	return d, nil
}

func (r *sqlRepository) TransitionDatabase(ctx context.Context, id string, from []DatabaseStatus, to DatabaseStatus, at time.Time) (Database, error) {
	if len(from) == 0 {
		return Database{}, fmt.Errorf("catalog: transition requires non-empty from states")
	}
	var current string
	err := r.db.QueryRowContext(ctx, `SELECT status FROM sys_databases WHERE id = ?`, id).Scan(&current)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Database{}, &StateTransitionError{From: from, To: to}
		}
		return Database{}, err
	}
	ok := false
	for _, f := range from {
		if string(f) == current {
			ok = true
			break
		}
	}
	if !ok {
		return Database{}, &StateTransitionError{From: from, To: to}
	}

	placeholders := make([]string, len(from))
	args := make([]any, 0, len(from)+3)
	args = append(args, string(to), at.UTC(), id)
	for i, f := range from {
		placeholders[i] = "?"
		args = append(args, string(f))
	}
	query := fmt.Sprintf(
		`UPDATE sys_databases SET status = ?, updated_at = ? WHERE id = ? AND status IN (%s)`,
		strings.Join(placeholders, ","))
	if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
		return Database{}, err
	}
	r.touch(ctx)
	row := r.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, project_id, name, kind, status, storage_prefix, format_version, deleted_at, created_at, updated_at
		 FROM sys_databases WHERE id = ?`, id)
	return scanDatabase(row)
}

func (r *sqlRepository) MarkDeleted(ctx context.Context, id string, at time.Time) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE sys_databases SET status = ?, deleted_at = ?, updated_at = ? WHERE id = ?`,
		string(DatabaseDeleted), at.UTC(), at.UTC(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var existing string
		if err := r.db.QueryRowContext(ctx, `SELECT id FROM sys_databases WHERE id = ?`, id).Scan(&existing); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: database %s", ErrNotFound, id)
			}
			return err
		}
	}
	r.touch(ctx)
	return nil
}

func (r *sqlRepository) UpsertProviderConfig(ctx context.Context, cfg LLMProviderConfig) error {
	var existing string
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM sys_llm_provider_configs WHERE project_id = ? AND provider = ?`,
		cfg.ProjectID, cfg.Provider).Scan(&existing)
	enabled := int64(0)
	if cfg.Enabled {
		enabled = 1
	}
	if err == nil {
		_, err = r.db.ExecContext(ctx,
			`UPDATE sys_llm_provider_configs SET credential_ref=?, enabled=?, allowed_models_json=?, default_model=?, updated_at=?
			 WHERE project_id=? AND provider=?`,
			cfg.CredentialRef, enabled, cfg.AllowedModelsJSON, cfg.DefaultModel, cfg.UpdatedAt.UTC(),
			cfg.ProjectID, cfg.Provider)
		if err != nil {
			return err
		}
		r.touch(ctx)
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO sys_llm_provider_configs(id, project_id, provider, credential_ref, enabled, allowed_models_json, default_model, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cfg.ID, cfg.ProjectID, cfg.Provider, cfg.CredentialRef,
		enabled, cfg.AllowedModelsJSON, cfg.DefaultModel,
		cfg.CreatedAt.UTC(), cfg.UpdatedAt.UTC())
	if err != nil {
		return err
	}
	r.touch(ctx)
	return nil
}

func (r *sqlRepository) ListEnabledProviders(ctx context.Context, projectID string) ([]LLMProviderConfig, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, project_id, provider, credential_ref, enabled, allowed_models_json, default_model, created_at, updated_at
		 FROM sys_llm_provider_configs WHERE project_id = ? AND enabled = 1`,
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LLMProviderConfig, 0)
	for rows.Next() {
		c, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func scanProvider(s scanner) (LLMProviderConfig, error) {
	var c LLMProviderConfig
	var enabled int64
	if err := s.Scan(&c.ID, &c.ProjectID, &c.Provider, &c.CredentialRef,
		&enabled, &c.AllowedModelsJSON, &c.DefaultModel, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return LLMProviderConfig{}, err
	}
	c.Enabled = enabled != 0
	return c, nil
}

func (r *sqlRepository) AppendUsage(ctx context.Context, events []UsageEvent) error {
	if len(events) == 0 {
		return nil
	}
	for _, e := range events {
		if _, err := r.db.ExecContext(ctx,
			`INSERT INTO sys_usage_events(id, project_id, kind, provider, model, input_tokens, output_tokens, cost_micros, request_id, occurred_at)
			 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			e.ID, e.ProjectID, e.Kind, e.Provider, e.Model,
			e.InputTokens, e.OutputTokens, e.CostMicros, e.RequestID, e.OccurredAt.UTC()); err != nil {
			return err
		}
	}
	r.touch(ctx)
	return nil
}

func (r *sqlRepository) AppendOperation(ctx context.Context, op Operation) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO sys_operations(id, database_id, project_id, principal_id, kind, request_id, status, created_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		op.ID, op.DatabaseID, op.ProjectID, op.PrincipalID, op.Kind, op.RequestID, op.Status, op.CreatedAt.UTC())
	if err != nil {
		return err
	}
	r.touch(ctx)
	return nil
}

func (r *sqlRepository) ProjectBelongsToTenant(ctx context.Context, projectID, tenantID string) (bool, error) {
	var count int64
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM sys_projects WHERE id = ? AND tenant_id = ?`,
		projectID, tenantID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *sqlRepository) GetProjectTenant(ctx context.Context, projectID string) (string, error) {
	var tenantID string
	err := r.db.QueryRowContext(ctx,
		`SELECT tenant_id FROM sys_projects WHERE id = ?`, projectID).Scan(&tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return tenantID, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "constraint failed: UNIQUE") ||
		strings.Contains(msg, "duplicate")
}

func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC()
}

func (r *sqlRepository) GetLLMProviders(ctx context.Context, projectID string) (LLMProviders, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, project_id, provider, credential_ref, enabled, allowed_models_json, default_model, created_at, updated_at
		 FROM sys_llm_provider_configs WHERE project_id = ? AND enabled = 1`, projectID)
	if err != nil {
		return LLMProviders{}, err
	}
	defer rows.Close()
	var out LLMProviders
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return LLMProviders{}, err
		}
		if out.Default == "" {
			out.Default = p.Provider
		}
		out.Providers = append(out.Providers, p)
	}
	return out, rows.Err()
}

func (r *sqlRepository) SetLLMProviders(ctx context.Context, projectID string, providers LLMProviders) error {
	for _, p := range providers.Providers {
		p.ProjectID = projectID
		p.Enabled = true
		if err := r.UpsertProviderConfig(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

func (r *sqlRepository) GetQuota(ctx context.Context, projectID string) (ProjectQuota, error) {
	var q ProjectQuota
	var period, maxDB, maxStorage, maxReq, maxTok int64
	err := r.db.QueryRowContext(ctx,
		`SELECT project_id, max_databases, max_storage_bytes, max_llm_requests, max_llm_tokens, period_seconds, updated_at
		 FROM sys_project_quotas WHERE project_id = ?`, projectID).Scan(
		&q.ProjectID, &maxDB, &maxStorage, &maxReq, &maxTok, &period, &q.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProjectQuota{ProjectID: projectID, PeriodSeconds: 3600}, nil
		}
		return ProjectQuota{}, err
	}
	q.MaxDatabases = int(maxDB)
	q.MaxStorageBytes = maxStorage
	q.MaxLLMRequests = maxReq
	q.MaxLLMTokens = maxTok
	q.PeriodSeconds = int(period)
	return q, nil
}

func (r *sqlRepository) UpsertQuota(ctx context.Context, quota ProjectQuota) error {
	now := time.Now().UTC()
	if quota.UpdatedAt.IsZero() {
		quota.UpdatedAt = now
	}
	if quota.PeriodSeconds <= 0 {
		quota.PeriodSeconds = 3600
	}
	var existing string
	err := r.db.QueryRowContext(ctx,
		`SELECT project_id FROM sys_project_quotas WHERE project_id = ?`, quota.ProjectID).Scan(&existing)
	if err == nil {
		_, err = r.db.ExecContext(ctx,
			`UPDATE sys_project_quotas SET max_databases=?, max_storage_bytes=?, max_llm_requests=?, max_llm_tokens=?, period_seconds=?, updated_at=?
			 WHERE project_id=?`,
			int64(quota.MaxDatabases), quota.MaxStorageBytes, quota.MaxLLMRequests, quota.MaxLLMTokens,
			int64(quota.PeriodSeconds), quota.UpdatedAt.UTC(), quota.ProjectID)
		if err != nil {
			return err
		}
		r.touch(ctx)
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO sys_project_quotas(project_id, max_databases, max_storage_bytes, max_llm_requests, max_llm_tokens, period_seconds, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?)`,
		quota.ProjectID, int64(quota.MaxDatabases), quota.MaxStorageBytes,
		quota.MaxLLMRequests, quota.MaxLLMTokens, int64(quota.PeriodSeconds), quota.UpdatedAt.UTC())
	if err != nil {
		return err
	}
	r.touch(ctx)
	return nil
}

func (r *sqlRepository) SumUsageSince(ctx context.Context, projectID string, since time.Time) (UsageSummary, error) {
	var sum UsageSummary
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(input_tokens + output_tokens), 0)
		 FROM sys_usage_events
		 WHERE project_id = ? AND kind = 'llm' AND occurred_at >= ?`,
		projectID, since.UTC()).Scan(&sum.LLMRequests, &sum.LLMTokens)
	if err != nil {
		return UsageSummary{}, err
	}
	sum.DatabaseStorage = 0
	return sum, nil
}

func (r *sqlRepository) ListOperations(ctx context.Context, projectID, databaseID string, limit int) ([]Operation, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id, database_id, project_id, principal_id, kind, request_id, status, created_at
	      FROM sys_operations WHERE 1=1`
	args := []any{}
	if projectID != "" {
		q += " AND project_id = ?"
		args = append(args, projectID)
	}
	if databaseID != "" {
		q += " AND database_id = ?"
		args = append(args, databaseID)
	}
	q += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, int64(limit))
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Operation
	for rows.Next() {
		var op Operation
		if err := rows.Scan(&op.ID, &op.DatabaseID, &op.ProjectID, &op.PrincipalID,
			&op.Kind, &op.RequestID, &op.Status, &op.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, op)
	}
	return out, rows.Err()
}
