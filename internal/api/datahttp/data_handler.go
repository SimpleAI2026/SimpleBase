package datahttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlx"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/collection"
	cquery "github.com/linkxzhou/SimpleBase/internal/collection/query"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

type DataService interface {
	GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error)
	ListDatabases(ctx context.Context, principal auth.Principal, projectID string, page catalog.Page) ([]catalog.Database, string, error)
	Acquire(ctx context.Context, db catalog.Database, mode godatabase.AccessMode) (SQLLease, error)
}

type collectionReader interface {
	Get(ctx context.Context, projectID, name string) (*collection.Collection, error)
	List(ctx context.Context, projectID string) ([]collection.Collection, error)
}

// recordEventPublisher 是集合写路径发布领域事件的端口（发布语言 collection.RecordEvent）；
// fan-out 的订阅方（realtime、trigger）由 app 在装配时登记。
type recordEventPublisher interface {
	PublishRecord(event collection.RecordEvent)
}

type DataHandler struct {
	svc         DataService
	writable    bool
	schemas     collectionReader
	events      recordEventPublisher
	scanLimit   int
	importLimit int64
	getSetting  settingGetter
	putSetting  settingPutter
	audit       auditRecorder
	aggregateLimitConfig
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
		return httpkit.WriteError(c, err)
	}
	defer lease.Release()
	engine, err := catalog.DatabaseEngine(db.DatabasePath)
	if err != nil {
		engine = catalog.EngineDuckDB
	}
	dialect, err := godatabase.DialectFor(engine)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	// 只把「文档形」表当集合：id / data / created_at / updated_at 缺一不可。
	// 这条规则同时挡住两类会被文档接口立刻拒绝的表：
	//   - dev apply 的记账表 _migrations（下划线开头，collection.NamePattern 会 400）；
	//   - 迁移里用原生 SQL 建的业务表（没有 data 列，文档查询固定 SELECT 四列会 500）。
	// 名字复用与文档接口同一份 collection.NamePattern，避免列表与读写口径漂移。
	tablesSQL := `SELECT table_name,
		MAX(CASE WHEN LOWER(column_name) = 'id' THEN 1 ELSE 0 END) AS has_id,
		MAX(CASE WHEN LOWER(column_name) = 'data' THEN 1 ELSE 0 END) AS has_data,
		MAX(CASE WHEN LOWER(column_name) = 'created_at' THEN 1 ELSE 0 END) AS has_created_at,
		MAX(CASE WHEN LOWER(column_name) = 'updated_at' THEN 1 ELSE 0 END) AS has_updated_at
		FROM (` + dialect.TableColumnsSQL("") + `) AS sb_tables
		GROUP BY table_name
		ORDER BY table_name`
	result, err := lease.Query(c.Request().Context(), godatabase.Statement{SQL: tablesSQL}, 1000)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	ctx := c.Request().Context()
	response := collectionResponse{Collections: []string{}, Items: []collectionInfo{}}
	seen := h.appendPhysicalCollections(ctx, lease, result.Rows, &response)
	if h.schemas != nil {
		registered, listErr := h.schemas.List(ctx, httpkit.ProjectID(c))
		if listErr != nil && !errors.Is(listErr, collection.ErrUnavailable) {
			return httpkit.WriteError(c, listErr)
		}
		h.appendViewCollections(ctx, lease, registered, seen, &response)
	}
	return c.JSON(http.StatusOK, response)
}

// countCollection 返回集合当前文档数；查询失败或无结果按 0 处理（列表页只是展示用）。
func countCollection(ctx context.Context, lease SQLLease, name string, schema *collection.Collection) int64 {
	count, err := lease.Query(ctx, godatabase.Statement{SQL: "SELECT COUNT(*) FROM " + collectionSource(name, schema)}, 1)
	if err != nil || len(count.Rows) == 0 || len(count.Rows[0]) == 0 {
		return 0
	}
	return anyInt64(count.Rows[0][0])
}

// appendPhysicalCollections 把「文档形」物理表并入响应，返回已出现的集合名集合。
func (h *DataHandler) appendPhysicalCollections(ctx context.Context, lease SQLLease, rows [][]any, response *collectionResponse) map[string]struct{} {
	seen := map[string]struct{}{}
	for _, row := range rows {
		if len(row) < 5 {
			continue
		}
		name, ok := row[0].(string)
		if !ok || godatabase.IsInternalTableName(name) || !collection.NamePattern.MatchString(name) {
			continue
		}
		if anyInt64(row[1]) == 0 || anyInt64(row[2]) == 0 || anyInt64(row[3]) == 0 || anyInt64(row[4]) == 0 {
			continue
		}
		seen[name] = struct{}{}
		response.Collections = append(response.Collections, name)
		info := collectionInfo{Name: name, SchemaMode: "json"}
		schema, err := h.schema(ctx, name)
		if err == nil && schema != nil {
			info.Registered, info.FieldCount, info.SchemaMode = true, len(schema.Fields), schema.SchemaMode
		}
		info.DocumentCount = countCollection(ctx, lease, name, schema)
		response.Items = append(response.Items, info)
	}
	return seen
}

// appendViewCollections 把已登记的视图型集合并入响应；物理表已出现或名字非法的跳过。
func (h *DataHandler) appendViewCollections(ctx context.Context, lease SQLLease, registered []collection.Collection, seen map[string]struct{}, response *collectionResponse) {
	for _, schema := range registered {
		if schema.SchemaMode != "view" {
			continue
		}
		// 与物理表同一口径：名字不合法时文档接口必然 400，不要放进列表。
		if !collection.NamePattern.MatchString(schema.Name) {
			continue
		}
		if _, exists := seen[schema.Name]; exists {
			continue
		}
		info := collectionInfo{Name: schema.Name, SchemaMode: "view", Registered: true}
		if full, err := h.schema(ctx, schema.Name); err == nil && full != nil {
			info.FieldCount = len(full.Fields)
			info.DocumentCount = countCollection(ctx, lease, schema.Name, full)
		}
		response.Collections = append(response.Collections, schema.Name)
		response.Items = append(response.Items, info)
	}
}

func (h *DataHandler) CreateCollection(c echo.Context) error {
	if !h.writable {
		return httpkit.WriteError(c, godatabase.ErrWriterUnavailable)
	}
	if err := h.systemGuard(c); err != nil {
		return httpkit.WriteError(c, err)
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := httpkit.DecodeJSONBody(c, &req, false); err != nil {
		return httpkit.WriteError(c, err)
	}
	if !collection.NamePattern.MatchString(req.Name) {
		return httpkit.WriteError(c, httpkit.BadRequest(c, "集合名称仅支持字母、数字和下划线，且必须以字母开头"))
	}
	schema, err := h.schema(c.Request().Context(), req.Name)
	if err != nil {
		return validationAPIError(c, err)
	}
	if schema != nil && schema.SchemaMode == "view" {
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusConflict, "collection_read_only", "view collections are read-only", httpkit.RequestID(c)))
	}
	lease, err := h.acquire(c, godatabase.ReadWrite)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	defer lease.Release()
	_, err = lease.Execute(c.Request().Context(), godatabase.Statement{SQL: createCollectionSQL(req.Name)})
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	return c.NoContent(http.StatusCreated)
}

func (h *DataHandler) ListDocuments(c echo.Context) error {
	name, err := collectionParam(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	db, lease, err := h.acquireDatabase(c, godatabase.ReadOnly)
	if err != nil {
		return httpkit.WriteError(c, err)
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
		return httpkit.WriteError(c, err)
	}
	rule, actor, admin, err := h.ruleScope(c, name)
	if err != nil {
		return h.ruleError(c, err)
	}
	extraWhere := ""
	var extraArgs []any
	if rule.RestrictsRead() && !admin {
		extraWhere = cquery.OwnerCondition(dialect, rule.OwnerField)
		extraArgs = []any{actor}
	}
	query, err := parseCollectionQuery(c, name, schema, dialect, h.scanLimit, extraWhere, extraArgs)
	if err != nil {
		return validationAPIError(c, err)
	}
	if query.ScanSQL != "" {
		count, err := lease.Query(c.Request().Context(), godatabase.Statement{SQL: query.ScanSQL}, 1)
		if err != nil {
			if isMissingRelation(err) {
				return c.JSON(http.StatusOK, rowsResponse{Rows: []map[string]any{}, Items: []documentDTO{}})
			}
			return httpkit.WriteError(c, err)
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
		return httpkit.WriteError(c, err)
	}
	response := rowsResponse{Rows: []map[string]any{}, Items: []documentDTO{}, RequestID: httpkit.RequestID(c)}
	for i, row := range result.Rows {
		if i == query.Limit {
			response.NextCursor = nextCursorFromRow(result.Rows[i-1])
			break
		}
		dto, err := rowDocument(row, query.Fields)
		if err != nil {
			continue
		}
		dto.Data = collection.HideDocumentFields(dto.Data, hiddenFieldNames(schema))
		response.Items = append(response.Items, dto)
		flat := flattenDocument(dto)
		response.Rows = append(response.Rows, flat)
	}
	if query.Count {
		count, err := lease.Query(c.Request().Context(), godatabase.Statement{SQL: query.CountSQL, Args: query.CountArgs}, 1)
		if err != nil {
			return httpkit.WriteError(c, err)
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
		return httpkit.WriteError(c, err)
	}
	defer lease.Release()
	schema, err := h.schema(c.Request().Context(), name)
	if err != nil {
		return validationAPIError(c, err)
	}
	rule, actor, admin, err := h.ruleScope(c, name)
	if err != nil {
		return h.ruleError(c, err)
	}
	if !rule.CanRead(actor, rule.OwnerID(dto.Data), admin) {
		return forbiddenRow(c)
	}
	dto.Data = collection.HideDocumentFields(dto.Data, hiddenFieldNames(schema))
	return c.JSON(http.StatusOK, dto)
}

func (h *DataHandler) CreateDocument(c echo.Context) error {
	if err := h.writeGuard(c); err != nil {
		return httpkit.WriteError(c, err)
	}
	name, err := collectionParam(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	var doc map[string]any
	if err := decodeDocument(c, &doc); err != nil {
		return httpkit.WriteError(c, err)
	}
	lease, err := h.acquire(c, godatabase.ReadWrite)
	if err != nil {
		return httpkit.WriteError(c, err)
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
	rule, actor, admin, err := h.ruleScope(c, name)
	if err != nil {
		return h.ruleError(c, err)
	}
	rule.StampOwner(doc, actor, admin)
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := h.ensureCollection(c, lease, name); err != nil {
		return httpkit.WriteError(c, err)
	}
	if err := h.checkUnique(c, lease, name, schema, doc, ""); err != nil {
		return validationAPIError(c, err)
	}
	body, _ := json.Marshal(doc)
	_, err = lease.Execute(c.Request().Context(), godatabase.Statement{SQL: "INSERT INTO " + sqlx.Ident(name) + " (id, data, created_at, updated_at) VALUES (?, ?, ?, ?)", Args: []any{id, string(body), now, now}})
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	h.publish(c, collection.RecordEvent{Collection: name, RecordID: id, Action: "create", Data: doc})
	return c.JSON(http.StatusCreated, flattenDocument(documentDTO{ID: id, CreatedAt: now, UpdatedAt: now, Data: collection.HideDocumentFields(doc, hiddenFieldNames(schema))}))
}

func (h *DataHandler) UpdateDocument(c echo.Context) error {
	return h.replaceDocument(c, false)
}

func (h *DataHandler) PatchDocument(c echo.Context) error {
	return h.replaceDocument(c, true)
}

func (h *DataHandler) replaceDocument(c echo.Context, patch bool) error {
	if err := h.writeGuard(c); err != nil {
		return httpkit.WriteError(c, err)
	}
	name, lease, before, err := h.loadDocumentForWrite(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	defer lease.Release()
	var input map[string]any
	if err := decodeDocument(c, &input); err != nil {
		return httpkit.WriteError(c, err)
	}
	schema, err := h.schema(c.Request().Context(), name)
	if err != nil {
		return validationAPIError(c, err)
	}
	rule, actor, admin, err := h.ruleScope(c, name)
	if err != nil {
		return h.ruleError(c, err)
	}
	if !rule.CanWrite(actor, rule.OwnerID(before.Data), admin) {
		return forbiddenRow(c)
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
	rule.PreserveOwner(input, before.Data, admin)
	if err := h.checkUnique(c, lease, name, schema, input, before.ID); err != nil {
		return validationAPIError(c, err)
	}
	body, _ := json.Marshal(input)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = lease.Execute(c.Request().Context(), godatabase.Statement{SQL: "UPDATE " + sqlx.Ident(name) + " SET data = ?, updated_at = ? WHERE id = ?", Args: []any{string(body), now, before.ID}})
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	action := "update"
	h.publish(c, collection.RecordEvent{Collection: name, RecordID: before.ID, Action: action, Changed: collection.ChangedFields(before.Data, input), Data: input})
	return c.JSON(http.StatusOK, flattenDocument(documentDTO{ID: before.ID, CreatedAt: before.CreatedAt, UpdatedAt: now, Data: collection.HideDocumentFields(input, hiddenFieldNames(schema))}))
}

func (h *DataHandler) BatchDocuments(c echo.Context) error {
	if err := h.writeGuard(c); err != nil {
		return httpkit.WriteError(c, err)
	}
	name, err := collectionParam(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	rule, actor, admin, err := h.ruleScope(c, name)
	if err != nil {
		return h.ruleError(c, err)
	}
	var req struct {
		Documents []map[string]any `json:"documents"`
	}
	if err := httpkit.DecodeJSONBody(c, &req, false); err != nil {
		return httpkit.WriteError(c, err)
	}
	if len(req.Documents) == 0 || len(req.Documents) > 100 {
		return httpkit.WriteError(c, httpkit.BadRequest(c, "documents must contain 1..100 items"))
	}
	lease, err := h.acquire(c, godatabase.ReadWrite)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	defer lease.Release()
	schema, err := h.schema(c.Request().Context(), name)
	if err != nil {
		return validationAPIError(c, err)
	}
	if err := h.ensureCollection(c, lease, name); err != nil {
		return httpkit.WriteError(c, err)
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
		rule.StampOwner(doc, actor, admin)
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
		return httpkit.WriteError(c, err)
	}
	for i, dto := range response {
		h.publish(c, collection.RecordEvent{Collection: name, RecordID: dto.ID, Action: "create", Data: dto.Data})
		response[i].Data = collection.HideDocumentFields(dto.Data, hiddenFieldNames(schema))
	}
	return c.JSON(http.StatusCreated, map[string]any{"items": response})
}

func (h *DataHandler) DeleteDocument(c echo.Context) error {
	if err := h.writeGuard(c); err != nil {
		return httpkit.WriteError(c, err)
	}
	name, err := collectionParam(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	id := c.Param("id")
	lease, err := h.acquire(c, godatabase.ReadWrite)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	defer lease.Release()
	rule, actor, admin, err := h.ruleScope(c, name)
	if err != nil {
		return h.ruleError(c, err)
	}
	if rule.RestrictsWrite() && !admin {
		schema, serr := h.schema(c.Request().Context(), name)
		if serr != nil {
			return validationAPIError(c, serr)
		}
		result, qerr := lease.Query(c.Request().Context(), godatabase.Statement{SQL: selectCollectionDocumentsSQL(name, schema) + " WHERE id = ?", Args: []any{id}}, 2)
		if qerr != nil {
			if isMissingRelation(qerr) {
				return c.NoContent(http.StatusNoContent)
			}
			return httpkit.WriteError(c, qerr)
		}
		if len(result.Rows) > 0 {
			dto, derr := rowDocument(result.Rows[0], nil)
			if derr != nil {
				return httpkit.WriteError(c, derr)
			}
			if !rule.CanWrite(actor, rule.OwnerID(dto.Data), admin) {
				return forbiddenRow(c)
			}
		}
	}
	_, err = lease.Execute(c.Request().Context(), godatabase.Statement{SQL: "DELETE FROM " + sqlx.Ident(name) + " WHERE id = ?", Args: []any{id}})
	if err != nil && !isMissingRelation(err) {
		return httpkit.WriteError(c, err)
	}
	h.publish(c, collection.RecordEvent{Collection: name, RecordID: id, Action: "delete"})
	return c.NoContent(http.StatusNoContent)
}

func (h *DataHandler) TruncateCollection(c echo.Context) error {
	if err := h.writeGuard(c); err != nil {
		return httpkit.WriteError(c, err)
	}
	name, err := collectionParam(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	lease, err := h.acquire(c, godatabase.ReadWrite)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	defer lease.Release()
	if _, err := lease.Execute(c.Request().Context(), godatabase.Statement{SQL: "DELETE FROM " + sqlx.Ident(name)}); err != nil && !isMissingRelation(err) {
		return httpkit.WriteError(c, err)
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
	schema, err := h.schemas.Get(ctx, httpkit.ProjectIDFrom(ctx), name)
	if errors.Is(err, collection.ErrNotFound) || errors.Is(err, collection.ErrUnavailable) {
		return nil, nil
	}
	if err != nil || schema == nil {
		return schema, err
	}
	principal, _ := httpkit.PrincipalFromContext(ctx)
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
		return "", nil, documentDTO{}, httpkit.BadRequest(c, "document id is required")
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
			return "", nil, documentDTO{}, httpkit.NewAPIError(http.StatusNotFound, "not_found", "document not found", httpkit.RequestID(c))
		}
		return "", nil, documentDTO{}, err
	}
	if len(result.Rows) == 0 {
		lease.Release()
		return "", nil, documentDTO{}, httpkit.NewAPIError(http.StatusNotFound, "not_found", "document not found", httpkit.RequestID(c))
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
	principal, ok := httpkit.PrincipalFromContext(c.Request().Context())
	if !ok {
		return catalog.Database{}, nil, auth.ErrMissingCredentials
	}
	project, err := httpkit.RequireProject(c)
	if err != nil {
		return catalog.Database{}, nil, err
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
		return catalog.Database{}, nil, httpkit.NewAPIError(http.StatusBadRequest, "data_model_mismatch", "SQL 数据库不支持集合与文档", httpkit.RequestID(c))
	}
	if mode == godatabase.ReadWrite && c.Param("collection") != "" {
		schema, schemaErr := h.schema(c.Request().Context(), c.Param("collection"))
		if schemaErr != nil {
			return catalog.Database{}, nil, schemaErr
		}
		if schema != nil && schema.SchemaMode == "view" {
			return catalog.Database{}, nil, httpkit.NewAPIError(http.StatusConflict, "collection_read_only", "view collections are read-only", httpkit.RequestID(c))
		}
	}
	lease, err := h.svc.Acquire(c.Request().Context(), db, mode)
	return db, lease, err
}

func (h *DataHandler) systemGuard(c echo.Context) error {
	principal, ok := httpkit.PrincipalFromContext(c.Request().Context())
	if !ok {
		return auth.ErrMissingCredentials
	}
	project, err := httpkit.RequireProject(c)
	if err != nil {
		return err
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

func (h *DataHandler) publish(c echo.Context, event collection.RecordEvent) {
	if h.events != nil {
		event.ProjectID = httpkit.ProjectID(c)
		h.events.PublishRecord(event)
	}
}

func collectionParam(c echo.Context) (string, error) {
	name := c.Param("collection")
	if !collection.NamePattern.MatchString(name) {
		return "", httpkit.BadRequest(c, "invalid collection name")
	}
	return name, nil
}

func createCollectionSQL(name string) string {
	return "CREATE TABLE IF NOT EXISTS " + sqlx.Ident(name) + " (id VARCHAR NOT NULL, data VARCHAR NOT NULL, created_at VARCHAR NOT NULL, updated_at VARCHAR)"
}
