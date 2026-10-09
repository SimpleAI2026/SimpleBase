package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/database/sqlguard"
)

var errNoSchemaColumns = errors.New("api: schema table has no columns")

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
		return WriteError(c, err)
	}
	limit, offset, err := parseSchemaPage(c)
	if err != nil {
		return WriteError(c, err)
	}
	tableName := strings.TrimSpace(c.Param("table"))
	if strings.HasPrefix(tableName, "__") {
		return WriteError(c, tableNotFound(c))
	}
	tableName, err = normalizeSchemaIdent(tableName)
	if err != nil {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "表名不合法", RequestIDFromContext(c.Request().Context())))
	}

	ctx, cancel := h.withQueryTimeout(c.Request().Context())
	defer cancel()
	lease, err := h.svc.Acquire(ctx, db, database.ReadOnly)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()

	cols, err := h.loadTableColumns(ctx, c, lease, tableName)
	if err != nil {
		return WriteError(c, err)
	}
	system := catalog.IsSystemDatabase(db)
	if system {
		for i := range cols {
			if isSystemSensitiveColumn(cols[i].Name) {
				cols[i].Sensitive = true
			}
		}
	}

	countSQL := "SELECT COUNT(*) FROM " + quoteIdentifier(tableName)
	if err := sqlguard.Validate(countSQL, sqlguard.ReadOnly); err != nil {
		return WriteError(c, err)
	}
	countResult, err := lease.Query(ctx, database.Statement{SQL: countSQL}, 1)
	if err != nil {
		return WriteError(c, err)
	}
	var total int64
	if len(countResult.Rows) > 0 && len(countResult.Rows[0]) > 0 {
		total = toInt64(countResult.Rows[0][0])
	}

	pageSQL, err := schemaRowsSelectSQL(tableName, cols, system)
	if err != nil {
		return WriteError(c, err)
	}
	if err := sqlguard.Validate(pageSQL, sqlguard.ReadOnly); err != nil {
		return WriteError(c, err)
	}
	pageResult, err := lease.Query(ctx, database.Statement{
		SQL:  pageSQL,
		Args: []any{limit, offset},
	}, limit)
	if err != nil {
		return WriteError(c, err)
	}
	rows, err := database.SerializeRows(pageResult.Rows)
	if err != nil {
		return WriteError(c, err)
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
	limit, ok := parseQueryLimit(c, schemaRowsDefaultLimit, schemaRowsMaxLimit)
	if !ok {
		return 0, 0, NewAPIError(http.StatusBadRequest, "invalid_request", "invalid limit or offset", RequestIDFromContext(c.Request().Context()))
	}
	raw := strings.TrimSpace(c.QueryParam("offset"))
	if raw == "" {
		return limit, 0, nil
	}
	offset, err := strconv.Atoi(raw)
	if err != nil || offset < 0 || offset > schemaRowsMaxOffset {
		return 0, 0, NewAPIError(http.StatusBadRequest, "invalid_request", "invalid limit or offset", RequestIDFromContext(c.Request().Context()))
	}
	return limit, offset, nil
}

func tableNotFound(c echo.Context) error {
	return NewAPIError(http.StatusNotFound, "table_not_found", "table not found", RequestIDFromContext(c.Request().Context()))
}

func (h *SchemaHandler) loadTableColumns(ctx context.Context, c echo.Context, lease SQLLease, table string) ([]schemaColumnJSON, error) {
	const lookup = `SELECT c.column_name, c.data_type, c.is_nullable
		FROM information_schema.columns c
		JOIN information_schema.tables t
		  ON c.table_catalog = t.table_catalog
		 AND c.table_schema = t.table_schema
		 AND c.table_name = t.table_name
		WHERE t.table_catalog = current_database()
		  AND t.table_schema = current_schema()
		  AND t.table_type = 'BASE TABLE'
		  AND c.table_name = ?
		ORDER BY c.ordinal_position`
	if err := sqlguard.Validate(lookup, sqlguard.ReadOnly); err != nil {
		return nil, err
	}
	result, err := lease.Query(ctx, database.Statement{SQL: lookup, Args: []any{table}}, schemaColumnScanLimit)
	if err != nil {
		return nil, err
	}
	cols := make([]schemaColumnJSON, 0, len(result.Rows))
	for _, row := range result.Rows {
		if len(row) < 3 {
			continue
		}
		name := cellString(row[0])
		if _, err := normalizeSchemaIdent(name); err != nil || name == "" {
			continue
		}
		cols = append(cols, schemaColumnJSON{
			Name:     name,
			Type:     cellString(row[1]),
			Nullable: strings.EqualFold(cellString(row[2]), "YES"),
		})
	}
	if len(cols) == 0 {
		return nil, tableNotFound(c)
	}
	return cols, nil
}

func schemaRowsSelectSQL(table string, cols []schemaColumnJSON, system bool) (string, error) {
	if len(cols) == 0 {
		return "", errNoSchemaColumns
	}
	proj := make([]string, 0, len(cols))
	for _, col := range cols {
		quoted := quoteIdentifier(col.Name)
		if system && isSystemSensitiveColumn(col.Name) {
			proj = append(proj, "NULL AS "+quoted)
			continue
		}
		proj = append(proj, quoted)
	}
	sqlText := "SELECT " + strings.Join(proj, ", ") + " FROM " + quoteIdentifier(table)
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
		order := quoteIdentifier(actual) + " DESC"
		if id, ok := byLower["id"]; ok {
			order += ", " + quoteIdentifier(id) + " ASC"
		}
		return order
	}
	if id, ok := byLower["id"]; ok {
		return quoteIdentifier(id) + " ASC"
	}
	if len(cols) == 0 {
		return ""
	}
	return quoteIdentifier(cols[0].Name) + " ASC"
}

func finishSchemaTables(tables []schemaTableJSON, system bool) []schemaTableJSON {
	out := make([]schemaTableJSON, 0, len(tables))
	for _, table := range tables {
		if table.Name == "" || strings.HasPrefix(table.Name, "__") {
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
