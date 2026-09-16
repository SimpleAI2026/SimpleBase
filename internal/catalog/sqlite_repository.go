package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// sqliteRepository 是基于 database/sql 的 Repository 实现。
// 它使用独立系统数据库（Turso/libSQL 或测试用 sqlite3），与用户 database 隔离。
type sqliteRepository struct {
	db *sql.DB
}

// NewSQLiteRepository 构造 Repository。db 必须已应用 migrations。
func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepository{db: db}
}

func (r *sqliteRepository) CreateTenant(ctx context.Context, t Tenant) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO tenants(id, name, created_at) VALUES(?, ?, ?)`,
		t.ID, t.Name, t.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: tenant %s", ErrAlreadyExists, t.ID)
		}
		return err
	}
	return nil
}

func (r *sqliteRepository) CreateProject(ctx context.Context, p Project) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO projects(id, tenant_id, name, created_at) VALUES(?, ?, ?, ?)`,
		p.ID, p.TenantID, p.Name, p.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: project name %q in tenant %s", ErrAlreadyExists, p.Name, p.TenantID)
		}
		return err
	}
	return nil
}

func (r *sqliteRepository) ListProjectsByTenant(ctx context.Context, tenantID string) ([]Project, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, tenant_id, name, created_at FROM projects WHERE tenant_id = ? ORDER BY created_at ASC, id ASC`,
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

func (r *sqliteRepository) CreateDatabase(ctx context.Context, d Database) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO databases(id, tenant_id, project_id, name, status, storage_prefix, format_version, deleted_at, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.TenantID, d.ProjectID, d.Name, string(d.Status), d.StoragePrefix,
		d.FormatVersion, d.DeletedAt, d.CreatedAt, d.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: database name %q in project %s", ErrAlreadyExists, d.Name, d.ProjectID)
		}
		return err
	}
	return nil
}

func (r *sqliteRepository) GetDatabase(ctx context.Context, projectID, databaseID string) (Database, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, project_id, name, status, storage_prefix, format_version, deleted_at, created_at, updated_at
		 FROM databases WHERE id = ? AND project_id = ?`,
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

func (r *sqliteRepository) ListDatabases(ctx context.Context, projectID string, page Page) ([]Database, string, error) {
	limit := page.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, tenant_id, project_id, name, status, storage_prefix, format_version, deleted_at, created_at, updated_at
		 FROM databases WHERE project_id = ? AND deleted_at IS NULL
		 ORDER BY created_at ASC, id ASC LIMIT ?`,
		projectID, limit+1)
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

// scanner 抽象 *sql.Row 和 *sql.Rows 的 Scan 方法。
type scanner interface {
	Scan(dest ...any) error
}

func scanDatabase(s scanner) (Database, error) {
	var d Database
	var status string
	var deletedAt sql.NullTime
	err := s.Scan(
		&d.ID, &d.TenantID, &d.ProjectID, &d.Name, &status, &d.StoragePrefix,
		&d.FormatVersion, &deletedAt, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return Database{}, err
	}
	d.Status = DatabaseStatus(status)
	if deletedAt.Valid {
		t := deletedAt.Time
		d.DeletedAt = &t
	}
	return d, nil
}

// TransitionDatabase 使用 UPDATE ... WHERE status IN (...) 并检查 affected rows。
// 防止并发错序：若当前状态不在 from 中，affected=0，返回 *StateTransitionError。
func (r *sqliteRepository) TransitionDatabase(ctx context.Context, id string, from []DatabaseStatus, to DatabaseStatus, at time.Time) (Database, error) {
	if len(from) == 0 {
		return Database{}, fmt.Errorf("catalog: transition requires non-empty from states")
	}
	placeholders := make([]string, len(from))
	// SQL 占位符顺序为: SET status=?, updated_at=? WHERE id=? AND status IN (?,?,...)
	// args 必须严格按该顺序排列: to, at, id, from...
	args := make([]any, 0, len(from)+3)
	args = append(args, string(to), at, id)
	for i, f := range from {
		placeholders[i] = "?"
		args = append(args, string(f))
	}

	query := fmt.Sprintf(
		`UPDATE databases SET status = ?, updated_at = ? WHERE id = ? AND status IN (%s)`,
		strings.Join(placeholders, ","))
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return Database{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return Database{}, err
	}
	if affected == 0 {
		// 当前状态不在 from 中，或 id 不存在
		return Database{}, &StateTransitionError{From: from, To: to}
	}
	// 返回更新后的记录（无 projectID，由 service 层持有）
	row := r.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, project_id, name, status, storage_prefix, format_version, deleted_at, created_at, updated_at
		 FROM databases WHERE id = ?`, id)
	return scanDatabase(row)
}

func (r *sqliteRepository) MarkDeleted(ctx context.Context, id string, at time.Time) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE databases SET status = ?, deleted_at = ?, updated_at = ? WHERE id = ?`,
		string(DatabaseDeleted), at, at, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("%w: database %s", ErrNotFound, id)
	}
	return nil
}

func (r *sqliteRepository) UpsertProviderConfig(ctx context.Context, cfg LLMProviderConfig) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO llm_provider_configs(id, project_id, provider, credential_ref, enabled, allowed_models_json, default_model, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(project_id, provider) DO UPDATE SET
		   credential_ref=excluded.credential_ref,
		   enabled=excluded.enabled,
		   allowed_models_json=excluded.allowed_models_json,
		   default_model=excluded.default_model,
		   updated_at=excluded.updated_at`,
		cfg.ID, cfg.ProjectID, cfg.Provider, cfg.CredentialRef,
		boolToInt(cfg.Enabled), cfg.AllowedModelsJSON, cfg.DefaultModel,
		cfg.CreatedAt, cfg.UpdatedAt)
	return err
}

func (r *sqliteRepository) ListEnabledProviders(ctx context.Context, projectID string) ([]LLMProviderConfig, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, project_id, provider, credential_ref, enabled, allowed_models_json, default_model, created_at, updated_at
		 FROM llm_provider_configs WHERE project_id = ? AND enabled = 1`,
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LLMProviderConfig, 0)
	for rows.Next() {
		var c LLMProviderConfig
		var enabled int
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.Provider, &c.CredentialRef,
			&enabled, &c.AllowedModelsJSON, &c.DefaultModel, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Enabled = enabled != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *sqliteRepository) AppendUsage(ctx context.Context, events []UsageEvent) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO usage_events(id, project_id, kind, provider, model, input_tokens, output_tokens, cost_micros, request_id, occurred_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range events {
		if _, err := stmt.ExecContext(ctx, e.ID, e.ProjectID, e.Kind, e.Provider, e.Model,
			e.InputTokens, e.OutputTokens, e.CostMicros, e.RequestID, e.OccurredAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *sqliteRepository) AppendOperation(ctx context.Context, op Operation) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO operations(id, database_id, project_id, principal_id, kind, request_id, status, created_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		op.ID, op.DatabaseID, op.ProjectID, op.PrincipalID, op.Kind, op.RequestID, op.Status, op.CreatedAt)
	return err
}

func (r *sqliteRepository) ProjectBelongsToTenant(ctx context.Context, projectID, tenantID string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM projects WHERE id = ? AND tenant_id = ?`,
		projectID, tenantID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetProjectTenant 返回 project 所属的 tenant ID。
func (r *sqliteRepository) GetProjectTenant(ctx context.Context, projectID string) (string, error) {
	var tenantID string
	err := r.db.QueryRowContext(ctx,
		`SELECT tenant_id FROM projects WHERE id = ?`, projectID).Scan(&tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return tenantID, nil
}

// isUniqueViolation 检测 SQLite 唯一约束冲突。
// libSQL/sqlite 驱动返回的错误消息含 "UNIQUE constraint failed"。
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "constraint failed: UNIQUE")
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// scanJob 从 sql.Rows 扫描一行到 Job。
func scanJob(s interface {
	Scan(dest ...any) error
}, j *Job) error {
	var attempt, maxAttempts int
	var runAfter, createdAt, updatedAt time.Time
	var payloadJSON, lastError, status, typ string
	err := s.Scan(&j.ID, &j.OperationID, &j.DatabaseID, &j.ProjectID,
		&typ, &status, &attempt, &maxAttempts, &runAfter,
		&payloadJSON, &lastError, &createdAt, &updatedAt)
	if err != nil {
		return err
	}
	j.Type = JobType(typ)
	j.Status = JobStatus(status)
	j.Attempt = attempt
	j.MaxAttempts = maxAttempts
	j.RunAfter = runAfter
	j.PayloadJSON = payloadJSON
	j.LastError = lastError
	j.CreatedAt = createdAt
	j.UpdatedAt = updatedAt
	return nil
}

func (r *sqliteRepository) Enqueue(ctx context.Context, job Job) error {
	if job.ID == "" {
		return errors.New("catalog: job id required")
	}
	if job.Status == "" {
		job.Status = JobStatusPending
	}
	if job.MaxAttempts <= 0 {
		job.MaxAttempts = 5
	}
	now := time.Now().UTC()
	if job.CreatedAt.IsZero() {
		job.CreatedAt = now
	}
	if job.UpdatedAt.IsZero() {
		job.UpdatedAt = now
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO jobs(id, operation_id, database_id, project_id, type, status, attempt, max_attempts,
			run_after, payload_json, last_error, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.OperationID, job.DatabaseID, job.ProjectID,
		string(job.Type), string(job.Status), job.Attempt, job.MaxAttempts,
		job.RunAfter, job.PayloadJSON, job.LastError, job.CreatedAt, job.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: job %s", ErrAlreadyExists, job.ID)
		}
		return err
	}
	return nil
}

func (r *sqliteRepository) Claim(ctx context.Context, workerID string, now time.Time) (Job, error) {
	// 单实例下使用事务 + 条件 UPDATE 原子领取。
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var j Job
	var typ, status string
	err = tx.QueryRowContext(ctx,
		`SELECT id, operation_id, database_id, project_id, type, status, attempt, max_attempts,
			run_after, payload_json, last_error, created_at, updated_at
		 FROM jobs
		 WHERE status = ? AND run_after <= ?
		 ORDER BY run_after ASC
		 LIMIT 1`,
		string(JobStatusPending), now).Scan(
		&j.ID, &j.OperationID, &j.DatabaseID, &j.ProjectID,
		&typ, &status, &j.Attempt, &j.MaxAttempts,
		&j.RunAfter, &j.PayloadJSON, &j.LastError, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Job{}, ErrNotFound
		}
		return Job{}, err
	}
	j.Type = JobType(typ)
	j.Status = JobStatus(status)

	_, err = tx.ExecContext(ctx,
		`UPDATE jobs SET status = ?, attempt = attempt + 1, updated_at = ? WHERE id = ? AND status = ?`,
		string(JobStatusRunning), now, j.ID, string(JobStatusPending))
	if err != nil {
		return Job{}, err
	}
	if err := tx.Commit(); err != nil {
		return Job{}, err
	}
	j.Attempt++
	j.Status = JobStatusRunning
	j.UpdatedAt = now
	_ = workerID
	return j, nil
}

func (r *sqliteRepository) Complete(ctx context.Context, id string) error {
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx,
		`UPDATE jobs SET status = ?, updated_at = ? WHERE id = ? AND status = ?`,
		string(JobStatusCompleted), now, id, string(JobStatusRunning))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqliteRepository) Retry(ctx context.Context, id string, lastErr string, next time.Time) error {
	now := time.Now().UTC()
	var attempt, maxAttempts int
	err := r.db.QueryRowContext(ctx,
		`SELECT attempt, max_attempts FROM jobs WHERE id = ?`, id).Scan(&attempt, &maxAttempts)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	newStatus := string(JobStatusPending)
	if attempt >= maxAttempts {
		newStatus = string(JobStatusDeadLetter)
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE jobs SET status = ?, last_error = ?, run_after = ?, updated_at = ? WHERE id = ? AND status = ?`,
		newStatus, lastErr, next, now, id, string(JobStatusRunning))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqliteRepository) GetJob(ctx context.Context, id string) (Job, error) {
	var j Job
	var typ, status string
	err := r.db.QueryRowContext(ctx,
		`SELECT id, operation_id, database_id, project_id, type, status, attempt, max_attempts,
			run_after, payload_json, last_error, created_at, updated_at
		 FROM jobs WHERE id = ?`, id).Scan(
		&j.ID, &j.OperationID, &j.DatabaseID, &j.ProjectID,
		&typ, &status, &j.Attempt, &j.MaxAttempts,
		&j.RunAfter, &j.PayloadJSON, &j.LastError, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Job{}, ErrNotFound
		}
		return Job{}, err
	}
	j.Type = JobType(typ)
	j.Status = JobStatus(status)
	return j, nil
}

func (r *sqliteRepository) ListJobs(ctx context.Context, databaseID string, limit int) ([]Job, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, operation_id, database_id, project_id, type, status, attempt, max_attempts,
			run_after, payload_json, last_error, created_at, updated_at
		 FROM jobs WHERE database_id = ?
		 ORDER BY created_at DESC LIMIT ?`, databaseID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		var j Job
		if err := scanJob(rows, &j); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// LLM 供应商配置实现（plan8.md）。

func (r *sqliteRepository) GetLLMProviders(ctx context.Context, projectID string) (LLMProviders, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, project_id, provider, credential_ref, enabled, allowed_models_json, default_model, created_at, updated_at
		 FROM llm_provider_configs WHERE project_id = ? AND enabled = 1`, projectID)
	if err != nil {
		return LLMProviders{}, err
	}
	defer rows.Close()
	var out LLMProviders
	for rows.Next() {
		var p LLMProviderConfig
		var enabled int
		if err := rows.Scan(&p.ID, &p.ProjectID, &p.Provider, &p.CredentialRef,
			&enabled, &p.AllowedModelsJSON, &p.DefaultModel, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return LLMProviders{}, err
		}
		p.Enabled = enabled == 1
		if out.Default == "" {
			out.Default = p.Provider
		}
		out.Providers = append(out.Providers, p)
	}
	return out, rows.Err()
}

func (r *sqliteRepository) SetLLMProviders(ctx context.Context, projectID string, providers LLMProviders) error {
	// 委托给 UpsertProviderConfig 逐条写入，保持与现有 schema 一致。
	for _, p := range providers.Providers {
		p.ProjectID = projectID
		p.Enabled = true
		if p.Provider == providers.Default {
			p.Enabled = true
		}
		if err := r.UpsertProviderConfig(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

// QuotaStore 实现（plan9.md）。

func (r *sqliteRepository) GetQuota(ctx context.Context, projectID string) (ProjectQuota, error) {
	var q ProjectQuota
	var period int
	err := r.db.QueryRowContext(ctx,
		`SELECT project_id, max_databases, max_storage_bytes, max_llm_requests, max_llm_tokens, period_seconds, updated_at
		 FROM project_quotas WHERE project_id = ?`, projectID).Scan(
		&q.ProjectID, &q.MaxDatabases, &q.MaxStorageBytes, &q.MaxLLMRequests,
		&q.MaxLLMTokens, &period, &q.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// 无配额记录：返回零值（不限）。
			return ProjectQuota{ProjectID: projectID, PeriodSeconds: 3600}, nil
		}
		return ProjectQuota{}, err
	}
	q.PeriodSeconds = period
	return q, nil
}

func (r *sqliteRepository) UpsertQuota(ctx context.Context, quota ProjectQuota) error {
	now := time.Now().UTC()
	if quota.UpdatedAt.IsZero() {
		quota.UpdatedAt = now
	}
	if quota.PeriodSeconds <= 0 {
		quota.PeriodSeconds = 3600
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO project_quotas(project_id, max_databases, max_storage_bytes, max_llm_requests, max_llm_tokens, period_seconds, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(project_id) DO UPDATE SET
		   max_databases=excluded.max_databases,
		   max_storage_bytes=excluded.max_storage_bytes,
		   max_llm_requests=excluded.max_llm_requests,
		   max_llm_tokens=excluded.max_llm_tokens,
		   period_seconds=excluded.period_seconds,
		   updated_at=excluded.updated_at`,
		quota.ProjectID, quota.MaxDatabases, quota.MaxStorageBytes,
		quota.MaxLLMRequests, quota.MaxLLMTokens, quota.PeriodSeconds, quota.UpdatedAt)
	return err
}

func (r *sqliteRepository) SumUsageSince(ctx context.Context, projectID string, since time.Time) (UsageSummary, error) {	var sum UsageSummary
	// LLM 请求数与 token 数从 usage_events 汇总。
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(input_tokens + output_tokens), 0)
		 FROM usage_events
		 WHERE project_id = ? AND kind = 'llm' AND occurred_at >= ?`,
		projectID, since).Scan(&sum.LLMRequests, &sum.LLMTokens)
	if err != nil {
		return UsageSummary{}, err
	}
	// 数据库存储字节数：从 databases 表汇总（简化：使用 storage_bytes 字段若存在）。
	// 当前 schema 无此字段，返回 0；由 cache.Manager.Usage 补充实时数据。
	sum.DatabaseStorage = 0
	return sum, nil
}

func (r *sqliteRepository) ListOperations(ctx context.Context, projectID, databaseID string, limit int) ([]Operation, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id, database_id, project_id, principal_id, kind, request_id, status, created_at
	      FROM operations WHERE 1=1`
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
	args = append(args, limit)
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
