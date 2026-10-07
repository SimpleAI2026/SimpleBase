package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/database/sqlguard"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

type cloudAgentDB struct {
	catalog  CatalogService
	registry RegistryService
}

func NewCloudAgentDB(cat CatalogService, reg RegistryService) cloudagent.DatabaseAccess {
	return newCloudAgentDB(cat, reg)
}

func NewCloudAgentObj(files objectstore.FileStore, index *systemdb.Store) cloudagent.ObjectAccess {
	return newCloudAgentObj(files, index)
}

func NewCloudAgentLLM(svc LLMService) cloudagent.ChatClient {
	return newLLMChatClient(svc)
}

func newCloudAgentDB(cat CatalogService, reg RegistryService) cloudagent.DatabaseAccess {
	if cat == nil || reg == nil {
		return nil
	}
	return &cloudAgentDB{catalog: cat, registry: reg}
}

func (a *cloudAgentDB) ListDatabases(ctx context.Context, principal auth.Principal, projectID string) ([]cloudagent.DatabaseInfo, error) {
	list, _, err := a.catalog.ListDatabases(ctx, principal, projectID, catalog.Page{Limit: 50})
	if err != nil {
		return nil, err
	}
	out := make([]cloudagent.DatabaseInfo, 0, len(list))
	for _, d := range list {
		out = append(out, cloudagent.DatabaseInfo{ID: d.ID, Name: d.Name, Status: string(d.Status)})
	}
	return out, nil
}

func (a *cloudAgentDB) ListCollections(ctx context.Context, principal auth.Principal, projectID, databaseID string) ([]string, error) {
	db, err := a.catalog.GetDatabase(ctx, principal, projectID, databaseID)
	if err != nil {
		return nil, err
	}
	lease, err := a.registry.Acquire(ctx, db, database.ReadOnly)
	if err != nil {
		return nil, err
	}
	defer lease.Release()
	result, err := lease.Handle.Query(ctx, database.Statement{SQL: `SELECT table_name FROM information_schema.tables
		WHERE table_catalog = current_database()
		  AND table_schema = current_schema()
		  AND table_type = 'BASE TABLE'
		ORDER BY table_name`}, 200)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(result.Rows))
	for _, row := range result.Rows {
		if len(row) > 0 {
			if name, ok := row[0].(string); ok {
				names = append(names, name)
			}
		}
	}
	return names, nil
}

func (a *cloudAgentDB) ReadOnlyQuery(ctx context.Context, principal auth.Principal, projectID, databaseID, sqlText string, maxRows int) (cloudagent.SQLResult, error) {
	if err := sqlguard.Validate(sqlText, sqlguard.ReadOnly); err != nil {
		return cloudagent.SQLResult{}, err
	}
	db, err := a.catalog.GetDatabase(ctx, principal, projectID, databaseID)
	if err != nil {
		return cloudagent.SQLResult{}, err
	}
	lease, err := a.registry.Acquire(ctx, db, database.ReadOnly)
	if err != nil {
		return cloudagent.SQLResult{}, err
	}
	defer lease.Release()
	if maxRows <= 0 || maxRows > 100 {
		maxRows = 50
	}
	result, err := lease.Handle.Query(ctx, database.Statement{SQL: sqlText}, maxRows)
	if err != nil {
		return cloudagent.SQLResult{}, err
	}
	rows, err := database.SerializeRows(result.Rows)
	if err != nil {
		return cloudagent.SQLResult{}, err
	}
	return cloudagent.SQLResult{Columns: result.Columns, Rows: rows, RowCount: len(rows)}, nil
}

type cloudAgentObj struct {
	files objectstore.FileStore
	index *systemdb.Store
}

func newCloudAgentObj(files objectstore.FileStore, index *systemdb.Store) cloudagent.ObjectAccess {
	if files == nil && index == nil {
		return nil
	}
	return &cloudAgentObj{files: files, index: index}
}

func (a *cloudAgentObj) ListObjects(ctx context.Context, projectID, prefix string, limit int) ([]cloudagent.ObjectInfo, error) {
	if a.index != nil {
		rows, err := a.index.ListS3Objects(ctx, projectID, prefix, limit)
		if err != nil {
			return nil, err
		}
		out := make([]cloudagent.ObjectInfo, 0, len(rows))
		for _, o := range rows {
			out = append(out, toAgentObjectInfo(o))
		}
		if len(out) > 0 || a.files == nil {
			return out, nil
		}
	}
	if a.files == nil {
		return nil, fmt.Errorf("object store is not configured")
	}
	listed, err := a.files.List(ctx, joinKey(projectID, prefix), limit)
	if err != nil {
		return nil, err
	}
	out := make([]cloudagent.ObjectInfo, 0, len(listed))
	for _, o := range listed {
		lm := ""
		if !o.LastModified.IsZero() {
			lm = o.LastModified.UTC().Format(time.RFC3339)
		}
		out = append(out, cloudagent.ObjectInfo{
			Key:          stripProject(projectID, o.Key),
			Size:         o.Size,
			LastModified: lm,
		})
	}
	return out, nil
}

func (a *cloudAgentObj) HeadObject(ctx context.Context, projectID, key string) (cloudagent.ObjectInfo, error) {
	if err := objectstore.ValidateFileKey(key); err != nil {
		return cloudagent.ObjectInfo{}, err
	}
	if a.index != nil {
		o, err := a.index.GetS3Object(ctx, projectID, key)
		if err == nil {
			return toAgentObjectInfo(o), nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return cloudagent.ObjectInfo{}, err
		}
	}
	if a.files == nil {
		return cloudagent.ObjectInfo{}, fmt.Errorf("object %s not found", key)
	}
	listed, err := a.files.List(ctx, joinKey(projectID, key), 16)
	if err != nil {
		return cloudagent.ObjectInfo{}, err
	}
	for _, o := range listed {
		rel := stripProject(projectID, o.Key)
		if rel == key || o.Key == joinKey(projectID, key) {
			lm := ""
			if !o.LastModified.IsZero() {
				lm = o.LastModified.UTC().Format(time.RFC3339)
			}
			return cloudagent.ObjectInfo{Key: key, Size: o.Size, LastModified: lm}, nil
		}
	}
	return cloudagent.ObjectInfo{}, fmt.Errorf("object %s not found", key)
}

func toAgentObjectInfo(o systemdb.S3Object) cloudagent.ObjectInfo {
	info := cloudagent.ObjectInfo{Key: o.Key, Size: o.Size, ETag: o.ETag, ContentType: o.ContentType}
	if !o.LastModified.IsZero() {
		info.LastModified = o.LastModified.UTC().Format(time.RFC3339)
	}
	return info
}

type llmChatClient struct{ svc LLMService }

func newLLMChatClient(svc LLMService) cloudagent.ChatClient {
	if svc == nil {
		return nil
	}
	return &llmChatClient{svc: svc}
}

func toLLMRequest(req cloudagent.ChatRequest) LLMRequest {
	out := LLMRequest{Model: req.Model, MaxTokens: req.MaxTokens, Temperature: req.Temperature}
	for _, m := range req.Messages {
		msg := LLMMessage{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, LLMToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
		}
		out.Messages = append(out.Messages, msg)
	}
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, LLMTool{Name: t.Name, Description: t.Description, Parameters: t.Parameters})
	}
	return out
}

func (c *llmChatClient) Chat(ctx context.Context, projectID string, req cloudagent.ChatRequest) (cloudagent.ChatResponse, error) {
	resp, err := c.svc.Chat(ctx, projectID, toLLMRequest(req))
	if err != nil {
		return cloudagent.ChatResponse{}, err
	}
	out := cloudagent.ChatResponse{Content: resp.Content, Model: resp.Model, Provider: resp.Provider, FinishReason: resp.FinishReason,
		Usage: cloudagent.ChatUsage{PromptTokens: resp.Usage.PromptTokens, CompletionTokens: resp.Usage.CompletionTokens, ReasoningTokens: resp.Usage.ReasoningTokens}}
	for _, tc := range resp.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, cloudagent.ChatToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
	}
	return out, nil
}

func (c *llmChatClient) Stream(ctx context.Context, projectID string, req cloudagent.ChatRequest) (cloudagent.TokenStream, error) {
	reader, err := c.svc.Stream(ctx, projectID, toLLMRequest(req))
	if err != nil {
		return nil, err
	}
	return &llmTokenStream{r: reader}, nil
}

type llmTokenStream struct{ r LLMStreamReader }

func (s *llmTokenStream) NextDelta() (cloudagent.StreamDelta, error) {
	chunk, err := s.r.Next()
	if err != nil {
		return cloudagent.StreamDelta{}, err
	}
	if chunk == nil {
		return cloudagent.StreamDelta{}, nil
	}
	out := cloudagent.StreamDelta{Content: chunk.Content, Finish: chunk.Done || chunk.Type == "end"}
	if chunk.Type == "tool_call_delta" {
		out.ToolCall = &cloudagent.ToolCallDelta{Index: chunk.ToolCallIndex, ID: chunk.ToolCallID, Name: chunk.ToolCallName, ArgsDelta: chunk.ToolCallArgs}
	}
	if chunk.Usage != nil {
		out.Usage = &cloudagent.ChatUsage{PromptTokens: chunk.Usage.PromptTokens, CompletionTokens: chunk.Usage.CompletionTokens, ReasoningTokens: chunk.Usage.ReasoningTokens}
	}
	return out, nil
}

func (s *llmTokenStream) Next() (string, bool, error) {
	chunk, err := s.r.Next()
	if err != nil {
		return "", true, err
	}
	if chunk == nil {
		return "", false, nil
	}
	return chunk.Content, chunk.Done || chunk.Type == "end" || chunk.FinishReason != "", nil
}

func (s *llmTokenStream) Close() error { return s.r.Close() }
