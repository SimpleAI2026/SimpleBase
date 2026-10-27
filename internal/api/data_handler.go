// 文件说明：data_handler.go — internal/api 中 DataService、RecordEvent 等符号的实现
//
// 用法
//
//	NewDataHandler(svc DataService, writable bool) *DataHandler
//	(h *DataHandler) WithImportLimit(limit int64) *DataHandler
//	(h *DataHandler) WithCollections(schemas collectionReader, scanLimit int) ...
//	同包调用：projectID() → adapter.go, agent_handler.go 等；schema() → collection_filter.go, collection_meta_handler.go 等；collectionNamePattern → collection_meta_handler.go, function_trigger_handler.go 等
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/data_handler.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase, godatabase/sqlx, internal/auth, internal/catalog 等
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] projectID() ← adapter.go, agent_handler.go, agent_schedule_handler.go
//
// 主要符号
//
//	type DataService interface {…}
//	type RecordEvent struct {…}
//	type DataHandler struct {…}
//	NewDataHandler(svc DataService, writable bool) *DataHandler
//	(h *DataHandler) WithImportLimit(limit int64) *DataHandler
//	(h *DataHandler) WithCollections(schemas collectionReader, scanLimit int) ...
//	... 另有 43 个顶层符号

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlx"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/collection"
)

var collectionNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}$`)

type DataService interface {
	GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error)
	ListDatabases(ctx context.Context, principal auth.Principal, projectID string, page catalog.Page) ([]catalog.Database, string, error)
	Acquire(ctx context.Context, db catalog.Database, mode godatabase.AccessMode) (SQLLease, error)
}

type collectionReader interface {
	Get(ctx context.Context, projectID, name string) (*collection.Collection, error)
	List(ctx context.Context, projectID string) ([]collection.Collection, error)
}

type RecordEvent struct {
	ProjectID  string
	Collection string
	RecordID   string
	Action     string
	Changed    []string
	Data       map[string]any
}

type recordEventPublisher interface {
	PublishRecord(event RecordEvent)
}

type DataHandler struct {
	svc         DataService
	writable    bool
	schemas     collectionReader
	events      recordEventPublisher
	scanLimit   int
	importLimit int64
}

func NewDataHandler(svc DataService, writable bool) *DataHandler {
	return &DataHandler{svc: svc, writable: writable, scanLimit: 50000, importLimit: 128 << 20}
}

func (h *DataHandler) WithImportLimit(limit int64) *DataHandler {
	if limit > 0 {
		h.importLimit = limit
	}
	return h
}

func (h *DataHandler) WithCollections(schemas collectionReader, scanLimit int) *DataHandler {
	h.schemas = schemas
	if scanLimit > 0 {
		h.scanLimit = scanLimit
	}
	return h
}

func (h *DataHandler) WithEvents(events recordEventPublisher) *DataHandler {
	h.events = events
	return h
}

type collectionResponse struct {
	Collections []string         `json:"collections"`
	Items       []collectionInfo `json:"items,omitempty"`
}

type collectionInfo struct {
	Name          string `json:"name"`
	SchemaMode    string `json:"schema_mode"`
	FieldCount    int    `json:"field_count"`
	DocumentCount int64  `json:"document_count"`
	Registered    bool   `json:"registered"`
}

type rowsResponse struct {
	Rows       []map[string]any `json:"rows"`
	Items      []documentDTO    `json:"items"`
	NextCursor string           `json:"next_cursor,omitempty"`
	Total      *int64           `json:"total,omitempty"`
	RequestID  string           `json:"request_id,omitempty"`
}

type documentDTO struct {
	ID        string         `json:"id"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
	Data      map[string]any `json:"data"`
}

func (h *DataHandler) ListCollections(c echo.Context) error {
	db, lease, err := h.acquireDatabase(c, godatabase.ReadOnly)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()
	engine, err := catalog.DatabaseEngine(db.DatabasePath)
	if err != nil {
		engine = catalog.EngineDuckDB
	}
	dialect, err := godatabase.DialectFor(engine)
	if err != nil {
		return WriteError(c, err)
	}
	result, err := lease.Query(c.Request().Context(), godatabase.Statement{SQL: "SELECT DISTINCT table_name FROM (" + dialect.TableColumnsSQL("") + ") AS sb_tables ORDER BY table_name"}, 200)
	if err != nil {
		return WriteError(c, err)
	}
	response := collectionResponse{Collections: []string{}, Items: []collectionInfo{}}
	seen := map[string]struct{}{}
	for _, row := range result.Rows {
		if len(row) == 0 {
			continue
		}
		name, ok := row[0].(string)
		if !ok || godatabase.IsInternalTableName(name) {
			continue
		}
		seen[name] = struct{}{}
		response.Collections = append(response.Collections, name)
		info := collectionInfo{Name: name, SchemaMode: "json"}
		var schema *collection.Collection
		if schema, err = h.schema(c.Request().Context(), name); err == nil && schema != nil {
			info.Registered, info.FieldCount, info.SchemaMode = true, len(schema.Fields), schema.SchemaMode
		}
		count, countErr := lease.Query(c.Request().Context(), godatabase.Statement{SQL: "SELECT COUNT(*) FROM " + collectionSource(name, schema)}, 1)
		if countErr == nil && len(count.Rows) > 0 && len(count.Rows[0]) > 0 {
			info.DocumentCount = anyInt64(count.Rows[0][0])
		}
		response.Items = append(response.Items, info)
	}
	if h.schemas != nil {
		registered, listErr := h.schemas.List(c.Request().Context(), projectID(c))
		if listErr != nil && !errors.Is(listErr, collection.ErrUnavailable) {
			return WriteError(c, listErr)
		}
		for _, schema := range registered {
			if schema.SchemaMode != "view" {
				continue
			}
			if _, exists := seen[schema.Name]; exists {
				continue
			}
			info := collectionInfo{Name: schema.Name, SchemaMode: "view", Registered: true}
			if full, getErr := h.schema(c.Request().Context(), schema.Name); getErr == nil && full != nil {
				info.FieldCount = len(full.Fields)
				count, countErr := lease.Query(c.Request().Context(), godatabase.Statement{SQL: "SELECT COUNT(*) FROM " + collectionSource(schema.Name, full)}, 1)
				if countErr == nil && len(count.Rows) > 0 && len(count.Rows[0]) > 0 {
					info.DocumentCount = anyInt64(count.Rows[0][0])
				}
			}
			response.Collections = append(response.Collections, schema.Name)
			response.Items = append(response.Items, info)
		}
	}
	return c.JSON(http.StatusOK, response)
}

func (h *DataHandler) CreateCollection(c echo.Context) error {
	if !h.writable {
		return WriteError(c, godatabase.ErrWriterUnavailable)
	}
	if err := h.systemGuard(c); err != nil {
		return WriteError(c, err)
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSONBody(c, &req, false); err != nil {
		return WriteError(c, err)
	}
	if !collectionNamePattern.MatchString(req.Name) {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "集合名称仅支持字母、数字和下划线，且必须以字母开头", RequestIDFromContext(c.Request().Context())))
	}
	schema, err := h.schema(c.Request().Context(), req.Name)
	if err != nil {
		return validationAPIError(c, err)
	}
	if schema != nil && schema.SchemaMode == "view" {
		return WriteError(c, NewAPIError(http.StatusConflict, "collection_read_only", "view collections are read-only", RequestIDFromContext(c.Request().Context())))
	}
	lease, err := h.acquire(c, godatabase.ReadWrite)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()
	_, err = lease.Execute(c.Request().Context(), godatabase.Statement{SQL: createCollectionSQL(req.Name)})
	if err != nil {
		return WriteError(c, err)
	}
	return c.NoContent(http.StatusCreated)
}

func (h *DataHandler) ListDocuments(c echo.Context) error {
	name, err := collectionParam(c)
	if err != nil {
		return WriteError(c, err)
	}
	db, lease, err := h.acquireDatabase(c, godatabase.ReadOnly)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()
	schema, err := h.schema(c.Request().Context(), name)
	if err != nil {
		return validationAPIError(c, err)
	}
	engine, err := catalog.DatabaseEngine(db.DatabasePath)
	if err != nil {
		engine = catalog.EngineDuckDB
	}
	dialect, err := godatabase.DialectFor(engine)
	if err != nil {
		return WriteError(c, err)
	}
	query, err := parseCollectionQuery(c, name, schema, dialect, h.scanLimit)
	if err != nil {
		return validationAPIError(c, err)
	}
	if query.ScanSQL != "" {
		count, err := lease.Query(c.Request().Context(), godatabase.Statement{SQL: query.ScanSQL}, 1)
		if err != nil {
			if isMissingRelation(err) {
				return c.JSON(http.StatusOK, rowsResponse{Rows: []map[string]any{}, Items: []documentDTO{}})
			}
			return WriteError(c, err)
		}
		if len(count.Rows) > 0 && len(count.Rows[0]) > 0 && anyInt64(count.Rows[0][0]) > int64(h.scanLimit) {
			return scanLimitError(c)
		}
	}
	result, err := lease.Query(c.Request().Context(), godatabase.Statement{SQL: query.SQL, Args: query.Args}, query.Limit+1)
	if err != nil {
		if isMissingRelation(err) {
			return c.JSON(http.StatusOK, rowsResponse{Rows: []map[string]any{}, Items: []documentDTO{}})
		}
		return WriteError(c, err)
	}
	response := rowsResponse{Rows: []map[string]any{}, Items: []documentDTO{}, RequestID: RequestIDFromContext(c.Request().Context())}
	for i, row := range result.Rows {
		if i == query.Limit {
			response.NextCursor = nextCursorFromRow(result.Rows[i-1])
			break
		}
		dto, err := rowDocument(row, query.Fields)
		if err != nil {
			continue
		}
		dto.Data = hideDocumentFields(dto.Data, hiddenFieldNames(schema))
		response.Items = append(response.Items, dto)
		flat := flattenDocument(dto)
		response.Rows = append(response.Rows, flat)
	}
	if query.Count {
		count, err := lease.Query(c.Request().Context(), godatabase.Statement{SQL: query.CountSQL, Args: query.CountArgs}, 1)
		if err != nil {
			return WriteError(c, err)
		}
		if len(count.Rows) > 0 && len(count.Rows[0]) > 0 {
			total := anyInt64(count.Rows[0][0])
			response.Total = &total
		}
	}
	return c.JSON(http.StatusOK, response)
}

func (h *DataHandler) GetDocument(c echo.Context) error {
	name, lease, dto, err := h.loadDocument(c)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()
	schema, err := h.schema(c.Request().Context(), name)
	if err != nil {
		return validationAPIError(c, err)
	}
	dto.Data = hideDocumentFields(dto.Data, hiddenFieldNames(schema))
	return c.JSON(http.StatusOK, dto)
}

func (h *DataHandler) CreateDocument(c echo.Context) error {
	if err := h.writeGuard(c); err != nil {
		return WriteError(c, err)
	}
	name, err := collectionParam(c)
	if err != nil {
		return WriteError(c, err)
	}
	var doc map[string]any
	if err := decodeDocument(c, &doc); err != nil {
		return WriteError(c, err)
	}
	lease, err := h.acquire(c, godatabase.ReadWrite)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()
	schema, err := h.schema(c.Request().Context(), name)
	if err != nil {
		return validationAPIError(c, err)
	}
	if validation := protectReadonlyFields(schema, doc, nil); validation != nil {
		return validationAPIError(c, validation)
	}
	doc, validation := collection.ValidateDocument(schema, doc, true)
	if validation != nil {
		return validationAPIError(c, validation)
	}
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := h.ensureCollection(c, lease, name); err != nil {
		return WriteError(c, err)
	}
	if err := h.checkUnique(c, lease, name, schema, doc, ""); err != nil {
		return validationAPIError(c, err)
	}
	body, _ := json.Marshal(doc)
	_, err = lease.Execute(c.Request().Context(), godatabase.Statement{SQL: "INSERT INTO " + sqlx.Ident(name) + " (id, data, created_at, updated_at) VALUES (?, ?, ?, ?)", Args: []any{id, string(body), now, now}})
	if err != nil {
		return WriteError(c, err)
	}
	h.publish(c, RecordEvent{Collection: name, RecordID: id, Action: "create", Data: doc})
	return c.JSON(http.StatusCreated, flattenDocument(documentDTO{ID: id, CreatedAt: now, UpdatedAt: now, Data: hideDocumentFields(doc, hiddenFieldNames(schema))}))
}

func (h *DataHandler) UpdateDocument(c echo.Context) error {
	return h.replaceDocument(c, false)
}

func (h *DataHandler) PatchDocument(c echo.Context) error {
	return h.replaceDocument(c, true)
}

func (h *DataHandler) replaceDocument(c echo.Context, patch bool) error {
	if err := h.writeGuard(c); err != nil {
		return WriteError(c, err)
	}
	name, lease, before, err := h.loadDocumentForWrite(c)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()
	var input map[string]any
	if err := decodeDocument(c, &input); err != nil {
		return WriteError(c, err)
	}
	schema, err := h.schema(c.Request().Context(), name)
	if err != nil {
		return validationAPIError(c, err)
	}
	if validation := protectReadonlyFields(schema, input, before.Data); validation != nil {
		return validationAPIError(c, validation)
	}
	if patch {
		input = collection.MergePatch(before.Data, input)
	}
	input, validation := collection.ValidateStoredDocument(schema, input, !patch)
	if validation != nil {
		return validationAPIError(c, validation)
	}
	if err := h.checkUnique(c, lease, name, schema, input, before.ID); err != nil {
		return validationAPIError(c, err)
	}
	body, _ := json.Marshal(input)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = lease.Execute(c.Request().Context(), godatabase.Statement{SQL: "UPDATE " + sqlx.Ident(name) + " SET data = ?, updated_at = ? WHERE id = ?", Args: []any{string(body), now, before.ID}})
	if err != nil {
		return WriteError(c, err)
	}
	action := "update"
	h.publish(c, RecordEvent{Collection: name, RecordID: before.ID, Action: action, Changed: collection.ChangedFields(before.Data, input), Data: input})
	return c.JSON(http.StatusOK, flattenDocument(documentDTO{ID: before.ID, CreatedAt: before.CreatedAt, UpdatedAt: now, Data: hideDocumentFields(input, hiddenFieldNames(schema))}))
}

func (h *DataHandler) BatchDocuments(c echo.Context) error {
	if err := h.writeGuard(c); err != nil {
		return WriteError(c, err)
	}
	name, err := collectionParam(c)
	if err != nil {
		return WriteError(c, err)
	}
	var req struct {
		Documents []map[string]any `json:"documents"`
	}
	if err := decodeJSONBody(c, &req, false); err != nil {
		return WriteError(c, err)
	}
	if len(req.Documents) == 0 || len(req.Documents) > 100 {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "documents must contain 1..100 items", RequestIDFromContext(c.Request().Context())))
	}
	lease, err := h.acquire(c, godatabase.ReadWrite)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()
	schema, err := h.schema(c.Request().Context(), name)
	if err != nil {
		return validationAPIError(c, err)
	}
	if err := h.ensureCollection(c, lease, name); err != nil {
		return WriteError(c, err)
	}
	statements := make([]godatabase.Statement, 0, len(req.Documents))
	response := make([]documentDTO, 0, len(req.Documents))
	batchUnique := map[string]map[string]struct{}{}
	for i, doc := range req.Documents {
		if validation := protectReadonlyFields(schema, doc, nil); validation != nil {
			validation.Index = &i
			return validationAPIError(c, validation)
		}
		doc, validation := collection.ValidateDocument(schema, doc, true)
		if validation != nil {
			validation.Index = &i
			return validationAPIError(c, validation)
		}
		if schema != nil {
			for _, field := range schema.Fields {
				if !field.Unique || doc[field.Name] == nil {
					continue
				}
				encoded, _ := json.Marshal(doc[field.Name])
				key := string(encoded)
				if _, ok := batchUnique[field.Name][key]; ok {
					return validationAPIError(c, &collection.ValidationError{Code: "duplicate_value", Field: field.Name, Message: "duplicate value", Index: &i})
				}
				if batchUnique[field.Name] == nil {
					batchUnique[field.Name] = map[string]struct{}{}
				}
				batchUnique[field.Name][key] = struct{}{}
			}
		}
		if err := h.checkUnique(c, lease, name, schema, doc, ""); err != nil {
			if validation, ok := err.(*collection.ValidationError); ok {
				validation.Index = &i
			}
			return validationAPIError(c, err)
		}
		id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
		body, _ := json.Marshal(doc)
		statements = append(statements, godatabase.Statement{SQL: "INSERT INTO " + sqlx.Ident(name) + " (id, data, created_at, updated_at) VALUES (?, ?, ?, ?)", Args: []any{id, string(body), now, now}})
		response = append(response, documentDTO{ID: id, CreatedAt: now, UpdatedAt: now, Data: doc})
	}
	if _, err := lease.Batch(c.Request().Context(), statements, true); err != nil {
		return WriteError(c, err)
	}
	for i, dto := range response {
		h.publish(c, RecordEvent{Collection: name, RecordID: dto.ID, Action: "create", Data: dto.Data})
		response[i].Data = hideDocumentFields(dto.Data, hiddenFieldNames(schema))
	}
	return c.JSON(http.StatusCreated, map[string]any{"items": response})
}

func (h *DataHandler) DeleteDocument(c echo.Context) error {
	if err := h.writeGuard(c); err != nil {
		return WriteError(c, err)
	}
	name, err := collectionParam(c)
	if err != nil {
		return WriteError(c, err)
	}
	id := c.Param("id")
	lease, err := h.acquire(c, godatabase.ReadWrite)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()
	_, err = lease.Execute(c.Request().Context(), godatabase.Statement{SQL: "DELETE FROM " + sqlx.Ident(name) + " WHERE id = ?", Args: []any{id}})
	if err != nil && !isMissingRelation(err) {
		return WriteError(c, err)
	}
	h.publish(c, RecordEvent{Collection: name, RecordID: id, Action: "delete"})
	return c.NoContent(http.StatusNoContent)
}

func (h *DataHandler) TruncateCollection(c echo.Context) error {
	if err := h.writeGuard(c); err != nil {
		return WriteError(c, err)
	}
	name, err := collectionParam(c)
	if err != nil {
		return WriteError(c, err)
	}
	lease, err := h.acquire(c, godatabase.ReadWrite)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()
	if _, err := lease.Execute(c.Request().Context(), godatabase.Statement{SQL: "DELETE FROM " + sqlx.Ident(name)}); err != nil && !isMissingRelation(err) {
		return WriteError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *DataHandler) writeGuard(c echo.Context) error {
	if !h.writable {
		return godatabase.ErrWriterUnavailable
	}
	return h.systemGuard(c)
}

func (h *DataHandler) schema(ctx context.Context, name string) (*collection.Collection, error) {
	if h.schemas == nil {
		return nil, nil
	}
	schema, err := h.schemas.Get(ctx, projectIDFrom(ctx), name)
	if errors.Is(err, collection.ErrNotFound) || errors.Is(err, collection.ErrUnavailable) {
		return nil, nil
	}
	if err != nil || schema == nil {
		return schema, err
	}
	principal, _ := PrincipalFromContext(ctx)
	if principal.Scope != nil && len(principal.Scope.HiddenFields) > 0 {
		copySchema := *schema
		copySchema.Fields = append([]collection.Field(nil), schema.Fields...)
		for _, name := range principal.Scope.HiddenFields {
			copySchema.Fields = append(copySchema.Fields, collection.Field{Name: name, Type: "json", Hidden: true})
		}
		return &copySchema, nil
	}
	return schema, nil
}

func (h *DataHandler) checkUnique(c echo.Context, lease SQLLease, name string, schema *collection.Collection, doc map[string]any, excludeID string) error {
	if schema == nil {
		return nil
	}
	for _, f := range schema.Fields {
		if !f.Unique || doc[f.Name] == nil {
			continue
		}
		body, _ := json.Marshal(doc[f.Name])
		sqlText := "SELECT id FROM " + sqlx.Ident(name) + " WHERE json_extract(data, '$." + f.Name + "') = json_extract(?, '$')"
		args := []any{string(body)}
		if excludeID != "" {
			sqlText += " AND id <> ?"
			args = append(args, excludeID)
		}
		result, err := lease.Query(c.Request().Context(), godatabase.Statement{SQL: sqlText, Args: args}, 1)
		if err != nil {
			return err
		}
		if len(result.Rows) > 0 {
			return &collection.ValidationError{Code: "duplicate_value", Field: f.Name, Message: "duplicate value"}
		}
	}
	return nil
}

func (h *DataHandler) ensureCollection(c echo.Context, lease SQLLease, name string) error {
	if _, err := lease.Execute(c.Request().Context(), godatabase.Statement{SQL: createCollectionSQL(name)}); err != nil {
		return err
	}
	_, err := lease.Query(c.Request().Context(), godatabase.Statement{SQL: "SELECT updated_at FROM " + sqlx.Ident(name) + " LIMIT 0"}, 0)
	if err != nil {
		_, err = lease.Execute(c.Request().Context(), godatabase.Statement{SQL: "ALTER TABLE " + sqlx.Ident(name) + " ADD COLUMN updated_at VARCHAR"})
	}
	return err
}

func (h *DataHandler) loadDocument(c echo.Context) (string, SQLLease, documentDTO, error) {
	name, lease, dto, err := h.loadDocumentWithMode(c, godatabase.ReadOnly)
	return name, lease, dto, err
}

func (h *DataHandler) loadDocumentForWrite(c echo.Context) (string, SQLLease, documentDTO, error) {
	return h.loadDocumentWithMode(c, godatabase.ReadWrite)
}

func (h *DataHandler) loadDocumentWithMode(c echo.Context, mode godatabase.AccessMode) (string, SQLLease, documentDTO, error) {
	name, err := collectionParam(c)
	if err != nil {
		return "", nil, documentDTO{}, err
	}
	if c.Param("id") == "" {
		return "", nil, documentDTO{}, NewAPIError(http.StatusBadRequest, "invalid_request", "document id is required", RequestIDFromContext(c.Request().Context()))
	}
	schema, err := h.schema(c.Request().Context(), name)
	if err != nil {
		return "", nil, documentDTO{}, err
	}
	lease, err := h.acquire(c, mode)
	if err != nil {
		return "", nil, documentDTO{}, err
	}
	statement := godatabase.Statement{SQL: selectCollectionDocumentsSQL(name, schema) + " WHERE id = ?", Args: []any{c.Param("id")}}
	result, err := lease.Query(c.Request().Context(), statement, 2)
	if err != nil {
		lease.Release()
		if isMissingRelation(err) {
			return "", nil, documentDTO{}, NewAPIError(http.StatusNotFound, "not_found", "document not found", RequestIDFromContext(c.Request().Context()))
		}
		return "", nil, documentDTO{}, err
	}
	if len(result.Rows) == 0 {
		lease.Release()
		return "", nil, documentDTO{}, NewAPIError(http.StatusNotFound, "not_found", "document not found", RequestIDFromContext(c.Request().Context()))
	}
	dto, err := rowDocument(result.Rows[0], nil)
	if err != nil {
		lease.Release()
		return "", nil, documentDTO{}, err
	}
	return name, lease, dto, nil
}

func (h *DataHandler) acquire(c echo.Context, mode godatabase.AccessMode) (SQLLease, error) {
	_, lease, err := h.acquireDatabase(c, mode)
	return lease, err
}

func (h *DataHandler) acquireDatabase(c echo.Context, mode godatabase.AccessMode) (catalog.Database, SQLLease, error) {
	principal, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return catalog.Database{}, nil, auth.ErrMissingCredentials
	}
	project, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return catalog.Database{}, nil, errors.New("project context missing")
	}
	databaseID := c.Param("databaseID")
	if databaseID == "" {
		return catalog.Database{}, nil, errors.New("databaseID required")
	}
	db, err := h.svc.GetDatabase(c.Request().Context(), principal, project.ID, databaseID)
	if err != nil {
		return catalog.Database{}, nil, err
	}
	if catalog.IsSQLDataModel(db) {
		return catalog.Database{}, nil, NewAPIError(http.StatusBadRequest, "data_model_mismatch", "SQL 数据库不支持集合与文档", RequestIDFromContext(c.Request().Context()))
	}
	if mode == godatabase.ReadWrite && c.Param("collection") != "" {
		schema, schemaErr := h.schema(c.Request().Context(), c.Param("collection"))
		if schemaErr != nil {
			return catalog.Database{}, nil, schemaErr
		}
		if schema != nil && schema.SchemaMode == "view" {
			return catalog.Database{}, nil, NewAPIError(http.StatusConflict, "collection_read_only", "view collections are read-only", RequestIDFromContext(c.Request().Context()))
		}
	}
	lease, err := h.svc.Acquire(c.Request().Context(), db, mode)
	return db, lease, err
}

func (h *DataHandler) systemGuard(c echo.Context) error {
	principal, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return auth.ErrMissingCredentials
	}
	project, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return errors.New("project context missing")
	}
	db, err := h.svc.GetDatabase(c.Request().Context(), principal, project.ID, c.Param("databaseID"))
	if err != nil {
		return err
	}
	if catalog.IsSystemDatabase(db) {
		return catalog.ErrSystemProtected
	}
	return nil
}

func (h *DataHandler) publish(c echo.Context, event RecordEvent) {
	if h.events != nil {
		event.ProjectID = projectID(c)
		h.events.PublishRecord(event)
	}
}

func collectionParam(c echo.Context) (string, error) {
	name := c.Param("collection")
	if !collectionNamePattern.MatchString(name) {
		return "", NewAPIError(http.StatusBadRequest, "invalid_request", "invalid collection name", RequestIDFromContext(c.Request().Context()))
	}
	return name, nil
}

func createCollectionSQL(name string) string {
	return "CREATE TABLE IF NOT EXISTS " + sqlx.Ident(name) + " (id VARCHAR NOT NULL, data VARCHAR NOT NULL, created_at VARCHAR NOT NULL, updated_at VARCHAR)"
}

func rowDocument(row []any, fields []string) (documentDTO, error) {
	var dto documentDTO
	if len(row) < 2 {
		return dto, errors.New("invalid document row")
	}
	dto.ID, _ = row[0].(string)
	raw, _ := row[1].(string)
	if err := json.Unmarshal([]byte(raw), &dto.Data); err != nil {
		dto.Data = map[string]any{}
	}
	if len(row) > 2 {
		dto.CreatedAt, _ = row[2].(string)
	}
	if len(row) > 3 {
		dto.UpdatedAt, _ = row[3].(string)
	}
	if dto.UpdatedAt == "" {
		dto.UpdatedAt = dto.CreatedAt
	}
	dto.Data = pickDocument(dto.Data, fieldsWithoutSystem(fields))
	return dto, nil
}

func fieldsWithoutSystem(fields []string) []string {
	if len(fields) == 0 {
		return nil
	}
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f != "id" && f != "created_at" && f != "updated_at" {
			out = append(out, f)
		}
	}
	return out
}

func hiddenFieldNames(schema *collection.Collection) []string {
	if schema == nil {
		return nil
	}
	hidden := make([]string, 0)
	for _, field := range schema.Fields {
		if field.Hidden {
			hidden = append(hidden, field.Name)
		}
	}
	return hidden
}

func flattenDocument(dto documentDTO) map[string]any {
	out := make(map[string]any, len(dto.Data)+3)
	for k, v := range dto.Data {
		out[k] = v
	}
	out["id"], out["created_at"], out["updated_at"] = dto.ID, dto.CreatedAt, dto.UpdatedAt
	return out
}

func nextCursorFromRow(row []any) string {
	if len(row) < 3 {
		return ""
	}
	id, _ := row[0].(string)
	created, _ := row[2].(string)
	return encodeCursor(created, id)
}

func protectReadonlyFields(schema *collection.Collection, input, before map[string]any) *collection.ValidationError {
	if schema == nil {
		return nil
	}
	for _, field := range schema.Fields {
		if !field.Readonly {
			continue
		}
		value, supplied := input[field.Name]
		previous, existed := before[field.Name]
		if before == nil && supplied {
			return &collection.ValidationError{Code: "readonly_field", Field: field.Name, Message: "field is read-only"}
		}
		if supplied {
			left, _ := json.Marshal(value)
			right, _ := json.Marshal(previous)
			if !existed || string(left) != string(right) {
				return &collection.ValidationError{Code: "readonly_field", Field: field.Name, Message: "field is read-only"}
			}
		}
		if existed {
			input[field.Name] = previous
		}
	}
	return nil
}

func anyInt64(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int32:
		return int64(v)
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case uint64:
		return int64(v)
	case []byte:
		n, _ := strconv.ParseInt(string(v), 10, 64)
		return n
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	default:
		return 0
	}
}

func projectID(c echo.Context) string { return projectIDFrom(c.Request().Context()) }

func projectIDFrom(ctx context.Context) string {
	project, _ := ProjectFromContext(ctx)
	return project.ID
}

func decodeDocument(c echo.Context, doc *map[string]any) error {
	if err := json.NewDecoder(c.Request().Body).Decode(doc); err != nil || *doc == nil {
		return NewAPIError(http.StatusBadRequest, "invalid_request", "document must be a JSON object", RequestIDFromContext(c.Request().Context()))
	}
	delete(*doc, "id")
	delete(*doc, "created_at")
	delete(*doc, "updated_at")
	principal, _ := PrincipalFromContext(c.Request().Context())
	if principal.Scope != nil {
		for _, name := range principal.Scope.ReadonlyFields {
			if _, ok := (*doc)[name]; ok {
				return NewAPIError(http.StatusForbidden, "key_scope_denied", "key scope denied", RequestIDFromContext(c.Request().Context()))
			}
		}
	}
	return nil
}

func isMissingRelation(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "no such table") || strings.Contains(s, "does not exist") || strings.Contains(s, "not found")
}
