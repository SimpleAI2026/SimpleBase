package datahttp

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlx"
	"github.com/linkxzhou/SimpleBase/internal/collection"
	"github.com/linkxzhou/SimpleBase/internal/collection/query"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

type collectionQuery struct {
	Limit      int
	Offset     int
	Count      bool
	Fields     []string
	SQL        string
	Args       []any
	CountSQL   string
	CountArgs  []any
	ScanSQL    string
	CustomSort bool
	Cursor     *pageCursor
}

type pageCursor struct {
	CreatedAt string `json:"c"`
	ID        string `json:"i"`
}

func parseCollectionQuery(c echo.Context, table string, schema *collection.Collection, dialect godatabase.Dialect, scanMax int, extraWhere string, extraArgs []any) (collectionQuery, error) {
	q := collectionQuery{Limit: 50}
	source := collectionSource(table, schema)
	var err error
	if raw := c.QueryParam("limit"); raw != "" {
		q.Limit, err = strconv.Atoi(raw)
		if err != nil || q.Limit < 1 || q.Limit > 200 {
			return q, collection.InvalidQuery("limit must be between 1 and 200")
		}
	}
	if raw := c.QueryParam("offset"); raw != "" {
		q.Offset, err = strconv.Atoi(raw)
		if err != nil || q.Offset < 0 || q.Offset > 100000 {
			return q, collection.InvalidQuery("offset must be between 0 and 100000")
		}
	}
	q.Count = c.QueryParam("count") == "1" || c.QueryParam("count") == "true"
	q.Fields, err = collection.ParseFields(c.QueryParam("fields"), schema)
	if err != nil {
		return q, err
	}
	filterSQL, filterArgs, err := query.CompileFilter(c.QueryParam("filter"), schema, dialect)
	if err != nil {
		return q, err
	}
	where := ""
	args := append([]any(nil), filterArgs...)
	if filterSQL != "" {
		where = " WHERE " + filterSQL
	}
	if extraWhere != "" {
		if where == "" {
			where = " WHERE " + extraWhere
		} else {
			where += " AND " + extraWhere
		}
		args = append(args, extraArgs...)
	}
	if where != "" {
		q.ScanSQL = "SELECT COUNT(*) FROM " + source
	}
	if q.Count {
		q.CountSQL = "SELECT COUNT(*) FROM " + source + where
		q.CountArgs = append([]any(nil), args...)
	}
	order := dialect.NullsFirst("created_at", true) + ", id DESC"
	if raw := c.QueryParam("sort"); raw != "" {
		q.CustomSort = true
		order, err = compileSort(raw, schema, dialect)
		if err != nil {
			return q, err
		}
	}
	if raw := c.QueryParam("cursor"); raw != "" {
		if c.QueryParam("offset") != "" || q.CustomSort {
			return q, collection.InvalidQuery("cursor requires default sort and no offset")
		}
		cursor, err := decodeCursor(raw)
		if err != nil {
			return q, err
		}
		q.Cursor = &cursor
		condition := "(created_at < ? OR (created_at = ? AND id < ?))"
		if where == "" {
			where = " WHERE " + condition
		} else {
			where += " AND " + condition
		}
		args = append(args, cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
	}
	q.SQL = selectDocumentsFromSource(source) + where + " ORDER BY " + order + " LIMIT " + strconv.Itoa(q.Limit+1)
	q.Args = args
	if q.Offset > 0 {
		q.SQL += " OFFSET " + strconv.Itoa(q.Offset)
	}
	return q, nil
}

func collectionSource(table string, schema *collection.Collection) string {
	if schema != nil && schema.SchemaMode == "view" {
		return "(" + strings.TrimSuffix(strings.TrimSpace(schema.ViewQuery), ";") + ") AS sb_view"
	}
	return sqlx.Ident(table)
}

func selectCollectionDocumentsSQL(table string, schema *collection.Collection) string {
	return selectDocumentsFromSource(collectionSource(table, schema))
}

func selectDocumentsFromSource(source string) string {
	return "SELECT id, data, created_at, COALESCE(updated_at, created_at) FROM " + source
}

func compileSort(raw string, schema *collection.Collection, dialect godatabase.Dialect) (string, error) {
	parts := make([]string, 0)
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		descending := strings.HasPrefix(item, "-")
		name := strings.TrimPrefix(item, "-")
		expr := ""
		switch name {
		case "id", "created_at", "updated_at":
			expr = name
		default:
			if !collection.FieldNamePattern.MatchString(name) {
				return "", collection.InvalidQuery("invalid sort field")
			}
			if schema == nil {
				return "", &collection.ValidationError{Code: "schema_not_registered", Field: name, Message: "schema required for sort"}
			}
			found := false
			for _, f := range schema.Fields {
				if f.Name != name {
					continue
				}
				found = true
				path := "$." + name
				switch f.Type {
				case "number":
					expr = dialect.JSONExtractNumber(path)
				case "bool":
					expr = dialect.JSONExtractBool(path)
				case "date":
					expr = dialect.CastTimestamp(dialect.JSONExtractText(path))
				case "json":
					return "", collection.InvalidQuery("json fields cannot be sorted")
				default:
					expr = dialect.JSONExtractText(path)
				}
			}
			if !found {
				return "", &collection.ValidationError{Code: "unknown_field", Field: name, Message: "field is not registered"}
			}
		}
		parts = append(parts, dialect.NullsFirst(expr, descending))
	}
	parts = append(parts, "id ASC")
	return strings.Join(parts, ", "), nil
}

func decodeCursor(raw string) (pageCursor, error) {
	var cursor pageCursor
	body, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || json.Unmarshal(body, &cursor) != nil || cursor.ID == "" {
		return cursor, collection.InvalidQuery("invalid cursor")
	}
	if _, err := time.Parse(time.RFC3339Nano, cursor.CreatedAt); err != nil {
		return cursor, collection.InvalidQuery("invalid cursor")
	}
	return cursor, nil
}

func encodeCursor(createdAt, id string) string {
	body, _ := json.Marshal(pageCursor{CreatedAt: createdAt, ID: id})
	return base64.RawURLEncoding.EncodeToString(body)
}

func validationAPIError(c echo.Context, err error) error {
	if validation, ok := err.(*collection.ValidationError); ok {
		status := http.StatusBadRequest
		if validation.Code == "duplicate_value" {
			status = http.StatusConflict
		}
		apiErr := httpkit.NewAPIError(status, validation.Code, validation.Message, httpkit.RequestID(c))
		apiErr.Body.Error.Field = validation.Field
		apiErr.Body.Error.FailedIndex = validation.Index
		return httpkit.WriteError(c, apiErr)
	}
	return httpkit.WriteError(c, err)
}

func scanLimitError(c echo.Context) error {
	return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusBadRequest, "filter_scan_limit_exceeded", "filter scan limit exceeded", httpkit.RequestID(c)))
}
