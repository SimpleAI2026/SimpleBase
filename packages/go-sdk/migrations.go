package gosdk

import (
	"context"
	"net/http"
)

type MigrationTableStatus struct {
	Name    string `json:"name"`
	Present bool   `json:"present"`
}

type MigrationStatus struct {
	FormatVersion int                    `json:"format_version"`
	Ready         bool                   `json:"ready"`
	Expected      int                    `json:"expected"`
	Present       int                    `json:"present"`
	Missing       []string               `json:"missing"`
	Tables        []MigrationTableStatus `json:"tables"`
}

type MigrationPlan struct {
	FormatVersion int      `json:"format_version"`
	Pending       []string `json:"pending"`
	Statements    []string `json:"statements"`
}

func (c *Client) MigrationStatus(ctx context.Context) (*MigrationStatus, error) {
	var out MigrationStatus
	err := c.requestJSON(ctx, http.MethodGet, c.projectPath("/migrations/status"), nil, nil, &out)
	return &out, err
}

func (c *Client) MigrationPlan(ctx context.Context) (*MigrationPlan, error) {
	var out MigrationPlan
	err := c.requestJSON(ctx, http.MethodGet, c.projectPath("/migrations/plan"), nil, nil, &out)
	return &out, err
}
