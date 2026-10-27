package gosdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var collectionNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}$`)

type Document map[string]any

type CollectionList struct {
	Collections []string         `json:"collections"`
	Items       []CollectionInfo `json:"items"`
}

type CollectionInfo struct {
	Name          string `json:"name"`
	FieldCount    int    `json:"field_count"`
	DocumentCount int64  `json:"document_count"`
	Registered    bool   `json:"registered"`
}

type DocumentList struct {
	Rows       []Document       `json:"rows"`
	Items      []DocumentRecord `json:"items"`
	NextCursor string           `json:"next_cursor,omitempty"`
	Total      *int64           `json:"total,omitempty"`
	RequestID  string           `json:"request_id,omitempty"`
}

type DocumentRecord struct {
	ID        string   `json:"id"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
	Data      Document `json:"data"`
}

type DocumentQuery struct {
	Limit  int
	Cursor string
	Offset int
	Sort   string
	Filter string
	Fields []string
	Count  bool
}

type CollectionSchema struct {
	ID         string            `json:"id,omitempty"`
	ProjectID  string            `json:"project_id,omitempty"`
	Name       string            `json:"name"`
	SchemaMode string            `json:"schema_mode,omitempty"`
	ViewQuery  string            `json:"view_query,omitempty"`
	Status     string            `json:"status,omitempty"`
	Fields     []CollectionField `json:"fields"`
}

type CollectionField struct {
	Name       string       `json:"name"`
	Type       string       `json:"type"`
	Required   bool         `json:"required,omitempty"`
	Unique     bool         `json:"unique,omitempty"`
	Default    any          `json:"default,omitempty"`
	Options    FieldOptions `json:"options"`
	Hidden     bool         `json:"hidden,omitempty"`
	Readonly   bool         `json:"readonly,omitempty"`
	OrderIndex int          `json:"order_index,omitempty"`
}

type FieldOptions struct {
	Min           any      `json:"min,omitempty"`
	Max           any      `json:"max,omitempty"`
	Pattern       string   `json:"pattern,omitempty"`
	Values        []string `json:"values,omitempty"`
	MaxSelect     int      `json:"max_select,omitempty"`
	MaxSize       int      `json:"max_size,omitempty"`
	OnlyInt       bool     `json:"only_int,omitempty"`
	ExceptDomains []string `json:"except_domains,omitempty"`
	OnlyDomains   []string `json:"only_domains,omitempty"`
}

type SchemaDryRunResult struct {
	Invalid   []map[string]any `json:"invalid"`
	Scanned   int              `json:"scanned"`
	Truncated bool             `json:"truncated"`
}

func (c *Client) collectionsPath(name string) (string, error) {
	path, err := c.databasePath("/data/collections")
	if err != nil {
		return "", err
	}
	if name == "" {
		return path, nil
	}
	if !collectionNamePattern.MatchString(name) {
		return "", errors.New("gosdk: invalid collection name")
	}
	return path + "/" + url.PathEscape(name), nil
}

func (c *Client) ListCollections(ctx context.Context) (*CollectionList, error) {
	path, err := c.collectionsPath("")
	if err != nil {
		return nil, err
	}
	var out CollectionList
	err = c.requestJSON(ctx, http.MethodGet, path, nil, nil, &out)
	return &out, err
}

func (c *Client) CreateCollection(ctx context.Context, name string) error {
	if !collectionNamePattern.MatchString(name) {
		return errors.New("gosdk: invalid collection name")
	}
	path, err := c.collectionsPath("")
	if err != nil {
		return err
	}
	return c.requestJSON(ctx, http.MethodPost, path, nil, struct {
		Name string `json:"name"`
	}{name}, nil)
}

func (q DocumentQuery) values() url.Values {
	values := url.Values{}
	if q.Limit > 0 {
		values.Set("limit", strconv.Itoa(q.Limit))
	}
	if q.Cursor != "" {
		values.Set("cursor", q.Cursor)
	}
	if q.Offset > 0 {
		values.Set("offset", strconv.Itoa(q.Offset))
	}
	if q.Sort != "" {
		values.Set("sort", q.Sort)
	}
	if q.Filter != "" {
		values.Set("filter", q.Filter)
	}
	if len(q.Fields) > 0 {
		values.Set("fields", strings.Join(q.Fields, ","))
	}
	if q.Count {
		values.Set("count", "1")
	}
	return values
}

func (c *Client) ListDocuments(ctx context.Context, collection string) (*DocumentList, error) {
	return c.ListDocumentsWithOptions(ctx, collection, DocumentQuery{})
}

func (c *Client) ListDocumentsWithOptions(ctx context.Context, collection string, query DocumentQuery) (*DocumentList, error) {
	path, err := c.collectionsPath(collection)
	if err != nil {
		return nil, err
	}
	if collection == "" {
		return nil, errors.New("gosdk: collection name is required")
	}
	var out DocumentList
	err = c.requestJSON(ctx, http.MethodGet, path, query.values(), nil, &out)
	return &out, err
}

func (c *Client) documentPath(collection, id string) (string, error) {
	path, err := c.collectionsPath(collection)
	if err != nil {
		return "", err
	}
	if collection == "" || id == "" {
		return "", errors.New("gosdk: collection and document ID are required")
	}
	return path + "/documents/" + url.PathEscape(id), nil
}

func (c *Client) InsertDocument(ctx context.Context, collection string, doc Document) (Document, error) {
	path, err := c.collectionsPath(collection)
	if err != nil {
		return nil, err
	}
	if collection == "" {
		return nil, errors.New("gosdk: collection name is required")
	}
	var out Document
	err = c.requestJSON(ctx, http.MethodPost, path+"/documents", nil, doc, &out)
	return out, err
}

func (c *Client) GetDocument(ctx context.Context, collection, id string) (*DocumentRecord, error) {
	path, err := c.documentPath(collection, id)
	if err != nil {
		return nil, err
	}
	var out DocumentRecord
	err = c.requestJSON(ctx, http.MethodGet, path, nil, nil, &out)
	return &out, err
}

func (c *Client) UpdateDocument(ctx context.Context, collection, id string, doc Document) (Document, error) {
	path, err := c.documentPath(collection, id)
	if err != nil {
		return nil, err
	}
	var out Document
	err = c.requestJSON(ctx, http.MethodPut, path, nil, doc, &out)
	return out, err
}

func (c *Client) PatchDocument(ctx context.Context, collection, id string, doc Document) (Document, error) {
	path, err := c.documentPath(collection, id)
	if err != nil {
		return nil, err
	}
	var out Document
	err = c.requestJSON(ctx, http.MethodPatch, path, nil, doc, &out)
	return out, err
}

func (c *Client) BatchDocuments(ctx context.Context, collection string, documents []Document) ([]DocumentRecord, error) {
	path, err := c.collectionsPath(collection)
	if err != nil {
		return nil, err
	}
	var out struct {
		Items []DocumentRecord `json:"items"`
	}
	err = c.requestJSON(ctx, http.MethodPost, path+"/documents/batch", nil, map[string]any{"documents": documents}, &out)
	return out.Items, err
}

func (c *Client) DeleteDocument(ctx context.Context, collection, id string) error {
	path, err := c.documentPath(collection, id)
	if err != nil {
		return err
	}
	return c.requestJSON(ctx, http.MethodDelete, path, nil, nil, nil)
}

func (c *Client) TruncateCollection(ctx context.Context, collection string) error {
	path, err := c.collectionsPath(collection)
	if err != nil {
		return err
	}
	return c.requestJSON(ctx, http.MethodDelete, path, nil, nil, nil)
}

func (c *Client) GetCollectionSchema(ctx context.Context, collection string) (*CollectionSchema, error) {
	path, err := c.collectionsPath(collection)
	if err != nil {
		return nil, err
	}
	var out CollectionSchema
	err = c.requestJSON(ctx, http.MethodGet, path+"/meta", nil, nil, &out)
	return &out, err
}

func (c *Client) PutCollectionSchema(ctx context.Context, schema CollectionSchema) (*CollectionSchema, error) {
	path, err := c.collectionsPath(schema.Name)
	if err != nil {
		return nil, err
	}
	var out CollectionSchema
	err = c.requestJSON(ctx, http.MethodPut, path+"/meta", nil, schema, &out)
	return &out, err
}

func (c *Client) DryRunCollectionSchema(ctx context.Context, schema CollectionSchema) (*SchemaDryRunResult, error) {
	path, err := c.collectionsPath(schema.Name)
	if err != nil {
		return nil, err
	}
	var out SchemaDryRunResult
	err = c.requestJSON(ctx, http.MethodPut, path+"/meta", url.Values{"dry_run": {"1"}}, schema, &out)
	return &out, err
}

type ViewDryRunResult struct {
	Columns []string          `json:"columns"`
	Fields  []CollectionField `json:"fields"`
	Items   []DocumentRecord  `json:"items"`
	Scanned int               `json:"scanned"`
}

func (c *Client) DryRunView(ctx context.Context, name, query string) (*ViewDryRunResult, error) {
	path, err := c.collectionsPath("")
	if err != nil {
		return nil, err
	}
	var out ViewDryRunResult
	err = c.requestJSON(ctx, http.MethodPost, path+"/meta/dry-run-view", nil, map[string]string{"name": name, "query": query}, &out)
	return &out, err
}

func (c *Client) DeleteCollectionSchema(ctx context.Context, collection string) error {
	path, err := c.collectionsPath(collection)
	if err != nil {
		return err
	}
	return c.requestJSON(ctx, http.MethodDelete, path+"/meta", nil, nil, nil)
}

type ImportResult struct {
	DryRun   bool `json:"dry_run"`
	Scanned  int  `json:"scanned"`
	Imported int  `json:"imported"`
}

type ExportQuery struct {
	Format string
	Filter string
	Sort   string
	Fields []string
}

func (c *Client) ImportCollection(ctx context.Context, collection, format string, dryRun bool, body io.Reader) (*ImportResult, error) {
	path, err := c.collectionsPath(collection)
	if err != nil {
		return nil, err
	}
	contentType := "application/x-ndjson"
	if format == "csv" {
		contentType = "text/csv"
	} else if format != "" && format != "ndjson" {
		return nil, fmt.Errorf("gosdk: unsupported import format %q", format)
	}
	query := url.Values{}
	if dryRun {
		query.Set("dry_run", "1")
	}
	res, err := c.requestStream(ctx, http.MethodPost, path+"/import", query, body, contentType)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var out ImportResult
	if err := json.NewDecoder(io.LimitReader(res.Body, maxResponseBytes)).Decode(&out); err != nil {
		return nil, fmt.Errorf("gosdk: decode import response: %w", err)
	}
	return &out, nil
}

func (c *Client) ExportCollection(ctx context.Context, collection string, query ExportQuery, output io.Writer) error {
	path, err := c.collectionsPath(collection)
	if err != nil {
		return err
	}
	values := url.Values{}
	if query.Format != "" {
		values.Set("format", query.Format)
	}
	if query.Filter != "" {
		values.Set("filter", query.Filter)
	}
	if query.Sort != "" {
		values.Set("sort", query.Sort)
	}
	if len(query.Fields) > 0 {
		values.Set("fields", strings.Join(query.Fields, ","))
	}
	res, err := c.requestStream(ctx, http.MethodGet, path+"/export", values, nil, "")
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if output == nil {
		return errors.New("gosdk: export output is required")
	}
	if _, err := io.Copy(output, res.Body); err != nil {
		return fmt.Errorf("gosdk: export collection: %w", err)
	}
	return nil
}
