package datahttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/collection"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

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
	dto.Data = collection.PickDocument(dto.Data, fieldsWithoutSystem(fields))
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

func decodeDocument(c echo.Context, doc *map[string]any) error {
	if err := json.NewDecoder(c.Request().Body).Decode(doc); err != nil || *doc == nil {
		return httpkit.BadRequest(c, "document must be a JSON object")
	}
	delete(*doc, "id")
	delete(*doc, "created_at")
	delete(*doc, "updated_at")
	principal := httpkit.PrincipalOf(c)
	if principal.Scope != nil {
		for _, name := range principal.Scope.ReadonlyFields {
			if _, ok := (*doc)[name]; ok {
				return httpkit.NewAPIError(http.StatusForbidden, "key_scope_denied", "key scope denied", httpkit.RequestID(c))
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
