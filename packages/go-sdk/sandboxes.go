package gosdk

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// SandboxInfo is a project-scoped sandbox resource.
type SandboxInfo struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	CloudName string `json:"cloud_name"`
	Source    string `json:"source"`
	Image     string `json:"image"`
	CPUs      int    `json:"cpus"`
	MemoryMiB int    `json:"memory_mib"`
	Network   string `json:"network"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type SandboxCapabilities struct {
	Available     bool     `json:"available"`
	Backend       string   `json:"backend"`
	Images        []string `json:"images"`
	DefaultImage  string   `json:"default_image"`
	MaxPerProject int      `json:"max_per_project"`
	MaxFileBytes  int      `json:"max_file_bytes"`
}
type CreateSandboxInput struct {
	Name         string `json:"name,omitempty"`
	Image        string `json:"image,omitempty"`
	CPUs         int    `json:"cpus,omitempty"`
	MemoryMiB    int    `json:"memory_mib,omitempty"`
	Network      string `json:"network,omitempty"`
	IdleTimeoutS int64  `json:"idle_timeout_s,omitempty"`
	Start        bool   `json:"start,omitempty"`
}
type UpdateSandboxInput struct {
	Name         *string `json:"name,omitempty"`
	IdleTimeoutS *int64  `json:"idle_timeout_s,omitempty"`
}
type SandboxExecInput struct {
	Command  string            `json:"command,omitempty"`
	Cmd      string            `json:"cmd,omitempty"`
	Args     []string          `json:"args,omitempty"`
	Cwd      string            `json:"cwd,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	TimeoutS int64             `json:"timeout_s,omitempty"`
}
type SandboxExecResult struct {
	ExitCode        int    `json:"exit_code"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	StdoutTruncated bool   `json:"stdout_truncated"`
	StderrTruncated bool   `json:"stderr_truncated"`
	TimedOut        bool   `json:"timed_out"`
	DurationMs      int64  `json:"duration_ms"`
	Status          string `json:"status"`
	SandboxID       string `json:"sandbox_id,omitempty"`
}
type SandboxRunInput struct {
	SandboxExecInput
	Image string           `json:"image,omitempty"`
	Files []SandboxRunFile `json:"files,omitempty"`
	Keep  bool             `json:"keep,omitempty"`
}
type SandboxRunFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}
type SandboxFileEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Kind string `json:"kind"`
	Size int64  `json:"size"`
}
type SandboxFileContent struct {
	Content   string `json:"content"`
	Encoding  string `json:"encoding"`
	Truncated bool   `json:"truncated"`
}

func (c *Client) sandboxPath(id, suffix string) string {
	p := "/sandboxes"
	if id != "" {
		p += "/" + url.PathEscape(id)
	}
	return c.projectPath(p + suffix)
}
func (c *Client) SandboxCapabilities(ctx context.Context) (*SandboxCapabilities, error) {
	var out SandboxCapabilities
	err := c.requestJSON(ctx, http.MethodGet, c.sandboxPath("", "/capabilities"), nil, nil, &out)
	return &out, err
}
func (c *Client) ListSandboxes(ctx context.Context, status, source string, limit int) ([]SandboxInfo, error) {
	page, err := c.ListSandboxesPage(ctx, SandboxListOptions{Status: status, Source: source, Limit: limit})
	if err != nil {
		return nil, err
	}
	return page.Sandboxes, nil
}

// SandboxListOptions filters and paginates ListSandboxesPage.
type SandboxListOptions struct {
	Status string
	Source string
	Limit  int
	// Cursor is the NextCursor of the previous page; empty starts from the first page.
	Cursor string
}

// SandboxPage is one page of sandboxes. NextCursor is empty on the last page.
type SandboxPage struct {
	Sandboxes  []SandboxInfo `json:"sandboxes"`
	NextCursor string        `json:"next_cursor"`
}

// ListSandboxesPage returns one page, newest first.
func (c *Client) ListSandboxesPage(ctx context.Context, opts SandboxListOptions) (*SandboxPage, error) {
	q := url.Values{}
	if opts.Status != "" {
		q.Set("status", opts.Status)
	}
	if opts.Source != "" {
		q.Set("source", opts.Source)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Cursor != "" {
		q.Set("cursor", opts.Cursor)
	}
	var out SandboxPage
	if err := c.requestJSON(ctx, http.MethodGet, c.sandboxPath("", ""), q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
func (c *Client) CreateSandbox(ctx context.Context, in CreateSandboxInput) (*SandboxInfo, error) {
	var out SandboxInfo
	err := c.requestJSON(ctx, http.MethodPost, c.sandboxPath("", ""), nil, in, &out)
	return &out, err
}
func (c *Client) GetSandbox(ctx context.Context, id string, refresh bool) (*SandboxInfo, error) {
	q := url.Values{}
	if refresh {
		q.Set("refresh", "1")
	}
	var out SandboxInfo
	err := c.requestJSON(ctx, http.MethodGet, c.sandboxPath(id, ""), q, nil, &out)
	return &out, err
}
func (c *Client) UpdateSandbox(ctx context.Context, id string, in UpdateSandboxInput) (*SandboxInfo, error) {
	var out SandboxInfo
	err := c.requestJSON(ctx, http.MethodPatch, c.sandboxPath(id, ""), nil, in, &out)
	return &out, err
}
func (c *Client) DeleteSandbox(ctx context.Context, id string) error {
	return c.requestJSON(ctx, http.MethodDelete, c.sandboxPath(id, ""), nil, nil, nil)
}
func (c *Client) StartSandbox(ctx context.Context, id string) (*SandboxInfo, error) {
	var out SandboxInfo
	err := c.requestJSON(ctx, http.MethodPost, c.sandboxPath(id, "/start"), nil, nil, &out)
	return &out, err
}
func (c *Client) StopSandbox(ctx context.Context, id string) (*SandboxInfo, error) {
	var out SandboxInfo
	err := c.requestJSON(ctx, http.MethodPost, c.sandboxPath(id, "/stop"), nil, nil, &out)
	return &out, err
}
func (c *Client) ExecSandbox(ctx context.Context, id string, in SandboxExecInput) (*SandboxExecResult, error) {
	var out SandboxExecResult
	err := c.requestJSON(ctx, http.MethodPost, c.sandboxPath(id, "/exec"), nil, in, &out)
	return &out, err
}
func (c *Client) RunSandbox(ctx context.Context, in SandboxRunInput) (*SandboxExecResult, error) {
	// anonymous embedded structs are flattened explicitly to keep JSON field names at the root.
	body := struct {
		SandboxExecInput
		Image string           `json:"image,omitempty"`
		Files []SandboxRunFile `json:"files,omitempty"`
		Keep  bool             `json:"keep,omitempty"`
	}{SandboxExecInput: in.SandboxExecInput, Image: in.Image, Files: in.Files, Keep: in.Keep}
	var out SandboxExecResult
	err := c.requestJSON(ctx, http.MethodPost, c.sandboxPath("", "/run"), nil, body, &out)
	return &out, err
}
func (c *Client) ListSandboxFiles(ctx context.Context, id, path string) ([]SandboxFileEntry, error) {
	q := url.Values{"path": {path}}
	var out struct {
		Entries []SandboxFileEntry `json:"entries"`
	}
	err := c.requestJSON(ctx, http.MethodGet, c.sandboxPath(id, "/files"), q, nil, &out)
	return out.Entries, err
}
func (c *Client) ReadSandboxFile(ctx context.Context, id, path string) (*SandboxFileContent, error) {
	q := url.Values{"path": {path}}
	var out SandboxFileContent
	err := c.requestJSON(ctx, http.MethodGet, c.sandboxPath(id, "/files/content"), q, nil, &out)
	return &out, err
}
func (c *Client) WriteSandboxFile(ctx context.Context, id, path, content string) error {
	q := url.Values{"path": {path}}
	return c.requestJSON(ctx, http.MethodPut, c.sandboxPath(id, "/files/content"), q, map[string]string{"content": content}, nil)
}
func (c *Client) DeleteSandboxFile(ctx context.Context, id, path string) error {
	q := url.Values{"path": {path}}
	return c.requestJSON(ctx, http.MethodDelete, c.sandboxPath(id, "/files/content"), q, nil, nil)
}
