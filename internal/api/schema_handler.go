package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/database/sqlguard"
)

// SchemaHandler 管理 SQL 数据库的表与列。集合库不能走这些路由。
// 系统库只读：可以列出表和分页读行，DDL 被拒绝。
type SchemaHandler struct {
	svc      DataService
	writable bool
	limits   SQLLimits
}

func NewSchemaHandler(svc DataService, writable bool, limits SQLLimits) *SchemaHandler {
	return &SchemaHandler{svc: svc, writable: writable, limits: limits}
}

type schemaColumnJSON struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Nullable  bool   `json:"nullable"`
	Sensitive bool   `json:"sensitive,omitempty"`
}

type schemaTableJSON struct {
	Name    string             `json:"name"`
	Columns []schemaColumnJSON `json:"columns"`
}

type schemaResponse struct {
	Tables []schemaTableJSON `json:"tables"`
}

type createTableRequest struct {
	Name    string              `json:"name"`
	Columns []schemaColumnInput `json:"columns"`
}

type schemaColumnInput struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable *bool  `json:"nullable,omitempty"`
}

type addColumnRequest struct {
	Table    string `json:"table"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable *bool  `json:"nullable,omitempty"`
}

func columnNullable(v *bool) bool {
	if v == nil {
		return true
	}
	return *v
}

// ListSchema: GET /v1/projects/:projectID/databases/:databaseID/schema
func (h *SchemaHandler) ListSchema(c echo.Context) error {
	db, err := h.ensureSQL(c, false)
	if err != nil {
		return WriteError(c, err)
	}
	lease, err := h.svc.Acquire(c.Request().Context(), db, database.ReadOnly)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()
	result, err := lease.Query(c.Request().Context(), database.Statement{
		SQL: `SELECT c.table_name, c.column_name, c.data_type, c.is_nullable
			FROM information_schema.columns c
			JOIN information_schema.tables t
			  ON c.table_catalog = t.table_catalog
			 AND c.table_schema = t.table_schema
			 AND c.table_name = t.table_name
			WHERE t.table_catalog = current_database()
			  AND t.table_schema = current_schema()
			  AND t.table_type = 'BASE TABLE'
			ORDER BY c.table_name, c.ordinal_position`,
	}, 5000)
	if err != nil {
		return WriteError(c, err)
	}
	tables := finishSchemaTables(groupSchemaRows(result.Rows), catalog.IsSystemDatabase(db))
	return c.JSON(http.StatusOK, schemaResponse{Tables: tables})
}

// CreateTable: POST .../schema/tables
func (h *SchemaHandler) CreateTable(c echo.Context) error {
	db, err := h.ensureSQL(c, true)
	if err != nil {
		return WriteError(c, err)
	}
	var req createTableRequest
	if err := decodeJSONBody(c, &req, true); err != nil {
		return WriteError(c, err)
	}
	cols := make([]schemaColumnSpec, 0, len(req.Columns))
	for _, col := range req.Columns {
		cols = append(cols, schemaColumnSpec{Name: col.Name, Type: col.Type, Nullable: columnNullable(col.Nullable)})
	}
	sqlText, err := buildCreateTableSQL(req.Name, cols)
	if err != nil {
		return WriteError(c, schemaInputError(c, err))
	}
	if err := sqlguard.Validate(sqlText, sqlguard.WriteAllowed); err != nil {
		return WriteError(c, err)
	}
	if err := h.exec(c, db, sqlText); err != nil {
		return WriteError(c, err)
	}
	outCols := make([]schemaColumnJSON, 0, len(cols))
	for _, col := range cols {
		name, _ := normalizeSchemaIdent(col.Name)
		typ, _ := normalizeSchemaType(col.Type)
		outCols = append(outCols, schemaColumnJSON{Name: name, Type: typ, Nullable: col.Nullable})
	}
	name, _ := normalizeSchemaIdent(req.Name)
	return c.JSON(http.StatusCreated, schemaTableJSON{Name: name, Columns: outCols})
}

// AddColumn: POST .../schema/columns
func (h *SchemaHandler) AddColumn(c echo.Context) error {
	db, err := h.ensureSQL(c, true)
	if err != nil {
		return WriteError(c, err)
	}
	var req addColumnRequest
	if err := decodeJSONBody(c, &req, true); err != nil {
		return WriteError(c, err)
	}
	col := schemaColumnSpec{Name: req.Name, Type: req.Type, Nullable: columnNullable(req.Nullable)}
	sqlText, err := buildAddColumnSQL(req.Table, col)
	if err != nil {
		return WriteError(c, schemaInputError(c, err))
	}
	if err := sqlguard.Validate(sqlText, sqlguard.WriteAllowed); err != nil {
		return WriteError(c, err)
	}
	if err := h.exec(c, db, sqlText); err != nil {
		return WriteError(c, err)
	}
	table, _ := normalizeSchemaIdent(req.Table)
	name, _ := normalizeSchemaIdent(req.Name)
	typ, _ := normalizeSchemaType(req.Type)
	return c.JSON(http.StatusCreated, struct {
		Table    string `json:"table"`
		Name     string `json:"name"`
		Type     string `json:"type"`
		Nullable bool   `json:"nullable"`
	}{Table: table, Name: name, Type: typ, Nullable: col.Nullable})
}

func (h *SchemaHandler) exec(c echo.Context, db catalog.Database, sqlText string) error {
	lease, err := h.svc.Acquire(c.Request().Context(), db, database.ReadWrite)
	if err != nil {
		return err
	}
	defer lease.Release()
	_, err = lease.Execute(c.Request().Context(), database.Statement{SQL: sqlText})
	return err
}

func (h *SchemaHandler) ensureSQL(c echo.Context, write bool) (catalog.Database, error) {
	if write && !h.writable {
		return catalog.Database{}, database.ErrWriterUnavailable
	}
	principal, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return catalog.Database{}, auth.ErrMissingCredentials
	}
	project, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return catalog.Database{}, fmt.Errorf("project context missing")
	}
	databaseID := c.Param("databaseID")
	db, err := h.svc.GetDatabase(c.Request().Context(), principal, project.ID, databaseID)
	if err != nil {
		return catalog.Database{}, err
	}
	if write && catalog.IsSystemDatabase(db) {
		return catalog.Database{}, catalog.ErrSystemProtected
	}
	if !catalog.IsSQLDataModel(db) {
		return catalog.Database{}, NewAPIError(http.StatusBadRequest, "data_model_mismatch", "只有 SQL 数据库可以管理表结构", RequestIDFromContext(c.Request().Context()))
	}
	return db, nil
}

func schemaInputError(c echo.Context, err error) error {
	msg := "表名或列定义不合法"
	if err != nil && strings.Contains(err.Error(), "column type") {
		msg = "不支持的列类型"
	}
	if err != nil && strings.Contains(err.Error(), "column count") {
		msg = "列数量必须在 1 到 32 之间"
	}
	if err != nil && strings.Contains(err.Error(), "duplicate column") {
		msg = "列名重复"
	}
	return NewAPIError(http.StatusBadRequest, "invalid_request", msg, RequestIDFromContext(c.Request().Context()))
}

func groupSchemaRows(rows [][]any) []schemaTableJSON {
	tables := make([]schemaTableJSON, 0)
	index := map[string]int{}
	for _, row := range rows {
		if len(row) < 4 {
			continue
		}
		table := cellString(row[0])
		col := schemaColumnJSON{
			Name:     cellString(row[1]),
			Type:     cellString(row[2]),
			Nullable: strings.EqualFold(cellString(row[3]), "YES"),
		}
		if table == "" || col.Name == "" {
			continue
		}
		i, ok := index[table]
		if !ok {
			index[table] = len(tables)
			tables = append(tables, schemaTableJSON{Name: table, Columns: []schemaColumnJSON{col}})
			continue
		}
		tables[i].Columns = append(tables[i].Columns, col)
	}
	return tables
}

func cellString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		if v == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
