package gosdk

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

type FunctionTrigger struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	Kind       string    `json:"kind"`
	Target     string    `json:"target"`
	Events     []string  `json:"events"`
	FuncFile   string    `json:"func_file"`
	FuncExport string    `json:"func_export"`
	Filter     string    `json:"filter,omitempty"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (c *Client) ListFunctionTriggers(ctx context.Context) ([]FunctionTrigger, error) {
	var out struct {
		Items []FunctionTrigger `json:"items"`
	}
	err := c.requestJSON(ctx, http.MethodGet, c.projectPath("/function-triggers"), nil, nil, &out)
	return out.Items, err
}

func (c *Client) CreateFunctionTrigger(ctx context.Context, trigger FunctionTrigger) (*FunctionTrigger, error) {
	var out FunctionTrigger
	err := c.requestJSON(ctx, http.MethodPost, c.projectPath("/function-triggers"), nil, trigger, &out)
	return &out, err
}

func (c *Client) UpdateFunctionTrigger(ctx context.Context, id string, patch map[string]any) (*FunctionTrigger, error) {
	var out FunctionTrigger
	err := c.requestJSON(ctx, http.MethodPatch, c.projectPath("/function-triggers/"+url.PathEscape(id)), nil, patch, &out)
	return &out, err
}

func (c *Client) DeleteFunctionTrigger(ctx context.Context, id string) error {
	return c.requestJSON(ctx, http.MethodDelete, c.projectPath("/function-triggers/"+url.PathEscape(id)), nil, nil, nil)
}

func (c *Client) InvokeFunctionTrigger(ctx context.Context, id string, body map[string]any) error {
	return c.requestJSON(ctx, http.MethodPost, c.projectPath("/function-triggers/"+url.PathEscape(id)+"/invoke"), nil, body, nil)
}
