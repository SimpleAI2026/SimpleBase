<!-- status: completed -->
<!-- verified: 2026-07-19 sqlguard/serialize/sql_handler 测试通过 -->

# Plan 6：SQL API、执行边界与响应协议

## 目标
基于 Plan 4 的 `Registry`/`Handle` 暴露受 project 隔离的 SQL API。所有 SQL 必须经过认证、数据库状态检查、超时与资源上限控制；不能把 `database/sql` 或 Turso 错误直接透传给客户端。

## 新增文件
```text
internal/api/sql_handler.go
internal/api/sql_types.go
internal/database/sqlguard/classifier.go
internal/database/sqlguard/validator.go
internal/database/serialize.go
internal/database/query_test.go
internal/api/sql_handler_test.go
```

## 路由
```go
p.POST("/databases/:databaseID/query", h.Query, Require(auth.DatabaseRead))
p.POST("/databases/:databaseID/execute", h.Execute, Require(auth.DatabaseWrite))
p.POST("/databases/:databaseID/batch", h.Batch, Require(auth.DatabaseWrite))
```
写路由先检查 `cfg.Instance.Writable`，再 Acquire `ReadWrite` lease；查询默认 Acquire `ReadOnly`，但首期同一实例可由同一 Turso handle 提供，仍必须保留 mode 参数以便以后接入只读实例。

## HTTP DTO
```go
type SQLStatementRequest struct {
    SQL  string `json:"sql" validate:"required,max=65536"`
    Args []any  `json:"args"`
}
type QueryRequest struct { SQLStatementRequest; MaxRows int `json:"max_rows"` }
type ExecuteRequest struct { SQLStatementRequest }
type BatchRequest struct {
    Statements    []SQLStatementRequest `json:"statements" validate:"required,min=1,max=100"`
    Transactional bool                  `json:"transactional"`
}
type QueryResponse struct {
    Columns []string `json:"columns"`
    Rows    [][]any  `json:"rows"`
    RowCount int     `json:"row_count"`
    DurationMS int64 `json:"duration_ms"`
    RequestID string `json:"request_id"`
}
type ExecuteResponse struct { RowsAffected int64 `json:"rows_affected"`; LastInsertID *int64 `json:"last_insert_id,omitempty"`; DurationMS int64 `json:"duration_ms"`; RequestID string `json:"request_id"` }
```
请求不接受 raw DSN、database path、S3 key、事务 ID 或任意 driver option。

## Handler 精确流程
```go
func (h *SQLHandler) Query(c echo.Context) error {
    ctx, cancel := context.WithTimeout(c.Request().Context(), h.limits.QueryTimeout)
    defer cancel()
    project := MustProject(ctx)
    req := decodeQueryRequest(c)                 // MaxBytesReader + DisallowUnknownFields + validate
    db := h.catalog.GetDatabase(ctx, project.ID, c.Param("databaseID"))
    sqlguard.Validate(req.SQL, sqlguard.ReadOnly)
    lease := h.registry.Acquire(ctx, db, database.ReadOnly)
    defer lease.Release()
    result := lease.Handle.Query(ctx, statement(req), effectiveMaxRows(req.MaxRows, h.limits.MaxQueryRows))
    audit query metadata; return JSON(200, result)
}
```
`Execute` 与 `Batch` 同理，但 `Validate` 使用 `WriteAllowed`。任何 context deadline/cancel 都返回稳定的业务错误码；不重试未知写结果，避免重复提交。

## SQL Guard
该模块不是 SQL 防火墙，只执行低成本、可测试的策略：

```go
type Intent uint8
const ( ReadOnly Intent = iota; WriteAllowed )
func Validate(sql string, intent Intent) error
func FirstKeyword(sql string) string
```
实现要求：去除前导空白与 SQL 注释；拒绝空 SQL、多语句（分号后还有非注释内容）、NUL 字符；按首关键字处理：
- `query` 仅允许 `SELECT`、`WITH`、`EXPLAIN`、受控 `PRAGMA`；
- `execute/batch` 可允许 DML/DDL；
- 全部拒绝 `ATTACH`、`DETACH`、`LOAD_EXTENSION`、`VACUUM INTO`、`PRAGMA writable_schema`、`PRAGMA load_extension`。

不要用关键字判断替代授权：DDL 仍需 `database:admin` 或在首期明确仅允许 `database:write` 的范围；此决策写入测试。若 parser 无法可靠判断，则保守拒绝并返回 `sql_not_allowed`。

## 值序列化（`serialize.go`）
SQLite 返回值必须稳定转 JSON：
- `nil → null`；`int64/float64/string/bool` 直接输出；
- `[]byte → {"type":"blob","base64":"..."}`，不能当 UTF-8 猜测；
- `time.Time → RFC3339Nano string`；其余 driver value 返回内部错误并计数。
- `Rows` 必须 `defer rows.Close()` 并检查 `rows.Err()`。

## 并发、上限与审计
- 全局及 project 级 semaphore；获取不到返回 429 `query_concurrency_exceeded`。
- `MaxRows` 取 `[1, configured limit]`，超过配置返回 400，而非悄悄修改。
- 字符串 SQL 最大字节数、args 总大小、batch 条数都由 `LimitsConfig` 控制。
- 审计只记录 statement keyword、SQL SHA-256、耗时、返回行数、结果/错误码；默认不记录原 SQL 与参数。

## 测试与验收
- Query 不能执行 INSERT/UPDATE；Execute 不能多语句或 ATTACH。
- 参数 bind 测试包含字符串、null、数字、blob；禁止拼接 SQL。
- batch transactional 中第 N 条失败必须回滚；非事务 batch 结果逐条返回并明确失败位置。
- rows 上限、context timeout、cancel、driver error、degraded/deleting database 有稳定 HTTP code。
- 并发请求不会绕过 Registry 的唯一 writer。
