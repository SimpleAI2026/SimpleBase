package datahttp

import (
	"context"
	"errors"
	"github.com/linkxzhou/SimpleBase/internal/collection/query"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlguard"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlx"
	"github.com/linkxzhou/SimpleBase/internal/catalog"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

var errNoSchemaColumns = errors.New("api: schema table has no columns")

// ErrSchemaTableNotFound 表示目标表不存在（handler 统一映射为 table_not_found）。
var ErrSchemaTableNotFound = errors.New("api: schema table not found")

const (
	schemaRowsDefaultLimit = 50
	schemaRowsMaxLimit     = 200
	schemaRowsMaxOffset    = 100000
	schemaColumnScanLimit  = 2000
)

// ListTableRows: GET /v1/projects/:projectID/databases/:databaseID/schema/tables/:table/rows
func (h *SchemaHandler) ListTableRows(c echo.Context) error {
	db, err := h.ensureSQL(c, false)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	limit, offset, err := parseSchemaPage(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	tableName := strings.TrimSpace(c.Param("table"))
	if godatabase.IsInternalTableName(tableName) {
		return httpkit.WriteError(c, tableNotFound(c))
	}
	tableName, err = normalizeSchemaIdent(tableName)
	if err != nil {
		return httpkit.WriteError(c, httpkit.BadRequest(c, "表名不合法"))
	}

	ctx, cancel := h.withQueryTimeout(c.Request().Context())
	defer cancel()
	lease, err := h.svc.Acquire(ctx, db, godatabase.ReadOnly)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	defer lease.Release()

	cols, err := h.loadTableColumns(ctx, db, lease, tableName)
	if err != nil {
		if errors.Is(err, ErrSchemaTableNotFound) {
			return httpkit.WriteError(c, tableNotFound(c))
		}
		return httpkit.WriteError(c, err)
	}
	dialect, err := dialectForDatabase(db)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	system := catalog.IsSystemDatabase(db)
	if system {
		for i := range cols {
			if isSystemSensitiveColumn(cols[i].Name) {
				cols[i].Sensitive = true
			}
		}
	}

	// 可选 filter：与聚合接口同一套语法与列白名单（真实列、敏感列视为不存在），
	// 数据看板的明细表据此跟随全局条件。无 filter 时行为与改造前完全一致。
	var whereSQL string
	var whereArgs []any
	if raw := strings.TrimSpace(c.QueryParam("filter")); raw != "" {
		columns := make([]query.SQLTableColumn, 0, len(cols))
		for _, col := range cols {
			columns = append(columns, query.SQLTableColumn{Name: col.Name, Kind: query.SQLColumnKind(col.Type)})
		}
		var sensitive func(string) bool
		if system {
			sensitive = isSystemSensitiveColumn
		}
		whereSQL, whereArgs, err = query.SQLTableEnv(dialect, columns, sensitive).Filter(raw)
		if err != nil {
			return validationAPIError(c, err)
		}
	}

	countSQL := dialect.CountRowsSQL(tableName)
	if whereSQL != "" {
		countSQL += " WHERE " + whereSQL
	}
	if err := sqlguard.Validate(countSQL, sqlguard.ReadOnly); err != nil {
		return httpkit.WriteError(c, err)
	}
	countResult, err := lease.Query(ctx, godatabase.Statement{SQL: countSQL, Args: append([]any(nil), whereArgs...)}, 1)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	var total int64
	if len(countResult.Rows) > 0 && len(countResult.Rows[0]) > 0 {
		total = toInt64(countResult.Rows[0][0])
	}

	pageSQL, err := schemaRowsSelectSQLWhere(tableName, cols, system, whereSQL)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if err := sqlguard.Validate(pageSQL, sqlguard.ReadOnly); err != nil {
		return httpkit.WriteError(c, err)
	}
	pageResult, err := lease.Query(ctx, godatabase.Statement{
		SQL:  pageSQL,
		Args: append(append([]any(nil), whereArgs...), limit, offset),
	}, limit)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	rows, err := godatabase.SerializeRows(pageResult.Rows)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if rows == nil {
		rows = [][]any{}
	}
	return c.JSON(http.StatusOK, schemaRowsResponse{
		Table:   tableName,
		Columns: cols,
		Rows:    rows,
		Limit:   limit,
		Offset:  offset,
		Total:   total,
	})
}

type schemaRowsResponse struct {
	Table   string             `json:"table"`
	Columns []schemaColumnJSON `json:"columns"`
	Rows    [][]any            `json:"rows"`
	Limit   int                `json:"limit"`
	Offset  int                `json:"offset"`
	Total   int64              `json:"total"`
}

func (h *SchemaHandler) withQueryTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	d := time.Duration(h.limits.QueryTimeout)
	if d <= 0 {
		d = 5 * time.Second
	}
	return context.WithTimeout(ctx, d)
}

func parseSchemaPage(c echo.Context) (int, int, error) {
	limit, ok := httpkit.ParseQueryLimit(c, schemaRowsDefaultLimit, schemaRowsMaxLimit)
	if !ok {
		return 0, 0, httpkit.BadRequest(c, "invalid limit or offset")
	}
	raw := strings.TrimSpace(c.QueryParam("offset"))
	if raw == "" {
		return limit, 0, nil
	}
	offset, err := strconv.Atoi(raw)
	if err != nil || offset < 0 || offset > schemaRowsMaxOffset {
		return 0, 0, httpkit.BadRequest(c, "invalid limit or offset")
	}
	return limit, offset, nil
}

func tableNotFound(c echo.Context) error {
	return httpkit.NewAPIError(http.StatusNotFound, "table_not_found", "table not found", httpkit.RequestID(c))
}

func (h *SchemaHandler) loadTableColumns(ctx context.Context, db catalog.Database, lease SQLLease, table string) ([]schemaColumnJSON, error) {
	dialect, err := dialectForDatabase(db)
	if err != nil {
		return nil, err
	}
	// 元数据查询是方言里的固定模板，表名已由 normalizeSchemaIdent 校验并经 sqlx.Literal 转义，
	// 不含任何用户 SQL。不能过 sqlguard：SQLite 的模板用 pragma_table_info()，sqlguard 对 PRAGMA
	// 一律拒绝（防用户提交 PRAGMA 语句），会让 SQLite 库的「查看数据」与聚合永远 sql_not_allowed。
	// 写保护由 ReadOnly lease 保证。
	lookup := dialect.TableColumnsSQL(table)
	result, err := lease.Query(ctx, godatabase.Statement{SQL: lookup}, schemaColumnScanLimit)
	if err != nil {
		return nil, err
	}
	cols := make([]schemaColumnJSON, 0, len(result.Rows))
	for _, row := range result.Rows {
		if len(row) < 4 {
			continue
		}
		name := cellString(row[1])
		if _, err := normalizeSchemaIdent(name); err != nil || name == "" {
			continue
		}
		cols = append(cols, schemaColumnJSON{
			Name:     name,
			Type:     cellString(row[2]),
			Nullable: strings.EqualFold(cellString(row[3]), "YES"),
		})
	}
	if len(cols) == 0 {
		return nil, ErrSchemaTableNotFound
	}
	return cols, nil
}

// schemaRowsSelectSQLWhere 在分页查询里插入已编译的 WHERE 片段（占位符参数由调用方按序前置）。
func schemaRowsSelectSQLWhere(table string, cols []schemaColumnJSON, system bool, where string) (string, error) {
	if len(cols) == 0 {
		return "", errNoSchemaColumns
	}
	proj := make([]string, 0, len(cols))
	for _, col := range cols {
		quoted := sqlx.Ident(col.Name)
		if system && isSystemSensitiveColumn(col.Name) {
			proj = append(proj, "NULL AS "+quoted)
			continue
		}
		proj = append(proj, quoted)
	}
	sqlText := "SELECT " + strings.Join(proj, ", ") + " FROM " + sqlx.Ident(table)
	if where != "" {
		sqlText += " WHERE " + where
	}
	if order := schemaRowsOrder(cols); order != "" {
		sqlText += " ORDER BY " + order
	}
	return sqlText + " LIMIT ? OFFSET ?", nil
}

func schemaRowsOrder(cols []schemaColumnJSON) string {
	byLower := make(map[string]string, len(cols))
	for _, col := range cols {
		byLower[strings.ToLower(col.Name)] = col.Name
	}
	for _, name := range []string{"occurred_at", "created_at", "updated_at", "started_at"} {
		actual, ok := byLower[name]
		if !ok {
			continue
		}
		order := sqlx.Ident(actual) + " DESC"
		if id, ok := byLower["id"]; ok {
			order += ", " + sqlx.Ident(id) + " ASC"
		}
		return order
	}
	if id, ok := byLower["id"]; ok {
		return sqlx.Ident(id) + " ASC"
	}
	if len(cols) == 0 {
		return ""
	}
	return sqlx.Ident(cols[0].Name) + " ASC"
}

func finishSchemaTables(tables []schemaTableJSON, system bool) []schemaTableJSON {
	out := make([]schemaTableJSON, 0, len(tables))
	for _, table := range tables {
		if table.Name == "" || godatabase.IsInternalTableName(table.Name) {
			continue
		}
		if system {
			for i := range table.Columns {
				if isSystemSensitiveColumn(table.Columns[i].Name) {
					table.Columns[i].Sensitive = true
				}
			}
		}
		out = append(out, table)
	}
	return out
}
