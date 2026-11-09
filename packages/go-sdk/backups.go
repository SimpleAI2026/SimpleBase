package gosdk

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"
)

type Backup struct {
	ID            string     `json:"id"`
	ProjectID     string     `json:"project_id"`
	SizeBytes     int64      `json:"size_bytes"`
	SHA256        string     `json:"sha256"`
	Engine        string     `json:"engine"`
	FormatVersion int        `json:"format_version"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
}

func (c *Client) backupsPath(id string) string {
	path := c.projectPath("/backups")
	if id != "" {
		path += "/" + url.PathEscape(id)
	}
	return path
}

func (c *Client) ListBackups(ctx context.Context) ([]Backup, error) {
	var out struct {
		Items []Backup `json:"items"`
	}
	err := c.requestJSON(ctx, http.MethodGet, c.backupsPath(""), nil, nil, &out)
	return out.Items, err
}

func (c *Client) CreateBackup(ctx context.Context) (*Backup, error) {
	var out Backup
	err := c.requestJSON(ctx, http.MethodPost, c.backupsPath(""), nil, map[string]any{}, &out)
	return &out, err
}

func (c *Client) DeleteBackup(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("gosdk: backup ID is required")
	}
	return c.requestJSON(ctx, http.MethodDelete, c.backupsPath(id), nil, nil, nil)
}

func (c *Client) RestoreBackup(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("gosdk: backup ID is required")
	}
	return c.requestJSON(ctx, http.MethodPost, c.backupsPath(id)+"/restore", nil, map[string]any{}, nil)
}
