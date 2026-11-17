// 文件说明：tools.go — 只读数据 / 日志内置工具与运行上下文

package cloudagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

type runCtxKey struct{}

// RunContext is injected for tool invocations (no secrets).
type RunContext struct {
	ProjectID string
	Principal Principal
	// ThreadID 供沙盒工具定位该 thread 的云沙盒；其余工具不使用。
	ThreadID string
	RunID    string
	Skills   []string
	Headless bool
	// Writable 是该助手是否允许写（planv6.0 v2 §3.4）：false 时 CLI 工具
	// 拒绝所有非只读命令（在破坏性确认之前拦截）。
	Writable bool
	Emit     func(Event)
}

func withRunContext(ctx context.Context, rc RunContext) context.Context {
	return context.WithValue(ctx, runCtxKey{}, rc)
}

func runContextFrom(ctx context.Context) (RunContext, bool) {
	v, ok := ctx.Value(runCtxKey{}).(RunContext)
	return v, ok
}

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
	Logs    LogAccess
	Sandbox Sandbox
}

func buildTools(ids []string, deps toolDeps) ([]tool.BaseTool, error) {
	want := map[string]bool{}
	for _, id := range ids {
		if KnownTool(id) {
			want[id] = true
		}
	}
	out := make([]tool.BaseTool, 0, len(want))
	add := func(t tool.InvokableTool, err error) error {
		if err != nil {
			return err
		}
		out = append(out, t)
		return nil
	}
	if want[ToolListDatabases] {
		if err := add(utils.InferTool(ToolListDatabases, "List user databases in this project (id, name, status). No storage paths or credentials.",
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
			})); err != nil {
			return nil, err
		}
	}
	if want[ToolListCollections] {
		if err := add(utils.InferTool(ToolListCollections, "List table/collection names in a database via information_schema.",
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
			})); err != nil {
			return nil, err
		}
	}
	if want[ToolReadonlySQL] {
		if err := add(utils.InferTool(ToolReadonlySQL, "Run a single readonly SQL statement (SELECT/WITH/EXPLAIN/DESCRIBE/SHOW). Writes are rejected.",
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
			})); err != nil {
			return nil, err
		}
	}
	if want[ToolListObjects] {
		if err := add(utils.InferTool(ToolListObjects, "List project object-store keys (relative keys, size, last modified). No credentials.",
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
			})); err != nil {
			return nil, err
		}
	}
	if want[ToolHeadObject] {
		if err := add(utils.InferTool(ToolHeadObject, "Return metadata for one project-relative object key (size, etag, content type). Does not read the body.",
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
			})); err != nil {
			return nil, err
		}
	}
	if want[ToolSearchLogs] {
		if err := add(utils.InferTool(ToolSearchLogs, "Search recent project log events by optional level and substring.",
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
				out := make([]row, 0, len(events))
				for _, e := range events {
					out = append(out, row{
						Level:      e.Level,
						Logger:     e.Logger,
						Message:    truncate(e.Message, 500),
						OccurredAt: e.OccurredAt.UTC().Format(time.RFC3339),
					})
				}
				return marshalToolJSON(out)
			})); err != nil {
			return nil, err
		}
	}
	if want[ToolLogLevelStats] {
		if err := add(utils.InferTool(ToolLogLevelStats, "Count log events in this project grouped by level.",
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
			})); err != nil {
			return nil, err
		}
	}
	if err := buildSandboxTools(&out, want, deps); err != nil {
		return nil, err
	}
	return wrapToolErrors(out), nil
}

// wrapToolErrors 把工具执行错误转换为结构化错误文本（BUG-02）：
// 单个工具失败不应终止整个 run；错误以 is_error=true 的 TOOL_RESULT 回给模型，
// 模型可向用户解释失败原因或换一种方式继续。
// context 取消不转换（需真正终止流）。
func wrapToolErrors(tools []tool.BaseTool) []tool.BaseTool {
	out := make([]tool.BaseTool, 0, len(tools))
	for _, t := range tools {
		inv, ok := t.(tool.InvokableTool)
		if !ok {
			out = append(out, t)
			continue
		}
		out = append(out, &errorTolerantTool{InvokableTool: inv})
	}
	return out
}

// errorTolerantTool 包装 InvokableTool：错误转为 {"error": "..."} 文本。
type errorTolerantTool struct {
	tool.InvokableTool
}

func (w *errorTolerantTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	out, err := w.InvokableTool.InvokableRun(ctx, args, opts...)
	if err == nil {
		return out, nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "", err
	}
	return marshalToolJSON(map[string]any{"error": err.Error(), "is_error": true})
}

func requireRun(ctx context.Context) (RunContext, error) {
	rc, ok := runContextFrom(ctx)
	if !ok || rc.ProjectID == "" {
		return RunContext{}, fmt.Errorf("agent run context missing")
	}
	return rc, nil
}

func marshalToolJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return agentcore.RedactSecrets(string(b)), nil
}
