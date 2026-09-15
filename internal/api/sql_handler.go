// sql_handler.go 实现 Plan 6 的 SQL API handler。
//
// 三个路由：
//   POST /v1/projects/:projectID/databases/:databaseID/query   (DatabaseRead)
//   POST /v1/projects/:projectID/databases/:databaseID/execute (DatabaseWrite)
//   POST /v1/projects/:projectID/databases/:databaseID/batch   (DatabaseWrite)
//
// 所有 handler 遵循同一流程：
//   1. 从 context 取已校验的 ProjectContext 与 Principal。
//   2. 解析 path 参数 databaseID；bind body 后显式校验。
//   3. 通过 catalog 校验数据库归属与状态。
//   4. sqlguard.Validate 按 intent 校验 SQL。
//   5. Acquire 租约（query 默认 ReadOnly，execute/batch 先检查 writable 再 ReadWrite）。
//   6. 执行 SQL；序列化结果。
//   7. 审计只记录关键字、SQL SHA-256、耗时、行数、错误码；不记录原 SQL 与参数。
package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/database/sqlguard"
)

// SQLService 抽象 SQL handler 所需的 catalog + registry 能力。
// 与 DatabaseService 分离，便于独立测试与未来扩展。
type SQLService interface {
	// GetDatabase 返回指定数据库的 catalog 记录（已校验 project 归属）。
	GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error)
	// Acquire 获取数据库访问租约。
	Acquire(ctx context.Context, db catalog.Database, mode database.AccessMode) (SQLLease, error)
}

// SQLLease 是 SQL handler 使用的租约抽象，封装 Handle 的查询能力。
// registry.Lease.Handle 自动满足此接口的方法集，但为避免暴露 registry 类型，
// 这里通过适配器在 adapter.go 中桥接。
type SQLLease interface {
	Release()
	Query(ctx context.Context, stmt database.Statement, maxRows int) (database.QueryResult, error)
	Execute(ctx context.Context, stmt database.Statement) (database.QueryResult, error)
	Batch(ctx context.Context, stmts []database.Statement, transactional bool) ([]database.QueryResult, error)
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
	mu  sync.Mutex

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
		return database.ErrQueryConcurrency
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

	req, err := decodeQueryRequest(c, h.limits)
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

	if err := h.acquireSem(ctx); err != nil {
		return WriteError(c, err)
	}
	defer h.releaseSem()

	lease, err := h.svc.Acquire(ctx, db, database.ReadOnly)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()

	maxRows := effectiveMaxRows(req.MaxRows, h.limits.MaxQueryRows)
	result, err := lease.Query(ctx, database.Statement{SQL: req.SQL, Args: req.Args}, maxRows)
	if err != nil {
		return WriteError(c, err)
	}

	serializedRows, err := database.SerializeRows(result.Rows)
	if err != nil {
		return WriteError(c, err)
	}

	rid := RequestIDFromContext(c.Request().Context())
	return c.JSON(http.StatusOK, QueryResponse{
		Columns:    result.Columns,
		Rows:       serializedRows,
		RowCount:   len(serializedRows),
		DurationMS: result.Duration.Milliseconds(),
		RequestID:  rid,
	})
}

// Execute: POST /v1/projects/:projectID/databases/:databaseID/execute
func (h *SQLHandler) Execute(c echo.Context) error {
	if !h.writable {
		return WriteError(c, database.ErrWriterUnavailable)
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

	req, err := decodeExecuteRequest(c, h.limits)
	if err != nil {
		return WriteError(c, err)
	}

	if err := sqlguard.Validate(req.SQL, sqlguard.WriteAllowed); err != nil {
		return WriteError(c, err)
	}

	databaseID := c.Param("databaseID")
	db, err := h.svc.GetDatabase(ctx, principal, project.ID, databaseID)
	if err != nil {
		return WriteError(c, err)
	}

	if err := h.acquireSem(ctx); err != nil {
		return WriteError(c, err)
	}
	defer h.releaseSem()

	lease, err := h.svc.Acquire(ctx, db, database.ReadWrite)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()

	result, err := lease.Execute(ctx, database.Statement{SQL: req.SQL, Args: req.Args})
	if err != nil {
		return WriteError(c, err)
	}

	rid := RequestIDFromContext(c.Request().Context())
	var lastID *int64
	if result.LastInsertID != 0 {
		v := result.LastInsertID
		lastID = &v
	}
	dur := "committed_local"
	if h.DurabilityFor != nil {
		dur = h.DurabilityFor(databaseID)
	}
	return c.JSON(http.StatusOK, ExecuteResponse{
		RowsAffected: result.RowsAffected,
		LastInsertID: lastID,
		Durability:   dur,
		DurationMS:   result.Duration.Milliseconds(),
		RequestID:    rid,
	})
}

// Batch: POST /v1/projects/:projectID/databases/:databaseID/batch
func (h *SQLHandler) Batch(c echo.Context) error {
	if !h.writable {
		return WriteError(c, database.ErrWriterUnavailable)
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

	req, err := decodeBatchRequest(c, h.limits)
	if err != nil {
		return WriteError(c, err)
	}

	// 逐条校验 SQL
	stmts := make([]database.Statement, 0, len(req.Statements))
	for _, s := range req.Statements {
		if err := sqlguard.Validate(s.SQL, sqlguard.WriteAllowed); err != nil {
			return WriteError(c, err)
		}
		stmts = append(stmts, database.Statement{SQL: s.SQL, Args: s.Args})
	}

	databaseID := c.Param("databaseID")
	db, err := h.svc.GetDatabase(ctx, principal, project.ID, databaseID)
	if err != nil {
		return WriteError(c, err)
	}

	if err := h.acquireSem(ctx); err != nil {
		return WriteError(c, err)
	}
	defer h.releaseSem()

	lease, err := h.svc.Acquire(ctx, db, database.ReadWrite)
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
			dur := "committed_local"
			if h.DurabilityFor != nil {
				dur = h.DurabilityFor(databaseID)
			}
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
		dur := "committed_local"
		if h.DurabilityFor != nil {
			dur = h.DurabilityFor(databaseID)
		}
		return c.JSON(http.StatusOK, BatchResponse{
			Results:    toBatchResultsTransactional(results),
			Durability: dur,
			RequestID:  rid,
		})
	}

	// Non-transactional: 逐条执行，失败项带 error 但不影响已提交
	results, err := lease.Batch(ctx, stmts, false)
	items := toBatchResultsNonTransactional(results, err)
	dur := "committed_local"
	if h.DurabilityFor != nil {
		dur = h.DurabilityFor(databaseID)
	}
	return c.JSON(http.StatusOK, BatchResponse{
		Results:    items,
		Durability: dur,
		RequestID:  rid,
	})
}

// decodeQueryRequest 解码并校验 QueryRequest。
func decodeQueryRequest(c echo.Context, limits SQLLimits) (*QueryRequest, error) {
	dec := json.NewDecoder(c.Request().Body)
	dec.DisallowUnknownFields()
	var req QueryRequest
	if err := dec.Decode(&req); err != nil {
		return nil, NewAPIError(http.StatusBadRequest, "invalid_request", "malformed JSON body", RequestIDFromContext(c.Request().Context()))
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
	dec := json.NewDecoder(c.Request().Body)
	dec.DisallowUnknownFields()
	var req ExecuteRequest
	if err := dec.Decode(&req); err != nil {
		return nil, NewAPIError(http.StatusBadRequest, "invalid_request", "malformed JSON body", RequestIDFromContext(c.Request().Context()))
	}
	if err := validateSQLStatement(req.SQL, req.Args, limits); err != nil {
		return nil, err
	}
	return &req, nil
}

// decodeBatchRequest 解码并校验 BatchRequest。
func decodeBatchRequest(c echo.Context, limits SQLLimits) (*BatchRequest, error) {
	dec := json.NewDecoder(c.Request().Body)
	dec.DisallowUnknownFields()
	var req BatchRequest
	if err := dec.Decode(&req); err != nil {
		return nil, NewAPIError(http.StatusBadRequest, "invalid_request", "malformed JSON body", RequestIDFromContext(c.Request().Context()))
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
// database.Batch 返回的错误格式为 "database: batch statement N failed..."
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
func toBatchResultsTransactional(results []database.QueryResult) []BatchResultItem {
	items := make([]BatchResultItem, len(results))
	for i, r := range results {
		var lastID *int64
		if r.LastInsertID != 0 {
			v := r.LastInsertID
			lastID = &v
		}
		items[i] = BatchResultItem{
			Index:        i,
			RowsAffected: r.RowsAffected,
			LastInsertID: lastID,
			DurationMS:   r.Duration.Milliseconds(),
		}
	}
	return items
}

// toBatchResultsNonTransactional 将非事务 batch 结果转为响应项。
// 若有错误，错误位置之后的项不会出现在 results 中，需补全为失败项。
func toBatchResultsNonTransactional(results []database.QueryResult, batchErr error) []BatchResultItem {
	items := make([]BatchResultItem, 0, len(results)+1)
	for i, r := range results {
		var lastID *int64
		if r.LastInsertID != 0 {
			v := r.LastInsertID
			lastID = &v
		}
		items = append(items, BatchResultItem{
			Index:        i,
			RowsAffected: r.RowsAffected,
			LastInsertID: lastID,
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

// sqlSHA256 返回 SQL 的 SHA-256 摘要（用于审计日志，不记录原 SQL）。
func sqlSHA256(sql string) string {
	h := sha256.Sum256([]byte(sql))
	return hex.EncodeToString(h[:])
}
