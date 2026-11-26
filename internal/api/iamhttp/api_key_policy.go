package iamhttp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/linkxzhou/SimpleBase/internal/collection"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/auth"
)

type apiKeyPolicyInput struct {
	Resources      []string   `json:"resources"`
	Actions        []string   `json:"actions"`
	HiddenFields   []string   `json:"hidden_fields,omitempty"`
	ReadonlyFields []string   `json:"readonly_fields,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}

func (p *apiKeyPolicyInput) policy() *auth.KeyPolicy {
	if p == nil {
		return nil
	}
	return &auth.KeyPolicy{Resources: p.Resources, Actions: p.Actions, HiddenFields: p.HiddenFields, ReadonlyFields: p.ReadonlyFields, ExpiresAt: p.ExpiresAt}
}

func validateAPIKeyPolicy(input *apiKeyPolicyInput) error {
	if input == nil {
		return nil
	}
	if len(input.Resources) == 0 || len(input.Actions) == 0 {
		return fmt.Errorf("resources and actions are required")
	}
	for _, resource := range input.Resources {
		parts := strings.SplitN(resource, ":", 2)
		if len(parts) != 2 || len(resource) > 160 || strings.ContainsAny(resource, " \t\r\n") {
			return fmt.Errorf("invalid resource scope")
		}
		switch parts[0] {
		case "database", "collection", "object", "function", "cron", "project":
		default:
			return fmt.Errorf("invalid resource scope")
		}
	}
	for _, action := range input.Actions {
		switch action {
		case "read", "write", "invoke", "presign":
		default:
			return fmt.Errorf("invalid action scope")
		}
	}
	for _, name := range append(append([]string{}, input.HiddenFields...), input.ReadonlyFields...) {
		if !collection.FieldNamePattern.MatchString(name) {
			return fmt.Errorf("invalid field scope")
		}
	}
	return nil
}

func scopeSubset(parent, child *auth.KeyPolicy) bool {
	if parent == nil {
		return true
	}
	if child == nil {
		return false
	}
	for _, values := range [][2][]string{{parent.Resources, child.Resources}, {parent.Actions, child.Actions}, {parent.HiddenFields, child.HiddenFields}, {parent.ReadonlyFields, child.ReadonlyFields}} {
		for _, value := range values[1] {
			found := false
			for _, allowed := range values[0] {
				if allowed == "*" || allowed == value {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}

func putAPIKeyPolicy(ctx context.Context, db *sql.DB, keyID, projectID string, input *apiKeyPolicyInput) error {
	if input == nil {
		return nil
	}
	resources, _ := json.Marshal(input.Resources)
	actions, _ := json.Marshal(input.Actions)
	hidden, _ := json.Marshal(input.HiddenFields)
	readonly, _ := json.Marshal(input.ReadonlyFields)
	_, err := db.ExecContext(ctx, `INSERT INTO sys_api_key_policies(key_id, project_id, resources_json, actions_json, hidden_fields_json, readonly_fields_json, expires_at, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		keyID, projectID, string(resources), string(actions), string(hidden), string(readonly), input.ExpiresAt, time.Now().UTC())
	return err
}
