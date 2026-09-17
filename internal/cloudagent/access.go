package cloudagent

import (
	"context"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// ChatMessage is a gateway-shaped chat turn (no secrets).
type ChatMessage struct {
	Role    string
	Content string
}

// ChatRequest is sent to the existing LLM gateway.
type ChatRequest struct {
	Model       string
	Messages    []ChatMessage
	MaxTokens   *int
	Temperature *float64
}

// ChatResponse is a non-stream completion.
type ChatResponse struct {
	Content      string
	Model        string
	Provider     string
	FinishReason string
}

// TokenStream is a streaming completion reader.
type TokenStream interface {
	Next() (content string, finish bool, err error)
	Close() error
}

// ChatClient is the LLM gateway surface used by the eino ChatModel adapter.
type ChatClient interface {
	Chat(ctx context.Context, projectID string, req ChatRequest) (ChatResponse, error)
	Stream(ctx context.Context, projectID string, req ChatRequest) (TokenStream, error)
}

// DatabaseInfo is a non-secret catalog row for tools/snapshots.
type DatabaseInfo struct {
	ID     string
	Name   string
	Status string
}

// SQLResult is a truncated readonly query result.
type SQLResult struct {
	Columns  []string
	Rows     [][]any
	RowCount int
}

// ObjectInfo is a project-relative object (no physical prefix, no credentials).
type ObjectInfo struct {
	Key          string
	Size         int64
	ETag         string
	ContentType  string
	LastModified string
}

// DatabaseAccess is the readonly database surface for tools.
type DatabaseAccess interface {
	ListDatabases(ctx context.Context, principal auth.Principal, projectID string) ([]DatabaseInfo, error)
	ListCollections(ctx context.Context, principal auth.Principal, projectID, databaseID string) ([]string, error)
	ReadOnlyQuery(ctx context.Context, principal auth.Principal, projectID, databaseID, sqlText string, maxRows int) (SQLResult, error)
}

// ObjectAccess is the readonly S3 surface for tools.
type ObjectAccess interface {
	ListObjects(ctx context.Context, projectID, prefix string, limit int) ([]ObjectInfo, error)
	HeadObject(ctx context.Context, projectID, key string) (ObjectInfo, error)
}

// LogAccess is the readonly logs surface for tools.
type LogAccess interface {
	SearchLogs(ctx context.Context, projectID, level, q string, limit int) ([]systemdb.LogEvent, error)
	LevelStats(ctx context.Context, projectID string) ([]systemdb.LogLevelCount, error)
}

// SettingsAccess reads project LLM defaults (no keys).
type SettingsAccess interface {
	LLMSettings(ctx context.Context, projectID string) (systemdb.LLMSettings, error)
}
