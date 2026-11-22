package agentbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

type emptyInput struct{}
type listCollectionsInput struct {
	DatabaseID string `json:"database_id" jsonschema:"description=Database UUID to list collections for"`
}
type readonlySQLInput struct {
	DatabaseID string `json:"database_id" jsonschema:"description=Database UUID"`
	SQL        string `json:"sql" jsonschema:"description=A single readonly SQL statement"`
}
type listObjectsInput struct {
	Prefix string `json:"prefix,omitempty" jsonschema:"description=Optional object key prefix"`
}
type headObjectInput struct {
	Key string `json:"key" jsonschema:"description=Project-relative object key"`
}
type searchLogsInput struct {
	Level string `json:"level,omitempty" jsonschema:"description=Optional level filter such as info, warn, error"`
	Q     string `json:"q,omitempty" jsonschema:"description=Optional message substring"`
	Limit int    `json:"limit,omitempty" jsonschema:"description=Max events, default 50"`
}

type toolDeps struct {
	DB      DatabaseAccess
	Obj     ObjectAccess
	Logs    LogReader
	Sandbox Sandbox
}

type toolBuilder struct {
	id    string
	build func(toolDeps) (tool.InvokableTool, error)
}

var builtinTools = []toolBuilder{
	{ToolListDatabases, listDatabasesTool},
	{ToolListCollections, listCollectionsTool},
	{ToolReadonlySQL, readonlySQLTool},
	{ToolListObjects, listObjectsTool},
	{ToolHeadObject, headObjectTool},
	{ToolSearchLogs, searchLogsTool},
	{ToolLogLevelStats, logLevelStatsTool},
}

func buildTools(ids []string, deps toolDeps) ([]tool.BaseTool, error) {
	want := map[string]bool{}
	for _, id := range ids {
		if KnownTool(id) {
			want[id] = true
		}
	}
	out := make([]tool.BaseTool, 0, len(want))
	for _, builder := range builtinTools {
		if !want[builder.id] {
			continue
		}
		t, err := builder.build(deps)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := buildSandboxTools(&out, want, deps); err != nil {
		return nil, err
	}
	return out, nil
}

func listDatabasesTool(deps toolDeps) (tool.InvokableTool, error) {
	return utils.InferTool(ToolListDatabases, "List user databases in this project (id, name, status). No storage paths or credentials.",
		func(ctx context.Context, _ emptyInput) (string, error) {
			rc, err := requireRun(ctx)
			if err != nil {
				return "", err
			}
			if deps.DB == nil {
				return "", fmt.Errorf("database access is not configured")
			}
			list, err := deps.DB.ListDatabases(ctx, rc.Principal, rc.ProjectID)
			if err != nil {
				return "", err
			}
			return marshalToolJSON(list)
		})
}

func listCollectionsTool(deps toolDeps) (tool.InvokableTool, error) {
	return utils.InferTool(ToolListCollections, "List table/collection names in a database via information_schema.",
		func(ctx context.Context, in listCollectionsInput) (string, error) {
			rc, err := requireRun(ctx)
			if err != nil {
				return "", err
			}
			if deps.DB == nil {
				return "", fmt.Errorf("database access is not configured")
			}
			if strings.TrimSpace(in.DatabaseID) == "" {
				return "", fmt.Errorf("database_id is required")
			}
			names, err := deps.DB.ListCollections(ctx, rc.Principal, rc.ProjectID, in.DatabaseID)
			if err != nil {
				return "", err
			}
			return marshalToolJSON(map[string]any{"collections": names})
		})
}

func readonlySQLTool(deps toolDeps) (tool.InvokableTool, error) {
	return utils.InferTool(ToolReadonlySQL, "Run a single readonly SQL statement (SELECT/WITH/EXPLAIN/DESCRIBE/SHOW). Writes are rejected.",
		func(ctx context.Context, in readonlySQLInput) (string, error) {
			rc, err := requireRun(ctx)
			if err != nil {
				return "", err
			}
			if deps.DB == nil {
				return "", fmt.Errorf("database access is not configured")
			}
			if strings.TrimSpace(in.DatabaseID) == "" || strings.TrimSpace(in.SQL) == "" {
				return "", fmt.Errorf("database_id and sql are required")
			}
			res, err := deps.DB.ReadOnlyQuery(ctx, rc.Principal, rc.ProjectID, in.DatabaseID, in.SQL, 50)
			if err != nil {
				return "", err
			}
			return marshalToolJSON(res)
		})
}

func listObjectsTool(deps toolDeps) (tool.InvokableTool, error) {
	return utils.InferTool(ToolListObjects, "List project object-store keys (relative keys, size, last modified). No credentials.",
		func(ctx context.Context, in listObjectsInput) (string, error) {
			rc, err := requireRun(ctx)
			if err != nil {
				return "", err
			}
			if deps.Obj == nil {
				return "", fmt.Errorf("object store is not configured")
			}
			list, err := deps.Obj.ListObjects(ctx, rc.ProjectID, in.Prefix, 100)
			if err != nil {
				return "", err
			}
			return marshalToolJSON(list)
		})
}

func headObjectTool(deps toolDeps) (tool.InvokableTool, error) {
	return utils.InferTool(ToolHeadObject, "Return metadata for one project-relative object key (size, etag, content type). Does not read the body.",
		func(ctx context.Context, in headObjectInput) (string, error) {
			rc, err := requireRun(ctx)
			if err != nil {
				return "", err
			}
			if deps.Obj == nil {
				return "", fmt.Errorf("object store is not configured")
			}
			if strings.TrimSpace(in.Key) == "" {
				return "", fmt.Errorf("key is required")
			}
			obj, err := deps.Obj.HeadObject(ctx, rc.ProjectID, in.Key)
			if err != nil {
				return "", err
			}
			return marshalToolJSON(obj)
		})
}

func searchLogsTool(deps toolDeps) (tool.InvokableTool, error) {
	return utils.InferTool(ToolSearchLogs, "Search recent project log events by optional level and substring.",
		func(ctx context.Context, in searchLogsInput) (string, error) {
			rc, err := requireRun(ctx)
			if err != nil {
				return "", err
			}
			if deps.Logs == nil {
				return "", fmt.Errorf("log search is not configured")
			}
			limit := in.Limit
			if limit <= 0 {
				limit = 50
			}
			events, err := deps.Logs.SearchLogs(ctx, rc.ProjectID, in.Level, in.Q, limit)
			if err != nil {
				return "", err
			}
			type row struct {
				Level      string `json:"level"`
				Logger     string `json:"logger"`
				Message    string `json:"message"`
				OccurredAt string `json:"occurred_at"`
			}
			rows := make([]row, 0, len(events))
			for _, event := range events {
				rows = append(rows, row{Level: event.Level, Logger: event.Logger, Message: truncate(event.Message, 500), OccurredAt: event.OccurredAt.UTC().Format(time.RFC3339)})
			}
			return marshalToolJSON(rows)
		})
}

func logLevelStatsTool(deps toolDeps) (tool.InvokableTool, error) {
	return utils.InferTool(ToolLogLevelStats, "Count log events in this project grouped by level.",
		func(ctx context.Context, _ emptyInput) (string, error) {
			rc, err := requireRun(ctx)
			if err != nil {
				return "", err
			}
			if deps.Logs == nil {
				return "", fmt.Errorf("log stats is not configured")
			}
			stats, err := deps.Logs.LevelStats(ctx, rc.ProjectID)
			if err != nil {
				return "", err
			}
			return marshalToolJSON(stats)
		})
}

func marshalToolJSON(v any) (string, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return agentcore.RedactSecrets(string(body)), nil
}
