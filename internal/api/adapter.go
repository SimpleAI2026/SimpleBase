// 文件说明：adapter.go — catalog.Service + registry.Registry 到 api.DatabaseService 的适配器
//
// 用法
//
//	NewDatabaseServiceAdapter(cat CatalogService, reg RegistryService, purger ...
//	type CatalogService（导出类型，同包与上层均可使用）
//	type RegistryService（导出类型，同包与上层均可使用）
//	同包调用：Release() → collection_meta_handler.go, collection_meta_handler.go 等；Query() → collection_meta_handler.go, collection_meta_handler.go 等；Execute() → collection_meta_handler.go, collection_meta_handler.go 等
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/adapter.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase, godatabase/cache, godatabase/registry, internal/audit 等
//	[同包] agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go, auth_handler.go 等 59 个
//	[关键引用] Release() ← collection_meta_handler.go, collection_transfer.go
//
// 主要符号
//
//	type CatalogService interface {…}
//	type RegistryService interface {…}
//	NewDatabaseServiceAdapter(cat CatalogService, reg RegistryService, purger ...
//	type SystemStoreRef interface {…}
//	NewSQLServiceAdapter(cat CatalogService, reg RegistryService, system ...
//	NewCacheService(m *cache.Manager) CacheService
//	... 另有 44 个顶层符号

package api

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/cache"
	"github.com/linkxzhou/SimpleBase/godatabase/registry"
	"github.com/linkxzhou/SimpleBase/internal/audit"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/usage"
	"github.com/voocel/litellm/providers"
)

// CatalogService 是 handler 依赖的 catalog.Service 的最小接口。
type CatalogService interface {
	CreateDatabase(ctx context.Context, in catalog.CreateDatabaseInput) (catalog.Database, error)
	GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error)
	ListDatabases(ctx context.Context, principal auth.Principal, projectID string, page catalog.Page) ([]catalog.Database, string, error)
	BeginDeleteDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error)
	DeleteDatabaseSync(ctx context.Context, principal auth.Principal, projectID, databaseID string, closer func(context.Context, string) error, purger catalog.StoragePurger) (catalog.Database, error)
	ResolveProjectTenant(ctx context.Context, projectID string) (string, error)
	ListProjects(ctx context.Context, principal auth.Principal) ([]catalog.Project, error)
	CreateProject(ctx context.Context, principal auth.Principal, in catalog.CreateProjectInput) (catalog.Project, error)
}

// RegistryService 是 handler 依赖的 registry 的最小接口。
type RegistryService interface {
	Acquire(ctx context.Context, db catalog.Database, mode godatabase.AccessMode) (*registry.Lease, error)
	CloseDatabase(ctx context.Context, databaseID string) error
}

// dbServiceAdapter 把 CatalogService + RegistryService 适配为 DatabaseService。
type dbServiceAdapter struct {
	catalog  CatalogService
	registry RegistryService
	purger   catalog.StoragePurger // 可为 nil（本地模式）
}

// NewDatabaseServiceAdapter 构造 DatabaseService。purger 用于删库时清理项目数据库文件。
func NewDatabaseServiceAdapter(cat CatalogService, reg RegistryService, purger catalog.StoragePurger) DatabaseService {
	return &dbServiceAdapter{catalog: cat, registry: reg, purger: purger}
}

func (a *dbServiceAdapter) CreateDatabase(ctx context.Context, in catalog.CreateDatabaseInput) (catalog.Database, error) {
	return a.catalog.CreateDatabase(ctx, in)
}

func (a *dbServiceAdapter) GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error) {
	return a.catalog.GetDatabase(ctx, principal, projectID, databaseID)
}

func (a *dbServiceAdapter) ListDatabases(ctx context.Context, principal auth.Principal, projectID string, page catalog.Page) ([]catalog.Database, string, error) {
	return a.catalog.ListDatabases(ctx, principal, projectID, page)
}

func (a *dbServiceAdapter) BeginDeleteDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error) {
	return a.catalog.BeginDeleteDatabase(ctx, principal, projectID, databaseID)
}

func (a *dbServiceAdapter) DeleteDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error) {
	var purger catalog.StoragePurger
	if a.purger != nil {
		purger = a.purger
	}
	return a.catalog.DeleteDatabaseSync(ctx, principal, projectID, databaseID, a.registry.CloseDatabase, purger)
}

// sqlServiceAdapter 把 CatalogService + RegistryService 适配为 SQLService。
type sqlServiceAdapter struct {
	catalog  CatalogService
	registry RegistryService
	system   SystemStoreRef // 可为 nil；系统库桥接
}

// SystemStoreRef 是系统库常驻连接的最小引用（*systemdb.Store 满足）。
type SystemStoreRef interface {
	Meta() catalog.Database
	DB() *sql.DB
}

// NewSQLServiceAdapter 构造 SQLService。
// system 可为 nil（测试）；非 nil 时系统库的 Acquire 桥接到 systemdb 常驻连接，
// 不再经 registry 打开第二实例（用户 factory 的 CacheDir 与系统库不同，会得到空 catalog）。
func NewSQLServiceAdapter(cat CatalogService, reg RegistryService, system SystemStoreRef) SQLService {

	return &sqlServiceAdapter{catalog: cat, registry: reg, system: system}
}

func (a *sqlServiceAdapter) GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error) {
	timer := StageTimerFrom(ctx)
	scope := timer.StageScope(StageCatalog)
	defer scope.Done()
	return a.catalog.GetDatabase(ctx, principal, projectID, databaseID)
}

func (a *sqlServiceAdapter) Acquire(ctx context.Context, db catalog.Database, mode godatabase.AccessMode) (SQLLease, error) {
	// 系统库：桥接到 systemdb 常驻连接（只读查询），不经 registry。
	if catalog.IsSystemDatabase(db) && a.system != nil && mode == godatabase.ReadOnly {
		return WrapTimedLease(ctx, &systemLeaseAdapter{conn: a.system.DB()}), nil
	}
	timer := StageTimerFrom(ctx)
	acquireScope := timer.StageScope(StageAcquire)
	l, err := a.registry.Acquire(ctx, db, mode)
	acquireScope.Done()
	if err != nil {
		return nil, err
	}
	return WrapTimedLease(ctx, &sqlLeaseAdapter{lease: l}), nil
}

// ListDatabases 使 sqlServiceAdapter 同时满足 DataService。
// §7.2 M1：catalog 列表查询计入 catalog_lookup 段。
func (a *sqlServiceAdapter) ListDatabases(ctx context.Context, principal auth.Principal, projectID string, page catalog.Page) ([]catalog.Database, string, error) {
	timer := StageTimerFrom(ctx)
	scope := timer.StageScope(StageCatalog)
	defer scope.Done()
	return a.catalog.ListDatabases(ctx, principal, projectID, page)
}

// systemLeaseAdapter 包装系统库常驻 *sql.DB 为只读租约。
// Release 不关闭连接（连接由 systemdb.Store 管理生命周期）。
type systemLeaseAdapter struct {
	conn *sql.DB
}

func (s *systemLeaseAdapter) Release() {}

func (s *systemLeaseAdapter) Query(ctx context.Context, stmt godatabase.Statement, maxRows int) (godatabase.QueryResult, error) {
	return godatabase.Query(ctx, s.conn, stmt, maxRows)
}

func (s *systemLeaseAdapter) Execute(ctx context.Context, stmt godatabase.Statement) (godatabase.QueryResult, error) {
	return godatabase.QueryResult{}, catalog.ErrSystemProtected
}

func (s *systemLeaseAdapter) Batch(ctx context.Context, stmts []godatabase.Statement, transactional bool) ([]godatabase.QueryResult, error) {
	return nil, catalog.ErrSystemProtected
}

// Raw 返回系统库常驻连接（只读用途；写由 Execute/Batch 层拒绝）。
func (s *systemLeaseAdapter) Raw() *sql.DB { return s.conn }

// NotifyWrite 对系统库是 no-op（系统库写已被拒绝，不存在外部写路径）。
func (s *systemLeaseAdapter) NotifyWrite(ctx context.Context) {}

// sqlLeaseAdapter 包装 registry.Lease，暴露 Handle 的 Query/Execute/Batch。
type sqlLeaseAdapter struct {
	lease *registry.Lease
}

func (s *sqlLeaseAdapter) Release() {
	s.lease.Release()
}

func (s *sqlLeaseAdapter) Query(ctx context.Context, stmt godatabase.Statement, maxRows int) (godatabase.QueryResult, error) {
	return s.lease.Handle.Query(ctx, stmt, maxRows)
}

func (s *sqlLeaseAdapter) Execute(ctx context.Context, stmt godatabase.Statement) (godatabase.QueryResult, error) {
	return s.lease.Handle.Execute(ctx, stmt)
}

func (s *sqlLeaseAdapter) Batch(ctx context.Context, stmts []godatabase.Statement, transactional bool) ([]godatabase.QueryResult, error) {
	return s.lease.Handle.Batch(ctx, stmts, transactional)
}

// Raw 返回底层 *sql.DB（KV 仓库自管事务用；连接生命周期由 Handle 管理）。
func (s *sqlLeaseAdapter) Raw() *sql.DB { return s.lease.Handle.Conn() }

// NotifyWrite 触发 Handle 的 onWrite 回调（CatalogSyncer.MarkDirty 等）。
func (s *sqlLeaseAdapter) NotifyWrite(ctx context.Context) { s.lease.Handle.NotifyWrite(ctx) }

// cacheServiceAdapter 适配 *cache.Manager。
type cacheServiceAdapter struct{ m *cache.Manager }

func (a *cacheServiceAdapter) Usage(ctx context.Context) (CacheUsage, error) {
	u, err := a.m.Usage(ctx)
	if err != nil {
		return CacheUsage{}, err
	}
	return CacheUsage{TotalBytes: u.TotalBytes, DatabaseDirs: u.DatabaseDirs}, nil
}

// NewCacheService 构造 CacheService 适配器。
func NewCacheService(m *cache.Manager) CacheService {
	if m == nil {
		return nil
	}
	return &cacheServiceAdapter{m: m}
}

// usageServiceAdapter 适配 *usage.Service。
type usageServiceAdapter struct{ s *usage.Service }

func (a *usageServiceAdapter) CheckQuota(ctx context.Context, projectID string, kind string) error {
	return a.s.CheckQuota(ctx, projectID, kind)
}

// NewUsageService 构造 UsageService 适配器。
func NewUsageService(s *usage.Service) UsageService {
	if s == nil {
		return nil
	}
	return &usageServiceAdapter{s: s}
}

// auditServiceAdapter 适配 *audit.Service。
type auditServiceAdapter struct{ s *audit.Service }

func (a *auditServiceAdapter) Record(ctx context.Context, e AuditEvent) error {
	return a.s.Record(ctx, audit.Event{
		DatabaseID:  e.DatabaseID,
		ProjectID:   e.ProjectID,
		PrincipalID: e.PrincipalID,
		Kind:        e.Kind,
		RequestID:   e.RequestID,
		Status:      e.Status,
		Detail:      e.Detail,
	})
}

func (a *auditServiceAdapter) ListOperations(ctx context.Context, projectID, databaseID string, limit int) ([]catalog.Operation, error) {
	return a.s.ListOperations(ctx, audit.Query{ProjectID: projectID, DatabaseID: databaseID, Limit: limit})
}

// NewAuditService 构造 AuditService 适配器。
func NewAuditService(s *audit.Service) AuditService {
	if s == nil {
		return nil
	}
	return &auditServiceAdapter{s: s}
}

// llmServiceAdapter 适配 llmgateway.Service。
type llmServiceAdapter struct{ s llmgateway.Service }

func (a *llmServiceAdapter) Chat(ctx context.Context, projectID string, req LLMRequest) (LLMResponse, error) {
	resp, err := a.s.Chat(ctx, projectID, toGatewayRequest(req))
	if err != nil {
		return LLMResponse{}, err
	}
	out := LLMResponse{
		Content: resp.Content,
		Usage: LLMTokenUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
			ReasoningTokens:  resp.Usage.ReasoningTokens,
		},
		Model:        resp.Model,
		Provider:     resp.Provider,
		FinishReason: resp.FinishReason,
	}
	for _, tc := range resp.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, LLMToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
	}
	return out, nil
}

func (a *llmServiceAdapter) Stream(ctx context.Context, projectID string, req LLMRequest) (LLMStreamReader, error) {
	reader, err := a.s.Stream(ctx, projectID, toGatewayRequest(req))
	if err != nil {
		return nil, err
	}
	return &llmStreamReaderAdapter{inner: reader}, nil
}

// toGatewayRequest 把 api 层请求映射为 litellm 形状；工具与工具往返消息原样透传。
func toGatewayRequest(req LLMRequest) llmgateway.Request {
	msgs := make([]providers.Message, len(req.Messages))
	for i, m := range req.Messages {
		pm := providers.Message{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			pm.ToolCalls = append(pm.ToolCalls, providers.ToolCall{
				ID: tc.ID, Type: "function",
				Function: providers.FunctionCall{Name: tc.Name, Arguments: tc.Arguments},
			})
		}
		msgs[i] = pm
	}
	out := llmgateway.Request{
		Model:       req.Model,
		Messages:    msgs,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		ToolChoice:  decodeToolChoice(req.ToolChoice),
	}
	for _, t := range req.Tools {
		var params any = map[string]any{"type": "object", "properties": map[string]any{}}
		if len(t.Parameters) > 0 {
			params = t.Parameters
		}
		out.Tools = append(out.Tools, providers.Tool{
			Type:     "function",
			Function: providers.FunctionDef{Name: t.Name, Description: t.Description, Parameters: params},
		})
	}
	return out
}

// decodeToolChoice 把 HTTP 层保存的 tool_choice JSON 还原成 litellm 接受的 any。
//
// tool_choice 要么是字符串（"auto"/"none"/"required"），要么是
// {"type":"function","function":{"name":"..."}} 对象，json 解码到 any 即可保持原形。
// 空值或非法 JSON 都返回 nil（不指定）：非法值应由 handler 入口校验并以 400 拒绝，
// 这里不再重复报错，避免改动 toGatewayRequest 的签名而波及所有调用方与测试桩。
func decodeToolChoice(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}

func (a *llmServiceAdapter) ListProviders(ctx context.Context, projectID string) ([]string, error) {
	return a.s.ListProviders(ctx, projectID)
}

// ProviderModels 暴露各 provider 可用模型名（无 base_url/api_key），
// 供云 Agent 模型列表（planv4.1 BUG-07）。
func (a *llmServiceAdapter) ProviderModels(ctx context.Context, projectID string) (map[string][]string, error) {
	return a.s.ProviderModels(ctx, projectID)
}

// NewLLMService 构造 LLMService 适配器。
func NewLLMService(s llmgateway.Service) LLMService {
	if s == nil {
		return nil
	}
	return &llmServiceAdapter{s: s}
}

// llmStreamReaderAdapter 适配 llmgateway.StreamReader。
type llmStreamReaderAdapter struct{ inner llmgateway.StreamReader }

func (a *llmStreamReaderAdapter) Next() (*LLMStreamChunk, error) {
	chunk, err := a.inner.Next()
	if err != nil {
		return nil, err
	}
	out := &LLMStreamChunk{
		Type:         chunk.Type,
		Content:      chunk.Content,
		FinishReason: chunk.FinishReason,
		Done:         chunk.Done,
	}
	if d := chunk.ToolCallDelta; d != nil {
		out.ToolCallIndex = d.Index
		out.ToolCallID = d.ID
		out.ToolCallName = d.FunctionName
		out.ToolCallArgs = d.ArgumentsDelta
	}
	if u := chunk.Usage; u != nil {
		out.Usage = &LLMTokenUsage{
			PromptTokens: u.PromptTokens, CompletionTokens: u.CompletionTokens,
			TotalTokens: u.TotalTokens, ReasoningTokens: u.ReasoningTokens,
		}
	}
	return out, nil
}

func (a *llmStreamReaderAdapter) Close() error { return a.inner.Close() }
