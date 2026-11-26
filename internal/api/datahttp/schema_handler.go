package datahttp

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlguard"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/dbbridge"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

// SchemaHandler 管理 SQL 数据库的表与列。集合库不能走这些路由。
// 系统库只读：可以列出表和分页读行，DDL 被拒绝。
type SchemaHandler struct {
	svc      DataService
	writable bool
	limits   SQLLimits
	// 聚合接口的超时与扫描上限；零值走 collection_aggregate.go 的默认值。
	aggregateLimitConfig
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
		return httpkit.WriteError(c, err)
	}
	lease, err := h.svc.Acquire(c.Request().Context(), db, godatabase.ReadOnly)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	defer lease.Release()
	dialect, err := dialectForDatabase(db)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	result, err := lease.Query(c.Request().Context(), godatabase.Statement{
		SQL: dialect.TableColumnsSQL(""),
	}, 5000)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	tables := finishSchemaTables(groupSchemaRows(result.Rows), catalog.IsSystemDatabase(db))
	return c.JSON(http.StatusOK, schemaResponse{Tables: tables})
}

// CreateTable: POST .../schema/tables
func (h *SchemaHandler) CreateTable(c echo.Context) error {
	db, err := h.ensureSQL(c, true)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	var req createTableRequest
	if err := httpkit.DecodeJSONBody(c, &req, true); err != nil {
		return httpkit.WriteError(c, err)
	}
	cols := make([]schemaColumnSpec, 0, len(req.Columns))
	for _, col := range req.Columns {
		cols = append(cols, schemaColumnSpec{Name: col.Name, Type: col.Type, Nullable: columnNullable(col.Nullable)})
	}
	sqlText, err := buildCreateTableSQL(req.Name, cols)
	if err != nil {
		return httpkit.WriteError(c, schemaInputError(c, err))
	}
	if err := sqlguard.Validate(sqlText, sqlguard.WriteAllowed); err != nil {
		return httpkit.WriteError(c, err)
	}
	if err := h.exec(c, db, sqlText); err != nil {
		return httpkit.WriteError(c, err)
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
		return httpkit.WriteError(c, err)
	}
	var req addColumnRequest
	if err := httpkit.DecodeJSONBody(c, &req, true); err != nil {
		return httpkit.WriteError(c, err)
	}
	col := schemaColumnSpec{Name: req.Name, Type: req.Type, Nullable: columnNullable(req.Nullable)}
	sqlText, err := buildAddColumnSQL(req.Table, col)
	if err != nil {
		return httpkit.WriteError(c, schemaInputError(c, err))
	}
	if err := sqlguard.Validate(sqlText, sqlguard.WriteAllowed); err != nil {
		return httpkit.WriteError(c, err)
	}
	if err := h.exec(c, db, sqlText); err != nil {
		return httpkit.WriteError(c, err)
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
	lease, err := h.svc.Acquire(c.Request().Context(), db, godatabase.ReadWrite)
	if err != nil {
		return err
	}
	defer lease.Release()
	_, err = lease.Execute(c.Request().Context(), godatabase.Statement{SQL: sqlText})
	return err
}

func (h *SchemaHandler) ensureSQL(c echo.Context, write bool) (catalog.Database, error) {
	if write && !h.writable {
		return catalog.Database{}, godatabase.ErrWriterUnavailable
	}
	principal, ok := httpkit.PrincipalFromContext(c.Request().Context())
	if !ok {
		return catalog.Database{}, auth.ErrMissingCredentials
	}
	project, err := httpkit.RequireProject(c)
	if err != nil {
		return catalog.Database{}, err
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
		return catalog.Database{}, httpkit.NewAPIError(http.StatusBadRequest, "data_model_mismatch", "只有 SQL 数据库可以管理表结构", httpkit.RequestID(c))
	}
	return db, nil
}

// dialectForDatabase 按 catalog 行解析 SQL 方言；规则归 dbbridge（宿主与 godatabase 的适配层）。
func dialectForDatabase(db catalog.Database) (godatabase.Dialect, error) {
	return dbbridge.DialectOf(db)
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
	return httpkit.BadRequest(c, msg)
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
