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

// CreateDatabase creates a database with the given name.
func (c *Client) CreateDatabase(ctx context.Context, name string) (*DatabaseInfo, error) {
	if name == "" {
		return nil, errors.New("gosdk: database name is required")
	}
	var out DatabaseInfo
	err := c.requestJSON(ctx, http.MethodPost, c.projectPath("/databases"), nil, struct {
		Name string `json:"name"`
	}{name}, &out)
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
