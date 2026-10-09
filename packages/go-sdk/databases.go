package gosdk

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
)

// DatabaseInfo is a logical database resource.
type DatabaseInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	DocumentCount *int64 `json:"document_count,omitempty"`
	// DataModel is "collection" (default, including databases created before the column existed) or "sql".
	DataModel string `json:"data_model,omitempty"`
}

// CreateDatabaseInput creates a collection database or a SQL database.
// DataModel empty means collection. InitSQL is only valid when DataModel is sql.
type CreateDatabaseInput struct {
	Name      string `json:"name"`
	DataModel string `json:"data_model,omitempty"`
	InitSQL   string `json:"init_sql,omitempty"`
}

// SchemaColumn is one column in a SQL database table.
type SchemaColumn struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable *bool  `json:"nullable,omitempty"`
}

// SchemaTable is a user table and its columns.
type SchemaTable struct {
	Name    string         `json:"name"`
	Columns []SchemaColumn `json:"columns"`
}

// DatabaseSchema lists tables in a SQL database.
type DatabaseSchema struct {
	Tables []SchemaTable `json:"tables"`
}

// DatabaseList contains a page of databases and an optional next cursor.
type DatabaseList struct {
	Databases  []DatabaseInfo `json:"databases"`
	NextCursor string         `json:"next_cursor"`
}

// DeleteDatabaseResult describes the asynchronous deletion response (HTTP 202).
type DeleteDatabaseResult struct {
	DatabaseID string `json:"database_id"`
	Status     string `json:"status"`
}

// ListDatabases lists logical databases; limit 0 uses the server default.
func (c *Client) ListDatabases(ctx context.Context, limit int, cursor string) (*DatabaseList, error) {
	q := url.Values{}
	if limit != 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	var out DatabaseList
	err := c.requestJSON(ctx, http.MethodGet, c.projectPath("/databases"), q, nil, &out)
	return &out, err
}

// GetDatabase fetches one logical database.
func (c *Client) GetDatabase(ctx context.Context, id string) (*DatabaseInfo, error) {
	if id == "" {
		return nil, errors.New("gosdk: database ID is required")
	}
	var out DatabaseInfo
	err := c.requestJSON(ctx, http.MethodGet, c.projectPath("/databases/"+url.PathEscape(id)), nil, nil, &out)
	return &out, err
}

// CreateDatabase creates a collection database with the given name.
func (c *Client) CreateDatabase(ctx context.Context, name string) (*DatabaseInfo, error) {
	return c.CreateDatabaseWith(ctx, CreateDatabaseInput{Name: name})
}

// CreateDatabaseWith creates a collection or SQL database.
// SQL databases may include initialization statements in InitSQL.
func (c *Client) CreateDatabaseWith(ctx context.Context, in CreateDatabaseInput) (*DatabaseInfo, error) {
	if in.Name == "" {
		return nil, errors.New("gosdk: database name is required")
	}
	var out DatabaseInfo
	err := c.requestJSON(ctx, http.MethodPost, c.projectPath("/databases"), nil, in, &out)
	return &out, err
}

func (c *Client) schemaPath(databaseID, suffix string) (string, error) {
	if databaseID == "" {
		return "", errors.New("gosdk: database ID is required")
	}
	return c.projectPath("/databases/" + url.PathEscape(databaseID) + suffix), nil
}

// DatabaseSchema lists tables and columns. Only SQL databases accept this call.
func (c *Client) DatabaseSchema(ctx context.Context, databaseID string) (*DatabaseSchema, error) {
	path, err := c.schemaPath(databaseID, "/schema")
	if err != nil {
		return nil, err
	}
	var out DatabaseSchema
	err = c.requestJSON(ctx, http.MethodGet, path, nil, nil, &out)
	return &out, err
}

// CreateTable adds a table to a SQL database.
func (c *Client) CreateTable(ctx context.Context, databaseID string, table SchemaTable) (*SchemaTable, error) {
	path, err := c.schemaPath(databaseID, "/schema/tables")
	if err != nil {
		return nil, err
	}
	var out SchemaTable
	err = c.requestJSON(ctx, http.MethodPost, path, nil, table, &out)
	return &out, err
}

// AddColumn adds one column to an existing table in a SQL database.
func (c *Client) AddColumn(ctx context.Context, databaseID, table string, column SchemaColumn) (*SchemaColumn, error) {
	path, err := c.schemaPath(databaseID, "/schema/columns")
	if err != nil {
		return nil, err
	}
	body := struct {
		Table    string `json:"table"`
		Name     string `json:"name"`
		Type     string `json:"type"`
		Nullable *bool  `json:"nullable,omitempty"`
	}{Table: table, Name: column.Name, Type: column.Type, Nullable: column.Nullable}
	var out SchemaColumn
	err = c.requestJSON(ctx, http.MethodPost, path, nil, body, &out)
	return &out, err
}

// TableRows is a page of rows from a SQL table.
type TableRows struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

// ListTableRows reads rows from one SQL table.
func (c *Client) ListTableRows(ctx context.Context, databaseID, table string, limit int) (*TableRows, error) {
	path, err := c.schemaPath(databaseID, "/schema/tables/"+url.PathEscape(table)+"/rows")
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var out TableRows
	err = c.requestJSON(ctx, http.MethodGet, path, q, nil, &out)
	return &out, err
}

// DeleteDatabase requests deletion; the response is not a DatabaseInfo.
func (c *Client) DeleteDatabase(ctx context.Context, id string) (*DeleteDatabaseResult, error) {
	if id == "" {
		return nil, errors.New("gosdk: database ID is required")
	}
	var out DeleteDatabaseResult
	err := c.requestJSON(ctx, http.MethodDelete, c.projectPath("/databases/"+url.PathEscape(id)), nil, nil, &out)
	return &out, err
}
