package datahttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlguard"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlx"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/collection"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

type collectionRepository interface {
	collectionReader
	Put(ctx context.Context, schema collection.Collection) error
	Delete(ctx context.Context, projectID, name string) error
}

type CollectionMetaHandler struct {
	svc       DataService
	schemas   collectionRepository
	writable  bool
	scanLimit int
}

func NewCollectionMetaHandler(svc DataService, schemas collectionRepository, writable bool, scanLimit int) *CollectionMetaHandler {
	return &CollectionMetaHandler{svc: svc, schemas: schemas, writable: writable, scanLimit: scanLimit}
}

func (h *CollectionMetaHandler) Get(c echo.Context) error {
	name, err := collectionParam(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	schema, err := h.schemas.Get(c.Request().Context(), httpkit.ProjectID(c), name)
	if err != nil {
		return collectionMetaError(c, err)
	}
	return c.JSON(http.StatusOK, schema)
}

func (h *CollectionMetaHandler) Put(c echo.Context) error {
	if err := (&DataHandler{svc: h.svc, writable: h.writable}).writeGuard(c); err != nil {
		return httpkit.WriteError(c, err)
	}
	name, err := collectionParam(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	var schema collection.Collection
	if err := httpkit.DecodeJSONBody(c, &schema, false); err != nil {
		return httpkit.WriteError(c, err)
	}
	schema.Name, schema.ProjectID = name, httpkit.ProjectID(c)
	if schema.SchemaMode == "" {
		schema.SchemaMode = "json"
	}
	if err := collection.ValidateSchema(schema); err != nil {
		return validationAPIError(c, err)
	}
	dryRun := c.QueryParam("dry_run") == "1" || c.QueryParam("dry_run") == "true"
	if schema.SchemaMode == "view" {
		principal := httpkit.PrincipalOf(c)
		if !principal.HasPermission(auth.ProjectAdmin) {
			return httpkit.WriteError(c, auth.ErrForbidden)
		}
		preview, previewErr := h.previewView(c, schema.ViewQuery)
		if previewErr != nil {
			return httpkit.WriteError(c, previewErr)
		}
		if dryRun {
			return c.JSON(http.StatusOK, preview)
		}
		if err := h.schemas.Put(c.Request().Context(), schema); err != nil {
			return collectionMetaError(c, err)
		}
		return c.JSON(http.StatusOK, schema)
	}
	if dryRun {
		return h.dryRun(c, name, &schema)
	}
	lease, err := h.svcFor(c, godatabase.ReadWrite)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	defer lease.Release()
	if _, err := lease.Execute(c.Request().Context(), godatabase.Statement{SQL: createCollectionSQL(name)}); err != nil {
		return httpkit.WriteError(c, err)
	}
	if err := h.schemas.Put(c.Request().Context(), schema); err != nil {
		return collectionMetaError(c, err)
	}
	return c.JSON(http.StatusOK, schema)
}

func (h *CollectionMetaHandler) Delete(c echo.Context) error {
	if err := (&DataHandler{svc: h.svc, writable: h.writable}).writeGuard(c); err != nil {
		return httpkit.WriteError(c, err)
	}
	name, err := collectionParam(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if err := h.schemas.Delete(c.Request().Context(), httpkit.ProjectID(c), name); err != nil {
		return collectionMetaError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

type viewPreviewResult struct {
	Columns []string           `json:"columns"`
	Fields  []collection.Field `json:"fields"`
	Items   []documentDTO      `json:"items"`
	Scanned int                `json:"scanned"`
}

func (h *CollectionMetaHandler) DryRunView(c echo.Context) error {
	principal := httpkit.PrincipalOf(c)
	if !principal.HasPermission(auth.ProjectAdmin) {
		return httpkit.WriteError(c, auth.ErrForbidden)
	}
	var request struct {
		Name  string `json:"name"`
		Query string `json:"query"`
	}
	if err := httpkit.DecodeJSONBody(c, &request, false); err != nil {
		return httpkit.WriteError(c, err)
	}
	if !collection.NamePattern.MatchString(request.Name) {
		return httpkit.WriteError(c, httpkit.BadRequest(c, "invalid collection name"))
	}
	preview, err := h.previewView(c, request.Query)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, preview)
}

func (h *CollectionMetaHandler) previewView(c echo.Context, query string) (viewPreviewResult, error) {
	result := viewPreviewResult{Columns: []string{}, Fields: []collection.Field{}, Items: []documentDTO{}}
	invalid := func() (viewPreviewResult, error) {
		return result, httpkit.NewAPIError(http.StatusBadRequest, "invalid_view_query", "view query must return id, JSON data, created_at and updated_at", httpkit.RequestID(c))
	}
	query = strings.TrimSuffix(strings.TrimSpace(query), ";")
	keyword := sqlguard.FirstKeyword(query)
	if keyword != "SELECT" && keyword != "WITH" || sqlguard.Validate(query, sqlguard.ReadOnly) != nil {
		return invalid()
	}
	lease, err := h.svcFor(c, godatabase.ReadOnly)
	if err != nil {
		return result, err
	}
	defer lease.Release()
	rows, err := lease.Query(c.Request().Context(), godatabase.Statement{SQL: "SELECT id, data, created_at, COALESCE(updated_at, created_at) FROM (" + query + ") AS sb_view LIMIT 20"}, 20)
	if err != nil {
		return invalid()
	}
	result.Columns = rows.Columns
	fieldTypes := map[string]string{}
	for _, row := range rows.Rows {
		if len(row) != 4 {
			return invalid()
		}
		id, idOK := row[0].(string)
		raw, dataOK := row[1].(string)
		created, createdOK := row[2].(string)
		var data map[string]any
		if !idOK || id == "" || !dataOK || !createdOK || json.Unmarshal([]byte(raw), &data) != nil || data == nil {
			return invalid()
		}
		if created != "" {
			if _, err := time.Parse(time.RFC3339Nano, created); err != nil {
				return invalid()
			}
		}
		dto, err := rowDocument(row, nil)
		if err != nil {
			return invalid()
		}
		result.Items = append(result.Items, dto)
		for name, value := range data {
			if !collection.FieldNamePattern.MatchString(name) || name == "id" || name == "created_at" || name == "updated_at" {
				return invalid()
			}
			if _, exists := fieldTypes[name]; !exists {
				fieldTypes[name] = inferViewFieldType(value)
			}
		}
	}
	result.Scanned = len(result.Items)
	names := make([]string, 0, len(fieldTypes))
	for name := range fieldTypes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result.Fields = append(result.Fields, collection.Field{Name: name, Type: fieldTypes[name]})
	}
	return result, nil
}

func inferViewFieldType(value any) string {
	switch value.(type) {
	case float64:
		return "number"
	case bool:
		return "bool"
	case map[string]any, []any:
		return "json"
	default:
		return "text"
	}
}

func (h *CollectionMetaHandler) dryRun(c echo.Context, name string, schema *collection.Collection) error {
	lease, err := h.svcFor(c, godatabase.ReadOnly)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	defer lease.Release()
	result, err := lease.Query(c.Request().Context(), godatabase.Statement{SQL: "SELECT id, data FROM " + sqlx.Ident(name) + " LIMIT " + strconv.Itoa(h.scanLimit+1)}, h.scanLimit+1)
	if err != nil && !isMissingRelation(err) {
		return httpkit.WriteError(c, err)
	}
	response := struct {
		Invalid   []map[string]any `json:"invalid"`
		Scanned   int              `json:"scanned"`
		Truncated bool             `json:"truncated"`
	}{Invalid: []map[string]any{}}
	for i, row := range result.Rows {
		if i >= h.scanLimit {
			response.Truncated = true
			break
		}
		response.Scanned++
		if len(row) < 2 {
			continue
		}
		id, _ := row[0].(string)
		raw, _ := row[1].(string)
		var doc map[string]any
		if json.Unmarshal([]byte(raw), &doc) != nil {
			response.Invalid = append(response.Invalid, map[string]any{"id": id, "field": "", "reason": "invalid_json"})
			continue
		}
		if _, validation := collection.ValidateStoredDocument(schema, doc, false); validation != nil {
			response.Invalid = append(response.Invalid, map[string]any{"id": id, "field": validation.Field, "reason": validation.Code})
		}
	}
	return c.JSON(http.StatusOK, response)
}

func (h *CollectionMetaHandler) svcFor(c echo.Context, mode godatabase.AccessMode) (SQLLease, error) {
	return (&DataHandler{svc: h.svc}).acquire(c, mode)
}

func collectionMetaError(c echo.Context, err error) error {
	if errors.Is(err, collection.ErrNotFound) {
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusNotFound, "schema_not_registered", "schema is not registered", httpkit.RequestID(c)))
	}
	if errors.Is(err, collection.ErrUnavailable) {
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusServiceUnavailable, "collection_schema_unavailable", "collection schema migration is not applied", httpkit.RequestID(c)))
	}
	return validationAPIError(c, err)
}
