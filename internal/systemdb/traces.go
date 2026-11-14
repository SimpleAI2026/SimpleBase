// Package systemdb 提供系统库 sys_llm_traces 的写缓冲与查询（planv6.5 model-settings-and-trace-plan §3.1）。
package systemdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/linkxzhou/SimpleBase/internal/utils/limit"
	"github.com/linkxzhou/SimpleBase/internal/utils/sqlval"
)

// LLMTrace 是 sys_llm_traces 一行：一次模型调用（Chat 或 Stream）或一次工具执行的观测记录。
// 二期（planv6.5 §9）扩为 span 模型：trace_id 聚合顶层调用，span_kind 区分 llm/tool。
type LLMTrace struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"project_id"`
	RequestID    string    `json:"request_id"`
	Source       string    `json:"source"` // "llm"；一期遗留列，读取侧 caller 空时回退
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	Status       string    `json:"status"` // "ok" | "error"
	Stream       bool      `json:"stream"`
	DurationMs   int64     `json:"duration_ms"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	ErrorCode    string    `json:"error_code,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
	TraceID      string    `json:"trace_id"`              // 顶层 trace；agent 场景 = run_id，API 直调 = request_id
	Caller       string    `json:"caller"`                // "api" | "agent" | "cron"
	SpanKind     string    `json:"span_kind"`             // "llm" | "tool"
	Name         string    `json:"name"`                  // llm: 模型名；tool: 工具名
	InputJSON    string    `json:"input_json,omitempty"`  // llm: 请求消息数组快照；tool: arguments JSON
	OutputJSON   string    `json:"output_json,omitempty"` // llm: {content, tool_calls[]}；tool: 结果文本
	ToolsJSON    string    `json:"tools_json,omitempty"`  // 仅 llm span：声明的工具 [{name, description}]
	CreatedAt    time.Time `json:"created_at"`
}

// LLMTraceQuery 是 trace 查询条件。
type LLMTraceQuery struct {
	ProjectID string
	Status    string
	Model     string // 模糊匹配
	RequestID string // 精确匹配
	From      time.Time
	To        time.Time
	Limit     int
	Offset    int
}

// LLMTraceGroupQuery 是分组列表（二期 §9.3）的查询条件。
type LLMTraceGroupQuery struct {
	ProjectID string
	Caller    string // 空 = 全部
	Status    string // 空 = 全部；error 表示该 trace 任一 span 出错
	Model     string // 模糊匹配 llm span 的 name/model
	From      time.Time
	To        time.Time
	Limit     int
	Offset    int
}

// LLMTraceGroup 是一次顶层调用的聚合视图（列表行）。
type LLMTraceGroup struct {
	TraceID      string    `json:"trace_id"`
	Caller       string    `json:"caller"`
	Model        string    `json:"model"`
	Provider     string    `json:"provider"`
	Status       string    `json:"status"` // 任一 span error 即 error
	Stream       bool      `json:"stream"` // 任一 llm span 流式即 true
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	SpanCount    int64     `json:"span_count"`
	ToolCount    int64     `json:"tool_count"`
	LLMCount     int64     `json:"llm_count"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	DurationMs   int64     `json:"duration_ms"` // finished-started 墙钟差（毫秒）
	ErrorCode    string    `json:"error_code,omitempty"`
	RequestID    string    `json:"request_id"`
}

// 内容截断常量（二期 §9.1）：单 span 硬上限，防止历史消息膨胀拖垮写入。
const (
	traceErrorMessageMax  = 512
	traceSpanInputMax     = 64 * 1024 // llm span 输入消息快照总量
	traceSpanOutputMax    = 32 * 1024 // llm span 输出总量
	traceToolOutputMax    = 8 * 1024  // tool span 结果（与 runtime 工具卡片 4000 字符口径对齐取小）
	traceToolCardMaxChars = 4000      // 对齐 gocloudagent maxToolCardChars
)

// RecordLLMTrace 缓冲一条 trace（请求路径 O(1)；满批发信号由后台刷写）。
func (s *Store) RecordLLMTrace(t LLMTrace) {
	if s == nil {
		return
	}
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	if t.Source == "" {
		t.Source = "llm"
	}
	if t.Status == "" {
		t.Status = "ok"
	}
	if t.SpanKind == "" {
		t.SpanKind = "llm"
	}
	if t.Name == "" {
		t.Name = t.Model
	}
	if len(t.ErrorMessage) > traceErrorMessageMax {
		t.ErrorMessage = t.ErrorMessage[:traceErrorMessageMax]
	}
	t.InputJSON = truncateTraceJSON(t.InputJSON, traceSpanInputMax)
	t.OutputJSON = truncateTraceJSON(t.OutputJSON, traceSpanOutputMax)
	t.ToolsJSON = truncateTraceJSON(t.ToolsJSON, traceSpanOutputMax)
	s.mu.Lock()
	if len(s.traceBuf) >= flushQueueCap {
		s.droppedTrace++
		s.mu.Unlock()
		return
	}
	s.traceBuf = append(s.traceBuf, t)
	n := len(s.traceBuf)
	f := s.async
	closed := s.closed
	s.mu.Unlock()
	if n >= 64 {
		switch {
		case f != nil:
			f.notify()
		case !closed:
			// flusher 未启动（未 StartPeriodicFlush）：维持同步兜底语义。
			_ = s.FlushTraces(context.Background())
		}
	}
}

// truncateTraceJSON 按 byte 上限截断内容 JSON；超限时丢弃尾部字节并标记。
// 返回形如 {"truncated":true} 的标记 JSON，前端据此展示截断提示。
func truncateTraceJSON(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return `{"truncated":true}`
}

// FlushTraces 将缓冲 trace 写入 sys_llm_traces（批量单事务）。
func (s *Store) FlushTraces(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.flushTracesBatch(ctx, s.drainTraces())
}

// drainTraces 批量取空 trace 缓冲（由 Store.mu 保护调用约定）。
func (s *Store) drainTraces() []LLMTrace {
	s.mu.Lock()
	batch := s.traceBuf
	s.traceBuf = nil
	s.mu.Unlock()
	return batch
}

// insertLLMTraces 向 ex 批量插入 trace 行；失败由调用方负责退回缓冲。
func insertLLMTraces(ctx context.Context, ex execer, batch []LLMTrace) error {
	var sb strings.Builder
	sb.WriteString(`INSERT INTO sys_llm_traces(id, project_id, request_id, source, provider, model, status, stream, duration_ms, input_tokens, output_tokens, error_code, error_message, trace_id, caller, span_kind, name, input_json, output_json, tools_json, created_at) VALUES `)
	args := make([]any, 0, len(batch)*21)
	for i, t := range batch {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		args = append(args, t.ID, t.ProjectID, t.RequestID, t.Source, t.Provider, t.Model, t.Status,
			sqlval.BoolInt(t.Stream), t.DurationMs, t.InputTokens, t.OutputTokens, t.ErrorCode, t.ErrorMessage,
			t.TraceID, t.Caller, t.SpanKind, t.Name, t.InputJSON, t.OutputJSON, t.ToolsJSON, traceTimeArg(t.CreatedAt))
	}
	_, err := ex.ExecContext(ctx, sb.String(), args...)
	return err
}

// flushTracesBatch 单事务批量写 trace；空批 no-op。
func (s *Store) flushTracesBatch(ctx context.Context, batch []LLMTrace) error {
	if s == nil || s.db == nil || len(batch) == 0 {
		return nil
	}
	if err := insertLLMTraces(ctx, s.db, batch); err != nil {
		s.mu.Lock()
		s.traceBuf = append(batch, s.traceBuf...)
		s.mu.Unlock()
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// traceTimeArg 把时间统一为 RFC3339Nano 文本再绑定。
// uglyer/go-sqlite3 对 time.Time 的落盘会截断到微秒、且查询参数与写入格式不一致
// （写入 "2026-11-12T05:32:28.501712Z"、参数 "2026-11-12 05:32:28.501712500+00:00"），
// 字符串比较在纳秒差异下错位。写入（insertLLMTraces）与查询参数都走本函数，
// 保证两端文本格式严格一致；DuckLake 侧参数仍为 time.Time 语义不受影响。
func traceTimeArg(t time.Time) any {
	if t.IsZero() {
		return t
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// traceSelectColumns 是 SELECT 的完整列清单（与 insertLLMTraces 列序一致）。
const traceSelectColumns = `id, project_id, request_id, source, provider, model, status, stream,
	                  duration_ms, input_tokens, output_tokens, error_code, error_message,
	                  trace_id, caller, span_kind, name, input_json, output_json, tools_json, created_at`

// scanLLMTrace 把当前行扫进 LLMTrace。
func scanLLMTrace(rows *sql.Rows) (LLMTrace, error) {
	var t LLMTrace
	var stream int64
	if err := rows.Scan(&t.ID, &t.ProjectID, &t.RequestID, &t.Source, &t.Provider, &t.Model, &t.Status,
		&stream, &t.DurationMs, &t.InputTokens, &t.OutputTokens, &t.ErrorCode, &t.ErrorMessage,
		&t.TraceID, &t.Caller, &t.SpanKind, &t.Name, &t.InputJSON, &t.OutputJSON, &t.ToolsJSON, &t.CreatedAt); err != nil {
		return t, err
	}
	t.Stream = stream != 0
	// 一期遗留行（升级兜底路径）无 caller/name/span_kind：读取侧补默认语义。
	if t.SpanKind == "" {
		t.SpanKind = "llm"
	}
	if t.Name == "" {
		t.Name = t.Model
	}
	if t.Caller == "" {
		t.Caller = "api"
	}
	return t, nil
}

// QueryLLMTraces 按条件查询模型调用 trace。
// From 为空时默认取最近 24h（对齐 QueryLogs 的剪枝策略，避免全表扫描）。
func (s *Store) QueryLLMTraces(ctx context.Context, q LLMTraceQuery) ([]LLMTrace, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	q.Limit = limit.Normalize(q.Limit, 50, 200)
	if q.Offset < 0 {
		q.Offset = 0
	}
	if q.From.IsZero() {
		q.From = time.Now().UTC().Add(-24 * time.Hour)
	}
	sqlStr := `SELECT ` + traceSelectColumns + `
	           FROM sys_llm_traces WHERE project_id = ?`
	args := []any{q.ProjectID}
	if q.Status != "" {
		sqlStr += " AND status = ?"
		args = append(args, q.Status)
	}
	if q.Model != "" {
		sqlStr += " AND model LIKE ?"
		args = append(args, "%"+q.Model+"%")
	}
	if q.RequestID != "" {
		sqlStr += " AND request_id = ?"
		args = append(args, q.RequestID)
	}
	if !q.From.IsZero() {
		sqlStr += " AND created_at >= ?"
		args = append(args, traceTimeArg(q.From))
	}
	if !q.To.IsZero() {
		sqlStr += " AND created_at <= ?"
		args = append(args, traceTimeArg(q.To))
	}
	sqlStr += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, int64(q.Limit), int64(q.Offset))
	rows, err := s.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LLMTrace, 0)
	for rows.Next() {
		t, err := scanLLMTrace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CountLLMTraces 按 QueryLLMTraces 同条件计数（分页 total）。
func (s *Store) CountLLMTraces(ctx context.Context, q LLMTraceQuery) (int64, error) {
	if s == nil || s.db == nil {
		return 0, ErrUnavailable
	}
	if q.From.IsZero() {
		q.From = time.Now().UTC().Add(-24 * time.Hour)
	}
	sqlStr := `SELECT COUNT(*) FROM sys_llm_traces WHERE project_id = ?`
	args := []any{q.ProjectID}
	if q.Status != "" {
		sqlStr += " AND status = ?"
		args = append(args, q.Status)
	}
	if q.Model != "" {
		sqlStr += " AND model LIKE ?"
		args = append(args, "%"+q.Model+"%")
	}
	if q.RequestID != "" {
		sqlStr += " AND request_id = ?"
		args = append(args, q.RequestID)
	}
	if !q.From.IsZero() {
		sqlStr += " AND created_at >= ?"
		args = append(args, traceTimeArg(q.From))
	}
	if !q.To.IsZero() {
		sqlStr += " AND created_at <= ?"
		args = append(args, traceTimeArg(q.To))
	}
	var n int64
	err := s.db.QueryRowContext(ctx, sqlStr, args...).Scan(&n)
	return n, err
}

// traceGroupWhere 组装分组查询的公共 WHERE（caller / status / model / 时间窗）。
// model 关键词同时命中 model 与 name 列（tool span 的 name 是工具名，llm span 二者同值）。
func traceGroupWhere(q LLMTraceGroupQuery) (string, []any) {
	sqlStr := " WHERE project_id = ?"
	args := []any{q.ProjectID}
	if q.Caller != "" {
		sqlStr += " AND caller = ?"
		args = append(args, q.Caller)
	}
	if q.Status != "" {
		sqlStr += " AND status = ?"
		args = append(args, q.Status)
	}
	if q.Model != "" {
		sqlStr += " AND (model LIKE ? OR name LIKE ?)"
		args = append(args, "%"+q.Model+"%", "%"+q.Model+"%")
	}
	if !q.From.IsZero() {
		sqlStr += " AND created_at >= ?"
		args = append(args, traceTimeArg(q.From))
	}
	if !q.To.IsZero() {
		sqlStr += " AND created_at <= ?"
		args = append(args, traceTimeArg(q.To))
	}
	return sqlStr, args
}

// QueryLLMTraceGroups 按 trace_id 分组聚合顶层调用（二期 §9.3 列表）。
// From 为空时默认 24h 窗口；status=error 表示该 trace 任一 span 出错。
func (s *Store) QueryLLMTraceGroups(ctx context.Context, q LLMTraceGroupQuery) ([]LLMTraceGroup, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	q.Limit = limit.Normalize(q.Limit, 50, 200)
	if q.Offset < 0 {
		q.Offset = 0
	}
	if q.From.IsZero() {
		q.From = time.Now().UTC().Add(-24 * time.Hour)
	}
	where, args := traceGroupWhere(q)
	sqlStr := `SELECT trace_id,
	                  MIN(created_at), MAX(created_at),
	                  MAX(caller), MAX(model), MAX(provider), MAX(request_id),
	                  MAX(CASE WHEN status='error' THEN 1 ELSE 0 END),
	                  MAX(stream),
	                  COUNT(*),
	                  SUM(CASE WHEN span_kind='tool' THEN 1 ELSE 0 END),
	                  SUM(CASE WHEN span_kind!='tool' THEN 1 ELSE 0 END),
	                  SUM(input_tokens), SUM(output_tokens),
	                  MAX(error_code)
	           FROM sys_llm_traces` + where + ` GROUP BY trace_id
	           ORDER BY MAX(created_at) DESC LIMIT ? OFFSET ?`
	args = append(args, int64(q.Limit), int64(q.Offset))
	rows, err := s.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LLMTraceGroup, 0)
	for rows.Next() {
		var g LLMTraceGroup
		var hasErr, stream int64
		var started, finished string
		if err := rows.Scan(&g.TraceID, &started, &finished,
			&g.Caller, &g.Model, &g.Provider, &g.RequestID,
			&hasErr, &stream, &g.SpanCount, &g.ToolCount, &g.LLMCount,
			&g.InputTokens, &g.OutputTokens, &g.ErrorCode); err != nil {
			return nil, err
		}
		if hasErr == 1 {
			g.Status = "error"
		} else {
			g.Status = "ok"
		}
		g.Stream = stream != 0
		// MIN/MAX(created_at) 在 sqlite 驱动下回读为字符串；与写入侧同为
		// RFC3339Nano 文本（traceTimeArg），这里统一解析，失败则置零。
		g.StartedAt = parseTraceTime(started)
		g.FinishedAt = parseTraceTime(finished)
		g.DurationMs = g.FinishedAt.Sub(g.StartedAt).Milliseconds()
		if g.DurationMs < 0 {
			g.DurationMs = 0
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// parseTraceTime 解析 traceTimeArg 写入的时间文本（RFC3339Nano）。
func parseTraceTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	return time.Time{}
}

// CountLLMTraceGroups 按 QueryLLMTraceGroups 同条件计数（分组 total）。
func (s *Store) CountLLMTraceGroups(ctx context.Context, q LLMTraceGroupQuery) (int64, error) {
	if s == nil || s.db == nil {
		return 0, ErrUnavailable
	}
	if q.From.IsZero() {
		q.From = time.Now().UTC().Add(-24 * time.Hour)
	}
	where, args := traceGroupWhere(q)
	sqlStr := `SELECT COUNT(DISTINCT trace_id) FROM sys_llm_traces` + where
	var n int64
	err := s.db.QueryRowContext(ctx, sqlStr, args...).Scan(&n)
	return n, err
}

// QueryTraceSpans 取一个 trace 的全部 span（按时间升序；详情视图）。
// traceID 为空或不属于 project 时返回空切片。
func (s *Store) QueryTraceSpans(ctx context.Context, projectID, traceID string) ([]LLMTrace, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if traceID == "" {
		return []LLMTrace{}, nil
	}
	sqlStr := `SELECT ` + traceSelectColumns + `
	           FROM sys_llm_traces WHERE project_id = ? AND trace_id = ?
	           ORDER BY created_at ASC, id ASC`
	rows, err := s.db.QueryContext(ctx, sqlStr, projectID, traceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LLMTrace, 0)
	for rows.Next() {
		t, err := scanLLMTrace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// AgentToolCard 是 gocloudagent.RunResult.ToolCallsJSON 的单张工具卡片形状。
type AgentToolCard struct {
	CallID     string `json:"call_id"`
	Name       string `json:"name"`
	Arguments  string `json:"arguments"`
	Content    string `json:"content"`
	DurationMs int64  `json:"duration_ms"`
	Truncated  bool   `json:"truncated"`
	IsError    bool   `json:"is_error"`
}

// RecordAgentToolSpans 解析工具卡片 JSON，把每张卡片作为一条 tool span 入队
// （二期 §10.3：run 收尾时批量补写；created_at 统一取当前时刻，时间线顺序由
// span 序列 + duration 呈现）。解析失败静默返回错误由调用方决定是否记日志。
func (s *Store) RecordAgentToolSpans(projectID, traceID string, toolCardsJSON string) error {
	if s == nil {
		return nil
	}
	if strings.TrimSpace(toolCardsJSON) == "" || toolCardsJSON == "[]" || traceID == "" {
		return nil
	}
	var cards []AgentToolCard
	if err := json.Unmarshal([]byte(toolCardsJSON), &cards); err != nil {
		return fmt.Errorf("systemdb: parse tool cards: %w", err)
	}
	now := time.Now().UTC()
	for _, c := range cards {
		if strings.TrimSpace(c.Name) == "" && strings.TrimSpace(c.CallID) == "" {
			continue
		}
		status := "ok"
		errCode := ""
		errMsg := ""
		if c.IsError {
			status = "error"
			errCode = "tool_error"
			errMsg = c.Content
		}
		s.RecordLLMTrace(LLMTrace{
			ProjectID: projectID, TraceID: traceID, Caller: "agent",
			SpanKind: "tool", Name: c.Name, Status: status,
			DurationMs: c.DurationMs,
			InputJSON:  c.Arguments,
			OutputJSON: c.Content,
			ErrorCode:  errCode, ErrorMessage: errMsg,
			CreatedAt: now,
		})
	}
	return nil
}
