// 文件说明：sql_handler.go — Plan 6 的 SQL API handler
//
// 用法
//
//	NewSQLHandler(svc SQLService, limits SQLLimits, writable bool) *SQLHandler
//	(h *SQLHandler) Query(c echo.Context) error  // Query: POST /v1/projects/:projectID/databases/:databaseID/query
//	(h *SQLHandler) Execute(c echo.Context) error  // Execute: POST /v1/projects/:projectID/databases/:databaseID/execute
//	同包调用：Query() → adapter.go, collection_meta_handler.go 等；SQLLease → adapter.go, collection_meta_handler.go 等；Execute() → adapter.go, collection_meta_handler.go 等
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/sql_handler.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase, godatabase/sqlguard, internal/auth, internal/catalog
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] Query() ← adapter.go, collection_meta_handler.go, collection_transfer.go
//
// 主要符号
//
//	type SQLService interface {…}
//	type SQLLease interface {…}
//	type SQLLimits struct {…}
//	type SQLHandler struct {…}
//	NewSQLHandler(svc SQLService, limits SQLLimits, writable bool) *SQLHandler
//	(h *SQLHandler) Query(c echo.Context) error
//	... 另有 15 个顶层符号

package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlguard"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

// SQLService 抽象 SQL handler 所需的 catalog + registry 能力。
// 与 DatabaseService 分离，便于独立测试与未来扩展。
type SQLService interface {
	// GetDatabase 返回指定数据库的 catalog 记录（已校验 project 归属）。
	GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error)
	// Acquire 获取数据库访问租约。
	Acquire(ctx context.Context, db catalog.Database, mode godatabase.AccessMode) (SQLLease, error)
}

// SQLLease 是 SQL handler 使用的租约抽象，封装 Handle 的查询能力。
// registry.Lease.Handle 自动满足此接口的方法集，但为避免暴露 registry 类型，
// 这里通过适配器在 adapter.go 中桥接。
//
// Raw/NotifyWrite 为 KV 服务（key-value-ducklake-plan §5.1 方案 A）预留：
// KV 仓库层需要 *sql.DB 自管事务（Statement 抽象无法表达读-改-写分支），
// 写事务提交后经 NotifyWrite 触发与 Execute/Batch 相同的 onWrite 链路
// （CatalogSyncer.MarkDirty → S3 同步）。
type SQLLease interface {
	Release()
	Query(ctx context.Context, stmt godatabase.Statement, maxRows int) (godatabase.QueryResult, error)
	Execute(ctx context.Context, stmt godatabase.Statement) (godatabase.QueryResult, error)
	Batch(ctx context.Context, stmts []godatabase.Statement, transactional bool) ([]godatabase.QueryResult, error)
	// Raw 返回底层 *sql.DB（KV 仓库自管事务用；调用方不得关闭该连接）。
	Raw() *sql.DB
	// NotifyWrite 通知一次外部写提交（触发 onWrite 同步链路）。
	NotifyWrite(ctx context.Context)
}

// SQLLimits 控制 SQL handler 的资源上限。由 config.Limits 转换而来。
type SQLLimits struct {
	QueryTimeout       int64 // nanoseconds
	MaxQueryRows       int
	MaxConcurrent      int
	MaxBatchStatements int
	MaxSQLBytes        int
	MaxRequestBytes    int64
}

// SQLHandler 实现 SQL API 的三个路由。
type SQLHandler struct {
	svc      SQLService
	limits   SQLLimits
	writable bool

	// 全局并发 semaphore（nil 表示不限制）
	sem chan struct{}

	// DurabilityFor 返回写响应 durability；nil 时默认 committed_local。
	DurabilityFor func(databaseID string) string
}

// NewSQLHandler 构造 SQL handler。writable 为 false 时写操作返回 503。
// limits.MaxConcurrent <= 0 时不启用并发限制。
func NewSQLHandler(svc SQLService, limits SQLLimits, writable bool) *SQLHandler {
	h := &SQLHandler{svc: svc, limits: limits, writable: writable}
	if limits.MaxConcurrent > 0 {
		h.sem = make(chan struct{}, limits.MaxConcurrent)
	}
	return h
}

// durabilityFor 返回写响应的 durability 标注：无回调时默认 committed_local，
// 回调返回值（含空串）原样返回，不做兜底（planv6.0 §8 B2）。
func (h *SQLHandler) durabilityFor(databaseID string) string {
	if h.DurabilityFor == nil {
		return "committed_local"
	}
	return h.DurabilityFor(databaseID)
}

// writableDatabase 取用户库并拒绝系统库写入（execute/batch 共用的门禁段）。
// 系统库只读：一律拒绝，防止改写实例元数据。
func (h *SQLHandler) writableDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error) {
	db, err := h.svc.GetDatabase(ctx, principal, projectID, databaseID)
	if err != nil {
		return catalog.Database{}, err
	}
	if catalog.IsSystemDatabase(db) {
		return catalog.Database{}, catalog.ErrSystemProtected
	}
	return db, nil
}

// acquireSem 尝试获取并发槽位。失败返回 ErrQueryConcurrency。
func (h *SQLHandler) acquireSem(ctx context.Context) error {
	if h.sem == nil {
		return nil
	}
	select {
	case h.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return godatabase.ErrQueryConcurrency
	}
}

func (h *SQLHandler) releaseSem() {
	if h.sem != nil {
		<-h.sem
	}
}

// Query: POST /v1/projects/:projectID/databases/:databaseID/query
func (h *SQLHandler) Query(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), nanoToDuration(h.limits.QueryTimeout))
	defer cancel()
	project, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	principal, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, auth.ErrMissingCredentials)
	}

	timer := StageTimerFrom(ctx)
	decodeScope := timer.StageScope(StageDecode)
	req, err := decodeQueryRequest(c, h.limits)
	decodeScope.Done()
	if err != nil {
		return WriteError(c, err)
	}

	if err := sqlguard.Validate(req.SQL, sqlguard.ReadOnly); err != nil {
		return WriteError(c, err)
	}

	databaseID := c.Param("databaseID")
	db, err := h.svc.GetDatabase(ctx, principal, project.ID, databaseID)
	if err != nil {
		return WriteError(c, err)
	}
	// 系统库凭证列不允许通过别名或表达式读出。SELECT * 的列名在执行后抹掉。
	if catalog.IsSystemDatabase(db) && systemSQLReferencesSensitive(req.SQL) {
		return WriteError(c, NewAPIError(http.StatusForbidden, "system_column_redacted", "system database credential columns are hidden", RequestIDFromContext(c.Request().Context())))
	}

	semScope := timer.StageScope(StageSemaphore)
	if err := h.acquireSem(ctx); err != nil {
		semScope.Done()
		return WriteError(c, err)
	}
	semScope.Done()
	defer h.releaseSem()

	lease, err := h.svc.Acquire(ctx, db, godatabase.ReadOnly)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()

	maxRows := effectiveMaxRows(req.MaxRows, h.limits.MaxQueryRows)
	result, err := lease.Query(ctx, godatabase.Statement{SQL: req.SQL, Args: req.Args}, maxRows)
	if err != nil {
		return WriteError(c, err)
	}

	serScope := timer.StageScope(StageSerialize)
	serializedRows, err := godatabase.SerializeRows(result.Rows)
	serScope.Done()
	if err != nil {
		return WriteError(c, err)
	}

	rid := RequestIDFromContext(c.Request().Context())
	resp := QueryResponse{
		Columns:    result.Columns,
		Rows:       serializedRows,
		RowCount:   len(serializedRows),
		DurationMS: result.Duration.Milliseconds(),
		RequestID:  rid,
	}
	if catalog.IsSystemDatabase(db) {
		resp.RedactedColumns = redactSystemResult(resp.Columns, resp.Rows)
	}
	return c.JSON(http.StatusOK, resp)
}

// Execute: POST /v1/projects/:projectID/databases/:databaseID/execute
func (h *SQLHandler) Execute(c echo.Context) error {
	if !h.writable {
		return WriteError(c, godatabase.ErrWriterUnavailable)
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), nanoToDuration(h.limits.QueryTimeout))
	defer cancel()
	project, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	principal, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, auth.ErrMissingCredentials)
	}

	timer := StageTimerFrom(ctx)
	decodeScope := timer.StageScope(StageDecode)
	req, err := decodeExecuteRequest(c, h.limits)
	decodeScope.Done()
	if err != nil {
		return WriteError(c, err)
	}

	if err := sqlguard.Validate(req.SQL, sqlguard.WriteAllowed); err != nil {
		return WriteError(c, err)
	}

	db, err := h.writableDatabase(ctx, principal, project.ID, c.Param("databaseID"))
	if err != nil {
		return WriteError(c, err)
	}
	databaseID := c.Param("databaseID")

	semScope := timer.StageScope(StageSemaphore)
	if err := h.acquireSem(ctx); err != nil {
		semScope.Done()
		return WriteError(c, err)
	}
	semScope.Done()
	defer h.releaseSem()

	lease, err := h.svc.Acquire(ctx, db, godatabase.ReadWrite)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()

	result, err := lease.Execute(ctx, godatabase.Statement{SQL: req.SQL, Args: req.Args})
	if err != nil {
		return WriteError(c, err)
	}

	rid := RequestIDFromContext(c.Request().Context())
	dur := h.durabilityFor(databaseID)
	return c.JSON(http.StatusOK, ExecuteResponse{
		RowsAffected: result.RowsAffected,
		Durability:   dur,
		DurationMS:   result.Duration.Milliseconds(),
		RequestID:    rid,
	})
}

// Batch: POST /v1/projects/:projectID/databases/:databaseID/batch
func (h *SQLHandler) Batch(c echo.Context) error {
	if !h.writable {
		return WriteError(c, godatabase.ErrWriterUnavailable)
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), nanoToDuration(h.limits.QueryTimeout))
	defer cancel()
	project, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	principal, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, auth.ErrMissingCredentials)
	}

	timer := StageTimerFrom(ctx)
	decodeScope := timer.StageScope(StageDecode)
	req, err := decodeBatchRequest(c, h.limits)
	decodeScope.Done()
	if err != nil {
		return WriteError(c, err)
	}

	// 逐条校验 SQL
	stmts := make([]godatabase.Statement, 0, len(req.Statements))
	for _, s := range req.Statements {
		if err := sqlguard.Validate(s.SQL, sqlguard.WriteAllowed); err != nil {
			return WriteError(c, err)
		}
		stmts = append(stmts, godatabase.Statement{SQL: s.SQL, Args: s.Args})
	}

	db, err := h.writableDatabase(ctx, principal, project.ID, c.Param("databaseID"))
	if err != nil {
		return WriteError(c, err)
	}
	databaseID := c.Param("databaseID")

	semScope := timer.StageScope(StageSemaphore)
	if err := h.acquireSem(ctx); err != nil {
		semScope.Done()
		return WriteError(c, err)
	}
	semScope.Done()
	defer h.releaseSem()

	lease, err := h.svc.Acquire(ctx, db, godatabase.ReadWrite)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()

	rid := RequestIDFromContext(c.Request().Context())

	if req.Transactional {
		results, err := lease.Batch(ctx, stmts, true)
		if err != nil {
			// 事务回滚：返回整体错误，标明失败位置
			failedIdx := extractBatchIndex(err)
			dur := h.durabilityFor(databaseID)
			return c.JSON(http.StatusOK, BatchResponse{
				Results:    toBatchResultsTransactional(results),
				Durability: dur,
				DurationMS: 0,
				RequestID:  rid,
				Error: &BatchError{
					FailedIndex: failedIdx,
					Code:        "batch_rolled_back",
					Message:     err.Error(),
				},
			})
		}
		dur := h.durabilityFor(databaseID)
		return c.JSON(http.StatusOK, BatchResponse{
			Results:    toBatchResultsTransactional(results),
			Durability: dur,
			RequestID:  rid,
		})
	}

	// Non-transactional: 逐条执行，失败项带 error 但不影响已提交
	results, err := lease.Batch(ctx, stmts, false)
	items := toBatchResultsNonTransactional(results, err)
	dur := h.durabilityFor(databaseID)
	return c.JSON(http.StatusOK, BatchResponse{
		Results:    items,
		Durability: dur,
		RequestID:  rid,
	})
}

// decodeQueryRequest 解码并校验 QueryRequest。
func decodeQueryRequest(c echo.Context, limits SQLLimits) (*QueryRequest, error) {
	var req QueryRequest
	if err := decodeJSONBody(c, &req, true); err != nil {
		return nil, err
	}
	if err := validateSQLStatement(req.SQL, req.Args, limits); err != nil {
		return nil, err
	}
	if req.MaxRows < 0 {
		return nil, NewAPIError(http.StatusBadRequest, "invalid_request", "max_rows must be non-negative", RequestIDFromContext(c.Request().Context()))
	}
	if req.MaxRows > limits.MaxQueryRows {
		return nil, NewAPIError(http.StatusBadRequest, "invalid_request", fmt.Sprintf("max_rows exceeds limit %d", limits.MaxQueryRows), RequestIDFromContext(c.Request().Context()))
	}
	return &req, nil
}

// decodeExecuteRequest 解码并校验 ExecuteRequest。
func decodeExecuteRequest(c echo.Context, limits SQLLimits) (*ExecuteRequest, error) {
	var req ExecuteRequest
	if err := decodeJSONBody(c, &req, true); err != nil {
		return nil, err
	}
	if err := validateSQLStatement(req.SQL, req.Args, limits); err != nil {
		return nil, err
	}
	return &req, nil
}

// decodeBatchRequest 解码并校验 BatchRequest。
func decodeBatchRequest(c echo.Context, limits SQLLimits) (*BatchRequest, error) {
	var req BatchRequest
	if err := decodeJSONBody(c, &req, true); err != nil {
		return nil, err
	}
	if len(req.Statements) == 0 {
		return nil, NewAPIError(http.StatusBadRequest, "invalid_request", "statements must not be empty", RequestIDFromContext(c.Request().Context()))
	}
	if len(req.Statements) > limits.MaxBatchStatements {
		return nil, NewAPIError(http.StatusBadRequest, "invalid_request", fmt.Sprintf("statements count exceeds limit %d", limits.MaxBatchStatements), RequestIDFromContext(c.Request().Context()))
	}
	for _, s := range req.Statements {
		if err := validateSQLStatement(s.SQL, s.Args, limits); err != nil {
			return nil, err
		}
	}
	return &req, nil
}

// validateSQLStatement 校验单条 SQL 的基础约束（长度、args 数量）。
func validateSQLStatement(sql string, args []any, limits SQLLimits) error {
	if strings.TrimSpace(sql) == "" {
		return NewAPIError(http.StatusBadRequest, "invalid_request", "sql must not be empty", "")
	}
	if len(sql) > limits.MaxSQLBytes {
		return NewAPIError(http.StatusBadRequest, "invalid_request", fmt.Sprintf("sql exceeds max bytes %d", limits.MaxSQLBytes), "")
	}
	if len(args) > 1000 {
		return NewAPIError(http.StatusBadRequest, "invalid_request", "too many args", "")
	}
	return nil
}

// effectiveMaxRows 取请求与配置的较小值；请求为 0 表示用配置上限。
func effectiveMaxRows(reqMaxRows, cfgMax int) int {
	if reqMaxRows <= 0 {
		return cfgMax
	}
	if reqMaxRows > cfgMax {
		return cfgMax
	}
	return reqMaxRows
}

// extractBatchIndex 从 batch 错误信息中提取失败语句索引。
// godatabase.Batch 返回的错误格式为 "database: batch statement N failed..."
func extractBatchIndex(err error) int {
	if err == nil {
		return -1
	}
	// 简单解析：找不到则返回 -1
	msg := err.Error()
	prefix := "batch statement "
	idx := strings.Index(msg, prefix)
	if idx < 0 {
		return -1
	}
	rest := msg[idx+len(prefix):]
	n := 0
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// toBatchResultsTransactional 将事务 batch 的成功结果转为响应项。
func toBatchResultsTransactional(results []godatabase.QueryResult) []BatchResultItem {
	items := make([]BatchResultItem, len(results))
	for i, r := range results {
		items[i] = BatchResultItem{
			Index:        i,
			RowsAffected: r.RowsAffected,
			DurationMS:   r.Duration.Milliseconds(),
		}
	}
	return items
}

// toBatchResultsNonTransactional 将非事务 batch 结果转为响应项。
// 若有错误，错误位置之后的项不会出现在 results 中，需补全为失败项。
func toBatchResultsNonTransactional(results []godatabase.QueryResult, batchErr error) []BatchResultItem {
	items := make([]BatchResultItem, 0, len(results)+1)
	for i, r := range results {
		items = append(items, BatchResultItem{
			Index:        i,
			RowsAffected: r.RowsAffected,
			DurationMS:   r.Duration.Milliseconds(),
		})
	}
	if batchErr != nil {
		items = append(items, BatchResultItem{
			Index:        len(results),
			ErrorCode:    "batch_statement_failed",
			ErrorMessage: batchErr.Error(),
		})
	}
	return items
}

// nanoToDuration 将纳秒转为 time.Duration。避免在本文件 import time。
func nanoToDuration(ns int64) time.Duration {
	return time.Duration(ns)
}
