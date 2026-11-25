// 文件说明：ports.go — internal/agentbridge 中 DatabaseInfo、SQLResult 等符号的实现
//
// 用法
//
//	type DatabaseInfo struct{…}
//	type SQLResult struct{…}
//	type ObjectInfo struct{…}
//	type DatabaseAccess interface{…}
//	type ObjectAccess interface{…}
//	type LogReader interface{…}
//
// 调用架构
//
//	[上游] internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/ports.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] access_db.go, access_llm.go, access_object.go, access_test.go, bridge_test.go 等 33 个
//	[关键引用] DatabaseInfo ← access_db.go, testutil_test.go；SQLResult ← access_db.go, testutil_test.go；ObjectInfo ← access_object.go, testutil_test.go
//
// 主要符号
//
//	type DatabaseInfo struct{…}
//	type SQLResult struct{…}
//	type ObjectInfo struct{…}
//	type DatabaseAccess interface{…}
//	type ObjectAccess interface{…}
//	type LogReader interface{…}
//	type LogEvent struct{…}
//	type LogLevelCount struct{…}
//	... 另有 6 个顶层符号

package agentbridge

import (
	"context"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

type DatabaseInfo struct {
	ID     string
	Name   string
	Status string
}

type SQLResult struct {
	Columns  []string
	Rows     [][]any
	RowCount int
}

type ObjectInfo struct {
	Key          string
	Size         int64
	ETag         string
	ContentType  string
	LastModified string
}

type DatabaseAccess interface {
	ListDatabases(ctx context.Context, principal auth.Principal, projectID string) ([]DatabaseInfo, error)
	ListCollections(ctx context.Context, principal auth.Principal, projectID, databaseID string) ([]string, error)
	ReadOnlyQuery(ctx context.Context, principal auth.Principal, projectID, databaseID, sqlText string, maxRows int) (SQLResult, error)
}

type ObjectAccess interface {
	ListObjects(ctx context.Context, projectID, prefix string, limit int) ([]ObjectInfo, error)
	HeadObject(ctx context.Context, projectID, key string) (ObjectInfo, error)
}

type LogReader interface {
	SearchLogs(ctx context.Context, projectID, level, q string, limit int) ([]LogEvent, error)
	LevelStats(ctx context.Context, projectID string) ([]LogLevelCount, error)
}

type LogEvent struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	Level      string    `json:"level"`
	Logger     string    `json:"logger"`
	Message    string    `json:"message"`
	FieldsJSON string    `json:"fields_json,omitempty"`
	RequestID  string    `json:"request_id,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

type LogLevelCount struct {
	Level string
	Count int64
}

type LLMSettings struct {
	ProjectID    string
	DefaultModel string
}

type SettingsReader interface {
	LLMSettings(ctx context.Context, projectID string) (LLMSettings, error)
}

type SandboxOutput struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type Sandbox interface {
	Available() bool
	Exec(ctx context.Context, projectID, threadID, cmd string, args []string) (SandboxOutput, error)
	Shell(ctx context.Context, projectID, threadID, command string) (SandboxOutput, error)
	ReadFile(ctx context.Context, projectID, threadID, path string) (string, error)
	WriteFile(ctx context.Context, projectID, threadID, path, content string) error
	ReleaseThread(ctx context.Context, projectID, threadID string) error
}

type CommandClassifier interface {
	Classify(argv []string, stdin string) (cloudagent.ToolDecision, error)
	RedactArgv(argv []string) []string
}

type DelegationTokens interface {
	Issue(p auth.Principal, projectID, runID string, ttl time.Duration) (string, error)
	RevokeRun(runID string)
}
