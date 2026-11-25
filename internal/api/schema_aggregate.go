// 文件说明：schema_aggregate.go — SQL 表聚合接口 GET .../schema/tables/:table/aggregate
//
// 与集合聚合（collection_aggregate.go）共用同一套编译（aggregate_env.go），区别只在字段是真实列。
// 只读：ReadOnly lease、sqlguard 校验、扫描上限、超时；系统库可读，凭证列视为不存在。

package api

import (
	"context"
	"errors"
	collectionquery "github.com/linkxzhou/SimpleBase/internal/collection/query"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlguard"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlx"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

// WithAggregateLimits 设置 SQL 表聚合的扫描上限与超时；非正数保持默认。
func (h *SchemaHandler) WithAggregateLimits(timeout time.Duration, scan int) *SchemaHandler {
	h.setAggregateLimits(timeout, scan)
	return h
}

// AggregateTable: GET /v1/projects/:projectID/databases/:databaseID/schema/tables/:table/aggregate
func (h *SchemaHandler) AggregateTable(c echo.Context) error {
	db, err := h.ensureSQL(c, false)
	if err != nil {
		return WriteError(c, err)
	}
	tableName := strings.TrimSpace(c.Param("table"))
	if godatabase.IsInternalTableName(tableName) {
		return WriteError(c, tableNotFound(c))
	}
	tableName, err = normalizeSchemaIdent(tableName)
	if err != nil {
		return WriteError(c, badRequest(c, "表名不合法"))
	}

	timeout, scanMax := h.aggregateLimits()
	ctx, cancel := context.WithTimeout(c.Request().Context(), timeout)
	defer cancel()
	lease, err := h.svc.Acquire(ctx, db, godatabase.ReadOnly)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()

	cols, err := h.loadTableColumns(ctx, db, lease, tableName)
	if err != nil {
		if errors.Is(err, ErrSchemaTableNotFound) {
			return WriteError(c, tableNotFound(c))
		}
		return WriteError(c, err)
	}
	dialect, err := dialectForDatabase(db)
	if err != nil {
		return WriteError(c, err)
	}
	columns := make([]collectionquery.SQLTableColumn, 0, len(cols))
	for _, col := range cols {
		columns = append(columns, collectionquery.SQLTableColumn{Name: col.Name, Kind: collectionquery.SQLColumnKind(col.Type)})
	}
	var sensitive func(string) bool
	if catalog.IsSystemDatabase(db) {
		sensitive = isSystemSensitiveColumn
	}
	env := collectionquery.SQLTableEnv(dialect, columns, sensitive)

	query, err := collectionquery.ParseAggregateEnv(c.QueryParams(), sqlx.Ident(tableName), env)
	if err != nil {
		return validationAPIError(c, err)
	}
	// 拼出的 SQL 全部由固定模板与 sqlx.Ident 引用的真实列构成；仍过一遍只读校验作为兜底。
	// 同口径总量查询（with_total）同样过校验。
	for _, sqlText := range []string{query.CountSQL, query.SQL, query.TotalSQL} {
		if sqlText == "" {
			continue
		}
		if err := sqlguard.Validate(sqlText, sqlguard.ReadOnly); err != nil {
			return WriteError(c, err)
		}
	}

	count, err := lease.Query(ctx, godatabase.Statement{SQL: query.CountSQL, Args: query.CountArgs}, 1)
	if err != nil {
		return WriteError(c, err)
	}
	var scanned int64
	if len(count.Rows) > 0 && len(count.Rows[0]) > 0 {
		scanned = anyInt64(count.Rows[0][0])
	}
	if scanned > int64(scanMax) {
		return scanLimitError(c)
	}

	result, err := lease.Query(ctx, godatabase.Statement{SQL: query.SQL, Args: query.Args}, query.Limit+1)
	if err != nil {
		return WriteError(c, err)
	}
	rows := result.Rows
	truncated := false
	if len(rows) > query.Limit {
		truncated = true
		rows = rows[:query.Limit]
	}
	if query.NeedsSeriesFold && len(query.Columns) > 0 {
		collectionquery.FoldSeries(rows, len(query.Columns)-1)
	}
	serialized, err := godatabase.SerializeRows(rows)
	if err != nil {
		return WriteError(c, err)
	}
	if serialized == nil {
		serialized = [][]any{}
	}
	total, err := aggregateTotal(ctx, lease, query)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, aggregateResponse{
		Columns:    query.Columns,
		Rows:       serialized,
		Total:      total,
		Truncated:  truncated,
		Scanned:    scanned,
		DurationMS: result.Duration.Milliseconds(),
		RequestID:  requestID(c),
	})
}
