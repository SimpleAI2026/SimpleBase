package gosdk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
)

// Raw performs a JSON request and returns the raw JSON body (nil when the body is empty).
func (c *Client) Raw(ctx context.Context, method, path string, query url.Values, body any) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.requestJSON(ctx, method, path, query, body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// KVExec runs one key-value command. argv[0] is the command name.
func (c *Client) KVExec(ctx context.Context, argv []string) (json.RawMessage, error) {
	if len(argv) == 0 {
		return nil, errors.New("gosdk: kv command is required")
	}
	return c.Raw(ctx, http.MethodPost, c.projectPath("/kv"), nil, map[string]any{"type": "cmd", "argvs": argv})
}

// Function is a cloud function summary.
type Function struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	File          string   `json:"file"`
	Description   string   `json:"description"`
	ActiveVersion int64    `json:"active_version"`
	LatestVersion int64    `json:"latest_version"`
	Published     bool     `json:"published"`
	Exports       []string `json:"exports"`
	Source        string   `json:"source,omitempty"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
}

// FunctionVersion is one stored source revision.
type FunctionVersion struct {
	Version   int64    `json:"version"`
	Exports   []string `json:"exports"`
	Note      string   `json:"note"`
	CreatedAt string   `json:"created_at"`
	Active    bool     `json:"active"`
	Source    string   `json:"source,omitempty"`
}

// ListFunctions lists cloud functions in the project.
func (c *Client) ListFunctions(ctx context.Context) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodGet, c.projectPath("/gofunctions"), nil, nil)
}

// GetFunction fetches one function, including source when the server returns it.
func (c *Client) GetFunction(ctx context.Context, name string) (json.RawMessage, error) {
	if name == "" {
		return nil, errors.New("gosdk: function name is required")
	}
	return c.Raw(ctx, http.MethodGet, c.projectPath("/gofunctions/"+url.PathEscape(name)), nil, nil)
}

// CreateFunction creates a function and its first version.
func (c *Client) CreateFunction(ctx context.Context, name, source, description string) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodPost, c.projectPath("/gofunctions"), nil, map[string]any{
		"name": name, "source": source, "description": description, "activate": true,
	})
}

// UpdateFunction updates the description.
func (c *Client) UpdateFunction(ctx context.Context, name, description string) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodPatch, c.projectPath("/gofunctions/"+url.PathEscape(name)), nil, map[string]string{"description": description})
}

// DeleteFunction archives a function.
func (c *Client) DeleteFunction(ctx context.Context, name string) error {
	_, err := c.Raw(ctx, http.MethodDelete, c.projectPath("/gofunctions/"+url.PathEscape(name)), nil, nil)
	return err
}

// ListFunctionVersions lists versions of a function.
func (c *Client) ListFunctionVersions(ctx context.Context, name string) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodGet, c.projectPath("/gofunctions/"+url.PathEscape(name)+"/versions"), nil, nil)
}

// GetFunctionVersion fetches one version, including source.
func (c *Client) GetFunctionVersion(ctx context.Context, name, version string) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodGet, c.projectPath("/gofunctions/"+url.PathEscape(name)+"/versions/"+url.PathEscape(version)), nil, nil)
}

// CreateFunctionVersion stores a new source revision.
func (c *Client) CreateFunctionVersion(ctx context.Context, name, source, note string, activate bool) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodPost, c.projectPath("/gofunctions/"+url.PathEscape(name)+"/versions"), nil, map[string]any{
		"source": source, "note": note, "activate": activate,
	})
}

// ActivateFunctionVersion publishes a version.
func (c *Client) ActivateFunctionVersion(ctx context.Context, name, version string) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodPost, c.projectPath("/gofunctions/"+url.PathEscape(name)+"/versions/"+url.PathEscape(version)+"/activate"), nil, map[string]any{})
}

// TestFunction runs a version in the debug console.
func (c *Client) TestFunction(ctx context.Context, name, version, export string, body json.RawMessage) (json.RawMessage, error) {
	if len(body) == 0 {
		body = json.RawMessage(`{}`)
	}
	return c.Raw(ctx, http.MethodPost, c.projectPath("/gofunctions/"+url.PathEscape(name)+"/versions/"+url.PathEscape(version)+"/test"), nil, map[string]any{
		"function_name": export, "body": body,
	})
}

// InvokeFunction calls the active version via POST /go/:project/:name/:export.
func (c *Client) InvokeFunction(ctx context.Context, name, export string, body json.RawMessage) (json.RawMessage, error) {
	if name == "" || export == "" {
		return nil, errors.New("gosdk: function name and export are required")
	}
	if len(body) == 0 {
		body = json.RawMessage(`{}`)
	}
	path := "/go/" + url.PathEscape(c.projectID) + "/" + url.PathEscape(name) + "/" + url.PathEscape(export)
	return c.Raw(ctx, http.MethodPost, path, nil, body)
}

// CronJobInput is the create/update body for a scheduled function call.
type CronJobInput struct {
	Name            string `json:"name,omitempty"`
	Description     string `json:"description,omitempty"`
	ScheduleKind    string `json:"schedule_kind,omitempty"`
	CronExpr        string `json:"cron_expr,omitempty"`
	IntervalSeconds int64  `json:"interval_seconds,omitempty"`
	RunAt           string `json:"run_at,omitempty"`
	FuncFile        string `json:"func_file,omitempty"`
	FuncExport      string `json:"func_export,omitempty"`
	InputJSON       string `json:"input_json,omitempty"`
	Enabled         *bool  `json:"enabled,omitempty"`
}

// ListCronJobs lists schedules.
func (c *Client) ListCronJobs(ctx context.Context) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodGet, c.projectPath("/cron-jobs"), nil, nil)
}

// GetCronJob fetches one schedule.
func (c *Client) GetCronJob(ctx context.Context, id string) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodGet, c.projectPath("/cron-jobs/"+url.PathEscape(id)), nil, nil)
}

// CreateCronJob creates a schedule.
func (c *Client) CreateCronJob(ctx context.Context, in CronJobInput) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodPost, c.projectPath("/cron-jobs"), nil, in)
}

// UpdateCronJob patches a schedule.
func (c *Client) UpdateCronJob(ctx context.Context, id string, in CronJobInput) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodPatch, c.projectPath("/cron-jobs/"+url.PathEscape(id)), nil, in)
}

// DeleteCronJob archives a schedule.
func (c *Client) DeleteCronJob(ctx context.Context, id string) error {
	_, err := c.Raw(ctx, http.MethodDelete, c.projectPath("/cron-jobs/"+url.PathEscape(id)), nil, nil)
	return err
}

// ListCronRuns lists runs of a schedule.
func (c *Client) ListCronRuns(ctx context.Context, id string) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodGet, c.projectPath("/cron-jobs/"+url.PathEscape(id)+"/runs"), nil, nil)
}

// TriggerCronJob starts a run immediately.
func (c *Client) TriggerCronJob(ctx context.Context, id string) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodPost, c.projectPath("/cron-jobs/"+url.PathEscape(id)+"/trigger"), nil, map[string]any{})
}

// SearchLogs queries project log events.
func (c *Client) SearchLogs(ctx context.Context, level, q string, limit int) (json.RawMessage, error) {
	query := url.Values{}
	if level != "" {
		query.Set("level", level)
	}
	if q != "" {
		query.Set("q", q)
	}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	return c.Raw(ctx, http.MethodGet, c.projectPath("/logs"), query, nil)
}

// GetLogRetention reads the project log retention.
func (c *Client) GetLogRetention(ctx context.Context) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodGet, c.projectPath("/logs/retention"), nil, nil)
}

// SetLogRetention updates how many days of logs are kept.
func (c *Client) SetLogRetention(ctx context.Context, keepDays int) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodPut, c.projectPath("/logs/retention"), nil, map[string]any{"keep_days": keepDays})
}

// ListUsers lists instance users. The server omits passwords.
func (c *Client) ListUsers(ctx context.Context, limit int, cursor string) (json.RawMessage, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	return c.Raw(ctx, http.MethodGet, "/v1/users", q, nil)
}

// GetUser fetches one user.
func (c *Client) GetUser(ctx context.Context, id string) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodGet, "/v1/users/"+url.PathEscape(id), nil, nil)
}

// UserInput is a create or patch body. Password is accepted on the wire and must not be logged by callers.
type UserInput struct {
	Username    string  `json:"username,omitempty"`
	Password    string  `json:"password,omitempty"`
	Role        string  `json:"role,omitempty"`
	DisplayName string  `json:"display_name,omitempty"`
	Email       string  `json:"email,omitempty"`
	Status      *string `json:"status,omitempty"`
}

// CreateUser creates a console user.
func (c *Client) CreateUser(ctx context.Context, in UserInput) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodPost, "/v1/users", nil, in)
}

// UpdateUser patches a user.
func (c *Client) UpdateUser(ctx context.Context, id string, in UserInput) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodPatch, "/v1/users/"+url.PathEscape(id), nil, in)
}

// DeleteUser deletes a user.
func (c *Client) DeleteUser(ctx context.Context, id string) error {
	_, err := c.Raw(ctx, http.MethodDelete, "/v1/users/"+url.PathEscape(id), nil, nil)
	return err
}

// ListProjects lists projects visible to the caller.
func (c *Client) ListProjects(ctx context.Context) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodGet, "/v1/projects", nil, nil)
}

// CreateProject creates a project.
func (c *Client) CreateProject(ctx context.Context, name string) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodPost, "/v1/projects", nil, map[string]string{"name": name})
}

// APIKey is a project API key. Secret is set only on create.
type APIKey struct {
	ID          string   `json:"id"`
	Permissions []string `json:"permissions"`
	Secret      string   `json:"secret,omitempty"`
	CreatedAt   string   `json:"created_at,omitempty"`
}

// ListAPIKeys lists keys. Secrets are never returned.
func (c *Client) ListAPIKeys(ctx context.Context) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodGet, c.projectPath("/api-keys"), nil, nil)
}

// CreateAPIKey issues a key. The secret is only present in this response.
func (c *Client) CreateAPIKey(ctx context.Context, permissions []string) (*APIKey, error) {
	var out APIKey
	err := c.requestJSON(ctx, http.MethodPost, c.projectPath("/api-keys"), nil, map[string]any{"permissions": permissions}, &out)
	return &out, err
}

// RevokeAPIKey revokes a key.
func (c *Client) RevokeAPIKey(ctx context.Context, id string) error {
	_, err := c.Raw(ctx, http.MethodDelete, c.projectPath("/api-keys/"+url.PathEscape(id)), nil, nil)
	return err
}

// GetSettings reads project settings.
func (c *Client) GetSettings(ctx context.Context) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodGet, c.projectPath("/settings"), nil, nil)
}

// PutSettings replaces project settings.
func (c *Client) PutSettings(ctx context.Context, body json.RawMessage) (json.RawMessage, error) {
	if len(body) == 0 {
		body = json.RawMessage(`{}`)
	}
	return c.Raw(ctx, http.MethodPut, c.projectPath("/settings"), nil, body)
}

// GetQuota reads the project quota snapshot.
func (c *Client) GetQuota(ctx context.Context) (json.RawMessage, error) {
	return c.Raw(ctx, http.MethodGet, c.projectPath("/quota"), nil, nil)
}

// ListAudit lists recent operations.
func (c *Client) ListAudit(ctx context.Context, limit int) (json.RawMessage, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	return c.Raw(ctx, http.MethodGet, c.projectPath("/audit"), q, nil)
}
