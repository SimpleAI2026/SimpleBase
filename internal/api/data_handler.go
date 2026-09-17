package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
)

var collectionNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}$`)

// DataService is the minimal catalog and database access required by the document API.
type DataService interface {
	GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error)
	ListDatabases(ctx context.Context, principal auth.Principal, projectID string, page catalog.Page) ([]catalog.Database, string, error)
	Acquire(ctx context.Context, db catalog.Database, mode database.AccessMode) (SQLLease, error)
}

type DataHandler struct {
	svc      DataService
	writable bool
}

func NewDataHandler(svc DataService, writable bool) *DataHandler {
	return &DataHandler{svc: svc, writable: writable}
}

type collectionResponse struct {
	Collections []string `json:"collections"`
}

type rowsResponse struct {
	Rows []map[string]any `json:"rows"`
}

func (h *DataHandler) ListCollections(c echo.Context) error {
	lease, err := h.acquire(c, database.ReadOnly)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()

	result, err := lease.Query(c.Request().Context(), database.Statement{
		SQL: `SELECT table_name FROM information_schema.tables
			WHERE table_catalog = current_database()
			  AND table_schema = current_schema()
			  AND table_type = 'BASE TABLE'
			ORDER BY table_name`,
	}, 200)
	if err != nil {
		return WriteError(c, err)
	}
	collections := make([]string, 0, len(result.Rows))
	for _, row := range result.Rows {
		if len(row) > 0 {
			if name, ok := row[0].(string); ok {
				collections = append(collections, name)
			}
		}
	}
	return c.JSON(http.StatusOK, collectionResponse{Collections: collections})
}

func (h *DataHandler) CreateCollection(c echo.Context) error {
	if !h.writable {
		return WriteError(c, database.ErrWriterUnavailable)
	}
	if err := h.systemGuard(c); err != nil {
		return WriteError(c, err)
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "malformed JSON body", RequestIDFromContext(c.Request().Context())))
	}
	if !collectionNamePattern.MatchString(req.Name) {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "集合名称仅支持字母、数字和下划线，且必须以字母开头", RequestIDFromContext(c.Request().Context())))
	}
	lease, err := h.acquire(c, database.ReadWrite)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()
	_, err = lease.Execute(c.Request().Context(), database.Statement{SQL: createCollectionSQL(req.Name)})
	if err != nil {
		return WriteError(c, err)
	}
	return c.NoContent(http.StatusCreated)
}

func (h *DataHandler) ListDocuments(c echo.Context) error {
	collection, err := collectionParam(c)
	if err != nil {
		return WriteError(c, err)
	}
	lease, err := h.acquire(c, database.ReadOnly)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()
	result, err := lease.Query(c.Request().Context(), database.Statement{SQL: "SELECT id, data FROM " + quoteIdentifier(collection) + " ORDER BY created_at DESC"}, 1000)
	if err != nil {
		if isMissingRelation(err) {
			return c.JSON(http.StatusOK, rowsResponse{Rows: []map[string]any{}})
		}
		return WriteError(c, err)
	}
	rows := make([]map[string]any, 0, len(result.Rows))
	for _, row := range result.Rows {
		if len(row) != 2 {
			continue
		}
		id, _ := row[0].(string)
		data, _ := row[1].(string)
		doc := map[string]any{}
		if json.Unmarshal([]byte(data), &doc) != nil {
			doc = map[string]any{}
		}
		doc["id"] = id
		rows = append(rows, doc)
	}
	return c.JSON(http.StatusOK, rowsResponse{Rows: rows})
}

func (h *DataHandler) CreateDocument(c echo.Context) error {
	if !h.writable {
		return WriteError(c, database.ErrWriterUnavailable)
	}
	if err := h.systemGuard(c); err != nil {
		return WriteError(c, err)
	}
	collection, err := collectionParam(c)
	if err != nil {
		return WriteError(c, err)
	}
	var doc map[string]any
	if err := json.NewDecoder(c.Request().Body).Decode(&doc); err != nil || doc == nil {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "文档必须是 JSON 对象", RequestIDFromContext(c.Request().Context())))
	}
	id, _ := doc["id"].(string)
	if id == "" {
		id = uuid.NewString()
	}
	delete(doc, "id")
	body, err := json.Marshal(doc)
	if err != nil {
		return WriteError(c, err)
	}
	lease, err := h.acquire(c, database.ReadWrite)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()
	_, err = lease.Execute(c.Request().Context(), database.Statement{SQL: createCollectionSQL(collection)})
	if err == nil {
		_, err = lease.Execute(c.Request().Context(), database.Statement{SQL: "INSERT INTO " + quoteIdentifier(collection) + " (id, data, created_at) VALUES (?, ?, ?)", Args: []any{id, string(body), time.Now().UTC().Format(time.RFC3339Nano)}})
	}
	if err != nil {
		return WriteError(c, err)
	}
	doc["id"] = id
	return c.JSON(http.StatusCreated, doc)
}

func (h *DataHandler) UpdateDocument(c echo.Context) error {
	if !h.writable {
		return WriteError(c, database.ErrWriterUnavailable)
	}
	if err := h.systemGuard(c); err != nil {
		return WriteError(c, err)
	}
	collection, err := collectionParam(c)
	if err != nil {
		return WriteError(c, err)
	}
	id := c.Param("id")
	if id == "" {
		return WriteError(c, errors.New("document id required"))
	}
	var doc map[string]any
	if err := json.NewDecoder(c.Request().Body).Decode(&doc); err != nil || doc == nil {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "文档必须是 JSON 对象", RequestIDFromContext(c.Request().Context())))
	}
	delete(doc, "id")
	body, err := json.Marshal(doc)
	if err != nil {
		return WriteError(c, err)
	}
	lease, err := h.acquire(c, database.ReadWrite)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()
	result, err := lease.Execute(c.Request().Context(), database.Statement{
		SQL:  "UPDATE " + quoteIdentifier(collection) + " SET data = ? WHERE id = ?",
		Args: []any{string(body), id},
	})
	if err != nil {
		return WriteError(c, err)
	}
	if result.RowsAffected == 0 {
		return WriteError(c, NewAPIError(http.StatusNotFound, "not_found", "文档不存在", RequestIDFromContext(c.Request().Context())))
	}
	doc["id"] = id
	return c.JSON(http.StatusOK, doc)
}

func (h *DataHandler) DeleteDocument(c echo.Context) error {
	if !h.writable {
		return WriteError(c, database.ErrWriterUnavailable)
	}
	if err := h.systemGuard(c); err != nil {
		return WriteError(c, err)
	}
	collection, err := collectionParam(c)
	if err != nil {
		return WriteError(c, err)
	}
	id := c.Param("id")
	if id == "" {
		return WriteError(c, errors.New("document id required"))
	}
	lease, err := h.acquire(c, database.ReadWrite)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()
	_, err = lease.Execute(c.Request().Context(), database.Statement{SQL: "DELETE FROM " + quoteIdentifier(collection) + " WHERE id = ?", Args: []any{id}})
	if err != nil && !isMissingRelation(err) {
		return WriteError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *DataHandler) acquire(c echo.Context, mode database.AccessMode) (SQLLease, error) {
	principal, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return nil, auth.ErrMissingCredentials
	}
	project, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return nil, errors.New("project context missing")
	}
	// Path with :databaseID must use that database. Never fall back to "first DB".
	if databaseID := c.Param("databaseID"); databaseID != "" {
		db, err := h.svc.GetDatabase(c.Request().Context(), principal, project.ID, databaseID)
		if err != nil {
			return nil, err
		}
		return h.svc.Acquire(c.Request().Context(), db, mode)
	}
	// Legacy project-scoped routes: first database only.
	databases, _, err := h.svc.ListDatabases(c.Request().Context(), principal, project.ID, catalog.Page{Limit: 1})
	if err != nil {
		return nil, err
	}
	if len(databases) == 0 {
		return nil, fmt.Errorf("no database configured for project")
	}
	db, err := h.svc.GetDatabase(c.Request().Context(), principal, project.ID, databases[0].ID)
	if err != nil {
		return nil, err
	}
	return h.svc.Acquire(c.Request().Context(), db, mode)
}

// systemGuard 写路径前置调用：系统库只读，拒绝改写。
// 只查 catalog 记录，不开数据库连接。
func (h *DataHandler) systemGuard(c echo.Context) error {
	principal, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return auth.ErrMissingCredentials
	}
	project, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return errors.New("project context missing")
	}
	databaseID := c.Param("databaseID")
	if databaseID == "" {
		// Legacy project-scoped 路由：取第一个库判定。
		databases, _, err := h.svc.ListDatabases(c.Request().Context(), principal, project.ID, catalog.Page{Limit: 1})
		if err != nil {
			return err
		}
		if len(databases) == 0 {
			return fmt.Errorf("no database configured for project")
		}
		databaseID = databases[0].ID
	}
	db, err := h.svc.GetDatabase(c.Request().Context(), principal, project.ID, databaseID)
	if err != nil {
		return err
	}
	if catalog.IsSystemDatabase(db) {
		return catalog.ErrSystemProtected
	}
	return nil
}

func collectionParam(c echo.Context) (string, error) {
	name := c.Param("collection")
	if !collectionNamePattern.MatchString(name) {
		return "", NewAPIError(http.StatusBadRequest, "invalid_request", "invalid collection name", RequestIDFromContext(c.Request().Context()))
	}
	return name, nil
}

func quoteIdentifier(name string) string { return `"` + name + `"` }

// DuckLake 不支持 PRIMARY KEY / sequences；文档 ID 由应用层生成 UUID。
func createCollectionSQL(name string) string {
	return "CREATE TABLE IF NOT EXISTS " + quoteIdentifier(name) +
		" (id VARCHAR NOT NULL, data VARCHAR NOT NULL, created_at VARCHAR NOT NULL)"
}

func isMissingRelation(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "no such table") ||
		strings.Contains(s, "does not exist") ||
		strings.Contains(s, "not found")
}
