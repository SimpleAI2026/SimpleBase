// 文件说明：collection_transfer.go — internal/api 中 Import、Export 等符号的实现
//
// 用法
//
//	(h *DataHandler) Import(c echo.Context) error
//	(h *DataHandler) Export(c echo.Context) error
//	同包调用：Next() → adapter.go, adapter.go 等；Export() → router.go, system_handlers.go；Import() → router.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/collection_transfer.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase, godatabase/sqlx, internal/catalog, internal/collection
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] Next() ← adapter.go, api_keys_handler.go
//
// 主要符号
//
//	(h *DataHandler) Import(c echo.Context) error
//	(h *DataHandler) Export(c echo.Context) error
//	... 另有 15 个顶层符号

package api

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlx"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/collection"
)

type importRecord struct {
	id    string
	now   string
	doc   map[string]any
	index int
}

type importDecoder interface {
	Next() (map[string]any, int, error)
}

type ndjsonDecoder struct {
	scanner *bufio.Scanner
	index   int
}

func (d *ndjsonDecoder) Next() (map[string]any, int, error) {
	for d.scanner.Scan() {
		index := d.index
		d.index++
		line := strings.TrimSpace(d.scanner.Text())
		if line == "" {
			continue
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(line), &doc); err != nil || doc == nil {
			return nil, index, &collection.ValidationError{Code: "invalid_document", Message: "each NDJSON line must be a JSON object", Index: &index}
		}
		return doc, index, nil
	}
	if err := d.scanner.Err(); err != nil {
		return nil, d.index, err
	}
	return nil, d.index, io.EOF
}

type csvDecoder struct {
	reader  *csv.Reader
	header  []string
	schema  *collection.Collection
	index   int
	started bool
}

func (d *csvDecoder) Next() (map[string]any, int, error) {
	if !d.started {
		d.started = true
		header, err := d.reader.Read()
		if err != nil {
			return nil, 0, err
		}
		seen := map[string]struct{}{}
		for _, name := range header {
			if !fieldNamePattern.MatchString(name) && name != "id" && name != "created_at" && name != "updated_at" {
				return nil, 0, &collection.ValidationError{Code: "invalid_field", Field: name, Message: "invalid CSV header"}
			}
			if _, ok := seen[name]; ok {
				return nil, 0, &collection.ValidationError{Code: "invalid_field", Field: name, Message: "duplicate CSV header"}
			}
			seen[name] = struct{}{}
		}
		d.header = header
	}
	row, err := d.reader.Read()
	if err != nil {
		return nil, d.index, err
	}
	index := d.index
	d.index++
	if len(row) != len(d.header) {
		return nil, index, &collection.ValidationError{Code: "invalid_document", Message: "CSV column count mismatch", Index: &index}
	}
	doc := map[string]any{}
	for i, name := range d.header {
		if row[i] == "" || name == "id" || name == "created_at" || name == "updated_at" {
			continue
		}
		value, err := csvValue(row[i], schemaField(d.schema, name))
		if err != nil {
			return nil, index, &collection.ValidationError{Code: "field_type_mismatch", Field: name, Message: "invalid CSV value", Index: &index}
		}
		doc[name] = value
	}
	return doc, index, nil
}

func schemaField(schema *collection.Collection, name string) *collection.Field {
	if schema == nil {
		return nil
	}
	for i := range schema.Fields {
		if schema.Fields[i].Name == name {
			return &schema.Fields[i]
		}
	}
	return nil
}

func csvValue(raw string, field *collection.Field) (any, error) {
	if field == nil {
		return raw, nil
	}
	switch field.Type {
	case "number":
		return strconv.ParseFloat(raw, 64)
	case "bool":
		return strconv.ParseBool(raw)
	case "json":
		var value any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return nil, err
		}
		return value, nil
	case "select":
		if strings.HasPrefix(raw, "[") {
			var value []string
			if err := json.Unmarshal([]byte(raw), &value); err != nil {
				return nil, err
			}
			return value, nil
		}
	}
	return raw, nil
}

func newImportDecoder(contentType string, body io.Reader, schema *collection.Collection, limit int64) (importDecoder, error) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = contentType
	}
	switch mediaType {
	case "application/x-ndjson", "application/ndjson":
		scanner := bufio.NewScanner(body)
		maxToken := int(limit)
		if limit <= 0 || int64(maxToken) != limit {
			maxToken = 128 << 20
		}
		scanner.Buffer(make([]byte, 64*1024), maxToken)
		return &ndjsonDecoder{scanner: scanner}, nil
	case "text/csv":
		reader := csv.NewReader(body)
		reader.FieldsPerRecord = -1
		return &csvDecoder{reader: reader, schema: schema}, nil
	default:
		return nil, &collection.ValidationError{Code: "unsupported_media_type", Message: "use application/x-ndjson or text/csv"}
	}
}

func (h *DataHandler) Import(c echo.Context) error {
	if err := h.writeGuard(c); err != nil {
		return WriteError(c, err)
	}
	name, err := collectionParam(c)
	if err != nil {
		return WriteError(c, err)
	}
	schema, err := h.schema(c.Request().Context(), name)
	if err != nil {
		return validationAPIError(c, err)
	}
	if schema != nil && schema.SchemaMode == "view" {
		return WriteError(c, NewAPIError(http.StatusConflict, "collection_read_only", "view collections are read-only", RequestIDFromContext(c.Request().Context())))
	}
	decoder, err := newImportDecoder(c.Request().Header.Get(echo.HeaderContentType), c.Request().Body, schema, h.importLimit)
	if err != nil {
		return importAPIError(c, err)
	}
	dryRun := c.QueryParam("dry_run") == "1" || c.QueryParam("dry_run") == "true"
	lease, err := h.acquire(c, godatabase.ReadWrite)
	if err != nil {
		return WriteError(c, err)
	}
	defer lease.Release()
	var tx *sql.Tx
	if !dryRun {
		if err := h.ensureCollection(c, lease, name); err != nil {
			return WriteError(c, err)
		}
		tx, err = lease.Raw().BeginTx(c.Request().Context(), nil)
		if err != nil {
			return WriteError(c, err)
		}
		defer tx.Rollback()
	}
	unique := map[string]map[string]int{}
	result := struct {
		DryRun   bool `json:"dry_run"`
		Scanned  int  `json:"scanned"`
		Imported int  `json:"imported"`
	}{DryRun: dryRun}
	buffer := make([]importRecord, 0, 100)
	committed := make([]importRecord, 0)
	flush := func() error {
		if len(buffer) == 0 {
			return nil
		}
		for _, record := range buffer {
			if err := checkImportUnique(c.Request().Context(), lease, tx, name, schema, record.doc, unique, record.index); err != nil {
				return err
			}
			if dryRun {
				continue
			}
			body, _ := json.Marshal(record.doc)
			if _, err := tx.ExecContext(c.Request().Context(), "INSERT INTO "+sqlx.Ident(name)+" (id, data, created_at, updated_at) VALUES (?, ?, ?, ?)", record.id, string(body), record.now, record.now); err != nil {
				return err
			}
		}
		if !dryRun {
			result.Imported += len(buffer)
			committed = append(committed, buffer...)
		}
		buffer = buffer[:0]
		return nil
	}
	for {
		doc, index, nextErr := decoder.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return importAPIError(c, nextErr)
		}
		result.Scanned++
		delete(doc, "id")
		delete(doc, "created_at")
		delete(doc, "updated_at")
		if validation := protectReadonlyFields(schema, doc, nil); validation != nil {
			validation.Index = &index
			return validationAPIError(c, validation)
		}
		validated, validation := collection.ValidateDocument(schema, doc, true)
		if validation != nil {
			validation.Index = &index
			return validationAPIError(c, validation)
		}
		buffer = append(buffer, importRecord{id: uuid.NewString(), now: time.Now().UTC().Format(time.RFC3339Nano), doc: validated, index: index})
		if len(buffer) == 100 {
			if err := flush(); err != nil {
				return importAPIError(c, err)
			}
		}
	}
	if err := flush(); err != nil {
		return importAPIError(c, err)
	}
	if dryRun {
		return c.JSON(http.StatusOK, result)
	}
	if err := tx.Commit(); err != nil {
		return WriteError(c, err)
	}
	lease.NotifyWrite(c.Request().Context())
	for _, record := range committed {
		h.publish(c, RecordEvent{Collection: name, RecordID: record.id, Action: "create", Data: record.doc})
	}
	return c.JSON(http.StatusCreated, result)
}

func checkImportUnique(ctx context.Context, lease SQLLease, tx *sql.Tx, table string, schema *collection.Collection, doc map[string]any, seen map[string]map[string]int, index int) error {
	if schema == nil {
		return nil
	}
	for _, field := range schema.Fields {
		if !field.Unique || doc[field.Name] == nil {
			continue
		}
		encoded, _ := json.Marshal(doc[field.Name])
		key := string(encoded)
		if seen[field.Name] == nil {
			seen[field.Name] = map[string]int{}
		}
		if _, exists := seen[field.Name][key]; exists {
			return &collection.ValidationError{Code: "duplicate_value", Field: field.Name, Message: "duplicate value", Index: &index}
		}
		seen[field.Name][key] = index
		query := "SELECT id FROM " + sqlx.Ident(table) + " WHERE json_extract(data, '$." + field.Name + "') = json_extract(?, '$') LIMIT 1"
		var id string
		var err error
		if tx != nil {
			err = tx.QueryRowContext(ctx, query, key).Scan(&id)
		} else {
			result, queryErr := lease.Query(ctx, godatabase.Statement{SQL: query, Args: []any{key}}, 1)
			err = queryErr
			if len(result.Rows) > 0 {
				err = nil
				id = "duplicate"
			} else {
				id = ""
			}
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) && !isMissingRelation(err) {
			return err
		}
		if id != "" {
			return &collection.ValidationError{Code: "duplicate_value", Field: field.Name, Message: "duplicate value", Index: &index}
		}
	}
	return nil
}

func importAPIError(c echo.Context, err error) error {
	var validation *collection.ValidationError
	if errors.As(err, &validation) {
		if validation.Code == "unsupported_media_type" {
			return WriteError(c, NewAPIError(http.StatusUnsupportedMediaType, validation.Code, validation.Message, RequestIDFromContext(c.Request().Context())))
		}
		return validationAPIError(c, validation)
	}
	var parseError *csv.ParseError
	if errors.As(err, &parseError) {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_document", "invalid CSV input", RequestIDFromContext(c.Request().Context())))
	}
	return WriteError(c, err)
}

func (h *DataHandler) Export(c echo.Context) error {
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
	filterSQL, args, err := compileFilter(c.QueryParam("filter"), schema, dialect)
	if err != nil {
		return validationAPIError(c, err)
	}
	where := ""
	if filterSQL != "" {
		where = " WHERE " + filterSQL
	}
	order := dialect.NullsFirst("created_at", true) + ", id DESC"
	if raw := c.QueryParam("sort"); raw != "" {
		order, err = compileSort(raw, schema, dialect)
		if err != nil {
			return validationAPIError(c, err)
		}
	}
	fields, err := parseFields(c.QueryParam("fields"), schema)
	if err != nil {
		return validationAPIError(c, err)
	}
	format := c.QueryParam("format")
	if format == "" {
		format = "ndjson"
	}
	if format != "ndjson" && format != "csv" && format != "json" {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "format must be ndjson, csv or json", RequestIDFromContext(c.Request().Context())))
	}
	if format == "csv" && schema == nil && len(fields) == 0 {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "fields is required for CSV export without a schema", RequestIDFromContext(c.Request().Context())))
	}
	rows, err := lease.Raw().QueryContext(c.Request().Context(), selectCollectionDocumentsSQL(name, schema)+where+" ORDER BY "+order, args...)
	if err != nil {
		if isMissingRelation(err) {
			rows = nil
		} else {
			return WriteError(c, err)
		}
	}
	if rows != nil {
		defer rows.Close()
	}
	contentType := "application/x-ndjson"
	if format == "csv" {
		contentType = "text/csv; charset=utf-8"
	} else if format == "json" {
		contentType = "application/json; charset=utf-8"
	}
	c.Response().Header().Set(echo.HeaderContentType, contentType)
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.%s\"", name, format))
	c.Response().WriteHeader(http.StatusOK)
	writer := bufio.NewWriter(c.Response())
	defer writer.Flush()
	if format == "csv" {
		return writeCSVExport(c, writer, rows, fields, schema)
	}
	if format == "json" {
		return writeJSONExport(c, writer, rows, fields, schema)
	}
	return writeNDJSONExport(c, writer, rows, fields, schema)
}

func scanExportRow(rows *sql.Rows) ([]any, error) {
	values := make([]any, 4)
	pointers := make([]any, 4)
	for i := range values {
		pointers[i] = &values[i]
	}
	if err := rows.Scan(pointers...); err != nil {
		return nil, err
	}
	for i, value := range values {
		serialized, err := godatabase.SerializeValue(value)
		if err != nil {
			return nil, err
		}
		values[i] = serialized
	}
	return values, nil
}

func writeNDJSONExport(c echo.Context, writer *bufio.Writer, rows *sql.Rows, fields []string, schema *collection.Collection) error {
	encoder := json.NewEncoder(writer)
	for rows != nil && rows.Next() {
		values, err := scanExportRow(rows)
		if err != nil {
			return nil
		}
		dto, err := rowDocument(values, fields)
		if err != nil {
			return nil
		}
		dto.Data = hideDocumentFields(dto.Data, hiddenFieldNames(schema))
		if err := encoder.Encode(flattenDocument(dto)); err != nil {
			return nil
		}
		writer.Flush()
		c.Response().Flush()
	}
	return nil
}

func writeJSONExport(c echo.Context, writer *bufio.Writer, rows *sql.Rows, fields []string, schema *collection.Collection) error {
	if _, err := writer.WriteString("["); err != nil {
		return nil
	}
	first := true
	for rows != nil && rows.Next() {
		values, err := scanExportRow(rows)
		if err != nil {
			return nil
		}
		dto, err := rowDocument(values, fields)
		if err != nil {
			return nil
		}
		dto.Data = hideDocumentFields(dto.Data, hiddenFieldNames(schema))
		encoded, err := json.Marshal(flattenDocument(dto))
		if err != nil {
			return nil
		}
		if !first {
			if _, err := writer.WriteString(","); err != nil {
				return nil
			}
		}
		if _, err := writer.Write(encoded); err != nil {
			return nil
		}
		first = false
		writer.Flush()
		c.Response().Flush()
	}
	_, _ = writer.WriteString("]")
	return nil
}

func writeCSVExport(c echo.Context, writer *bufio.Writer, rows *sql.Rows, fields []string, schema *collection.Collection) error {
	csvWriter := csv.NewWriter(writer)
	columns := append([]string(nil), fields...)
	if len(columns) == 0 {
		columns = []string{"id", "created_at", "updated_at"}
		for _, field := range schema.Fields {
			if !field.Hidden {
				columns = append(columns, field.Name)
			}
		}
	}
	if err := csvWriter.Write(columns); err != nil {
		return nil
	}
	for rows != nil && rows.Next() {
		values, err := scanExportRow(rows)
		if err != nil {
			return nil
		}
		dto, err := rowDocument(values, fieldsWithoutSystem(columns))
		if err != nil {
			return nil
		}
		dto.Data = hideDocumentFields(dto.Data, hiddenFieldNames(schema))
		flat := flattenDocument(dto)
		record := make([]string, len(columns))
		for i, column := range columns {
			value, exists := flat[column]
			if !exists || value == nil {
				continue
			}
			switch typed := value.(type) {
			case string:
				record[i] = typed
			case json.Number:
				record[i] = typed.String()
			case bool:
				record[i] = strconv.FormatBool(typed)
			default:
				encoded, _ := json.Marshal(typed)
				record[i] = string(encoded)
			}
		}
		if err := csvWriter.Write(record); err != nil {
			return nil
		}
		csvWriter.Flush()
		writer.Flush()
		c.Response().Flush()
	}
	return nil
}
