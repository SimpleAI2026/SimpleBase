// 文件说明：gocloudagent_access.go — internal/api 中 NewCloudAgentDB、NewCloudAgentObj 等符号的实现
//
// 用法
//
//	NewCloudAgentDB(cat CatalogService, reg RegistryService) gocloudagent.DatabaseAccess
//	type cloudAgentDB（包内使用）
//	type cloudAgentObj（包内使用）
//	同包调用：Close() → adapter.go, api_keys_handler.go 等；Stream() → adapter.go, agent_handler.go 等；Next() → adapter.go, api_keys_handler.go 等
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/gocloudagent_access.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent, internal/agentbridge, internal/catalog, godatabase 等
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] Close() ← adapter.go, api_keys_handler.go, backup_handler.go
//
// 主要符号
//
//	NewCloudAgentDB(cat CatalogService, reg RegistryService) gocloudagent.DatabaseAccess
//	NewCloudAgentObj(files objectstore.FileStore, index *systemdb.Store) ...
//	NewCloudAgentLLM(svc LLMService) gocloudagent.ChatClient
//	... 另有 19 个顶层符号

package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlguard"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

type cloudAgentDB struct {
	catalog  CatalogService
	registry RegistryService
}

func NewCloudAgentDB(cat CatalogService, reg RegistryService) gocloudagent.DatabaseAccess {
	return newCloudAgentDB(cat, reg)
}

func NewCloudAgentObj(files objectstore.FileStore, index *systemdb.Store) gocloudagent.ObjectAccess {
	return newCloudAgentObj(files, index)
}

func NewCloudAgentLLM(svc LLMService) gocloudagent.ChatClient {
	return newLLMChatClient(svc)
}

func newCloudAgentDB(cat CatalogService, reg RegistryService) gocloudagent.DatabaseAccess {
	if cat == nil || reg == nil {
		return nil
	}
	return &cloudAgentDB{catalog: cat, registry: reg}
}

func (a *cloudAgentDB) ListDatabases(ctx context.Context, principal gocloudagent.Principal, projectID string) ([]gocloudagent.DatabaseInfo, error) {
	// 还原成宿主身份：catalog 的项目授权依赖 Role / TenantID / ProjectIDs。
	list, _, err := a.catalog.ListDatabases(ctx, agentbridge.ToHostPrincipal(principal), projectID, catalog.Page{Limit: 50})
	if err != nil {
		return nil, err
	}
	out := make([]gocloudagent.DatabaseInfo, 0, len(list))
	for _, d := range list {
		out = append(out, gocloudagent.DatabaseInfo{ID: d.ID, Name: d.Name, Status: string(d.Status)})
	}
	return out, nil
}

func (a *cloudAgentDB) ListCollections(ctx context.Context, principal gocloudagent.Principal, projectID, databaseID string) ([]string, error) {
	db, err := a.catalog.GetDatabase(ctx, agentbridge.ToHostPrincipal(principal), projectID, databaseID)
	if err != nil {
		return nil, err
	}
	lease, err := a.registry.Acquire(ctx, db, godatabase.ReadOnly)
	if err != nil {
		return nil, err
	}
	defer lease.Release()
	dialect, err := dialectForDatabase(db)
	if err != nil {
		return nil, err
	}
	result, err := lease.Handle.Query(ctx, godatabase.Statement{SQL: dialect.TableColumnsSQL("")}, 200*8)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(result.Rows))
	seen := make(map[string]struct{}, len(result.Rows))
	for _, row := range result.Rows {
		if len(row) == 0 {
			continue
		}
		name, ok := row[0].(string)
		if !ok || name == "" || dialect.IsInternalTable(name) {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names, nil
}

func (a *cloudAgentDB) ReadOnlyQuery(ctx context.Context, principal gocloudagent.Principal, projectID, databaseID, sqlText string, maxRows int) (gocloudagent.SQLResult, error) {
	if err := sqlguard.Validate(sqlText, sqlguard.ReadOnly); err != nil {
		return gocloudagent.SQLResult{}, err
	}
	db, err := a.catalog.GetDatabase(ctx, agentbridge.ToHostPrincipal(principal), projectID, databaseID)
	if err != nil {
		return gocloudagent.SQLResult{}, err
	}
	lease, err := a.registry.Acquire(ctx, db, godatabase.ReadOnly)
	if err != nil {
		return gocloudagent.SQLResult{}, err
	}
	defer lease.Release()
	if maxRows <= 0 || maxRows > 100 {
		maxRows = 50
	}
	result, err := lease.Handle.Query(ctx, godatabase.Statement{SQL: sqlText}, maxRows)
	if err != nil {
		return gocloudagent.SQLResult{}, err
	}
	rows, err := godatabase.SerializeRows(result.Rows)
	if err != nil {
		return gocloudagent.SQLResult{}, err
	}
	return gocloudagent.SQLResult{Columns: result.Columns, Rows: rows, RowCount: len(rows)}, nil
}

type cloudAgentObj struct {
	files objectstore.FileStore
	index *systemdb.Store
}

func newCloudAgentObj(files objectstore.FileStore, index *systemdb.Store) gocloudagent.ObjectAccess {
	if files == nil && index == nil {
		return nil
	}
	return &cloudAgentObj{files: files, index: index}
}

func (a *cloudAgentObj) ListObjects(ctx context.Context, projectID, prefix string, limit int) ([]gocloudagent.ObjectInfo, error) {
	if a.index != nil {
		rows, err := a.index.ListS3Objects(ctx, projectID, prefix, limit)
		if err != nil {
			return nil, err
		}
		out := make([]gocloudagent.ObjectInfo, 0, len(rows))
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
	out := make([]gocloudagent.ObjectInfo, 0, len(listed))
	for _, o := range listed {
		lm := ""
		if !o.LastModified.IsZero() {
			lm = o.LastModified.UTC().Format(time.RFC3339)
		}
		out = append(out, gocloudagent.ObjectInfo{
			Key:          stripProject(projectID, o.Key),
			Size:         o.Size,
			LastModified: lm,
		})
	}
	return out, nil
}

func (a *cloudAgentObj) HeadObject(ctx context.Context, projectID, key string) (gocloudagent.ObjectInfo, error) {
	if err := objectstore.ValidateFileKey(key); err != nil {
		return gocloudagent.ObjectInfo{}, err
	}
	if a.index != nil {
		o, err := a.index.GetS3Object(ctx, projectID, key)
		if err == nil {
			return toAgentObjectInfo(o), nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return gocloudagent.ObjectInfo{}, err
		}
	}
	if a.files == nil {
		return gocloudagent.ObjectInfo{}, fmt.Errorf("object %s not found", key)
	}
	listed, err := a.files.List(ctx, joinKey(projectID, key), 16)
	if err != nil {
		return gocloudagent.ObjectInfo{}, err
	}
	for _, o := range listed {
		rel := stripProject(projectID, o.Key)
		if rel == key || o.Key == joinKey(projectID, key) {
			lm := ""
			if !o.LastModified.IsZero() {
				lm = o.LastModified.UTC().Format(time.RFC3339)
			}
			return gocloudagent.ObjectInfo{Key: key, Size: o.Size, LastModified: lm}, nil
		}
	}
	return gocloudagent.ObjectInfo{}, fmt.Errorf("object %s not found", key)
}

func toAgentObjectInfo(o systemdb.S3Object) gocloudagent.ObjectInfo {
	info := gocloudagent.ObjectInfo{Key: o.Key, Size: o.Size, ETag: o.ETag, ContentType: o.ContentType}
	if !o.LastModified.IsZero() {
		info.LastModified = o.LastModified.UTC().Format(time.RFC3339)
	}
	return info
}

type llmChatClient struct{ svc LLMService }

func newLLMChatClient(svc LLMService) gocloudagent.ChatClient {
	if svc == nil {
		return nil
	}
	return &llmChatClient{svc: svc}
}

func toLLMRequest(req gocloudagent.ChatRequest) LLMRequest {
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

func (c *llmChatClient) Chat(ctx context.Context, projectID string, req gocloudagent.ChatRequest) (gocloudagent.ChatResponse, error) {
	resp, err := c.svc.Chat(ctx, projectID, toLLMRequest(req))
	if err != nil {
		return gocloudagent.ChatResponse{}, err
	}
	out := gocloudagent.ChatResponse{Content: resp.Content, Model: resp.Model, Provider: resp.Provider, FinishReason: resp.FinishReason,
		Usage: gocloudagent.ChatUsage{PromptTokens: resp.Usage.PromptTokens, CompletionTokens: resp.Usage.CompletionTokens, ReasoningTokens: resp.Usage.ReasoningTokens}}
	for _, tc := range resp.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, gocloudagent.ChatToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
	}
	return out, nil
}

func (c *llmChatClient) Stream(ctx context.Context, projectID string, req gocloudagent.ChatRequest) (gocloudagent.TokenStream, error) {
	reader, err := c.svc.Stream(ctx, projectID, toLLMRequest(req))
	if err != nil {
		return nil, err
	}
	return &llmTokenStream{r: reader}, nil
}

type llmTokenStream struct{ r LLMStreamReader }

func (s *llmTokenStream) NextDelta() (gocloudagent.StreamDelta, error) {
	chunk, err := s.r.Next()
	if err != nil {
		return gocloudagent.StreamDelta{}, err
	}
	if chunk == nil {
		return gocloudagent.StreamDelta{}, nil
	}
	out := gocloudagent.StreamDelta{Content: chunk.Content, Finish: chunk.Done || chunk.Type == "end"}
	if chunk.Type == "tool_call_delta" {
		out.ToolCall = &gocloudagent.ToolCallDelta{Index: chunk.ToolCallIndex, ID: chunk.ToolCallID, Name: chunk.ToolCallName, ArgsDelta: chunk.ToolCallArgs}
	}
	if chunk.Usage != nil {
		out.Usage = &gocloudagent.ChatUsage{PromptTokens: chunk.Usage.PromptTokens, CompletionTokens: chunk.Usage.CompletionTokens, ReasoningTokens: chunk.Usage.ReasoningTokens}
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
