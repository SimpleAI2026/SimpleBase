<!-- status: completed -->
<!-- verified: 2026-07-19 auth/api 测试通过 -->

# Plan 5：认证、项目上下文、数据库管理 API

## 目标
在 Echo 上建立 v1 管理 API，所有资源以 project 为边界。不得延续旧 `server/storage.go` 中将原始 S3 key 暴露给客户端的模式。

## 目录骨架
```text
internal/auth/principal.go
internal/auth/service.go
internal/auth/middleware.go
internal/auth/api_key_repository.go
internal/api/router.go
internal/api/error.go
internal/api/request.go
internal/api/database_handler.go
internal/api/database_handler_test.go
```

## 认证模型
```go
type Permission string
const ( DatabaseRead Permission = "database:read"; DatabaseWrite = "database:write"; DatabaseAdmin = "database:admin"; LLMInvoke = "llm:invoke"; ProjectAdmin = "project:admin" )
type Principal struct { APIKeyID, TenantID string; ProjectIDs map[string]struct{}; Permissions map[Permission]struct{} }
type Service interface {
  Authenticate(ctx context.Context, rawKey string) (Principal, error)
  Authorize(p Principal, projectID string, permission Permission) error
}
func APIKeyMiddleware(s Service) echo.MiddlewareFunc
func Require(permission Permission) echo.MiddlewareFunc
func PrincipalFromContext(ctx context.Context) (Principal, bool)
```
API key 原文只在接收请求时出现；使用带 server secret 的 hash/HMAC 存储不可逆摘要。拒绝 `Authorization` 缺失、格式错误、已撤销 key、project 不匹配。响应和日志不得回显 key。

## 路由（`internal/api/router.go`）
```go
v1 := e.Group("/v1", APIKeyMiddleware(auth))
p := v1.Group("/projects/:projectID")
p.POST("/databases", h.CreateDatabase, Require(DatabaseAdmin))
p.GET("/databases", h.ListDatabases, Require(DatabaseRead))
p.GET("/databases/:databaseID", h.GetDatabase, Require(DatabaseRead))
p.POST("/databases/:databaseID/open", h.OpenDatabase, Require(DatabaseAdmin))
p.POST("/databases/:databaseID/close", h.CloseDatabase, Require(DatabaseAdmin))
p.POST("/databases/:databaseID/backups", h.CreateBackup, Require(DatabaseAdmin))
p.POST("/databases/:databaseID/restore", h.RestoreDatabase, Require(DatabaseAdmin))
p.DELETE("/databases/:databaseID", h.DeleteDatabase, Require(DatabaseAdmin))
```
项目参数必须先经 middleware 转为 context 内的 `ProjectContext{ID,TenantID}`；handler 不可自行相信 body 中 tenant/project ID。

## DTO 与 handler 骨架
```go
type CreateDatabaseRequest struct { Name string `json:"name" validate:"required,min=1,max=63"` }
type DatabaseResponse struct { ID, Name, Status string; CreatedAt time.Time; LastPersistedAt *time.Time }
func (h *DatabaseHandler) CreateDatabase(c echo.Context) error {
  project, err := ProjectFromContext(c.Request().Context())
  // bind + Validate；h.catalog.CreateDatabase；不得 Open/SQL/S3 直调
}
func (h *DatabaseHandler) OpenDatabase(c echo.Context) error {
  // catalog.GetDatabase(scope) → registry.Acquire(ReadWrite) → release → response
}
func (h *DatabaseHandler) DeleteDatabase(c echo.Context) error {
  // catalog.BeginDeleteDatabase；投递后台清理任务；返回 202 operation
}
```
每个 handler：解析 path → 限制 body → `Bind` 后显式 validate → 调 service → map typed error → JSON。所有创建/删除/恢复写审计事件，包含 request ID、principal ID、project/database ID，不含 body 密钥/SQL。

## 统一错误协议
```json
{"error":{"code":"database_not_found","message":"database not found","request_id":"..."}}
```
实现 `WriteError(c, err)`：validation=400、unauthenticated=401、forbidden=403、not found=404、conflict/state=409、row/query limit=422/429、dependency unavailable=503、未知=500。内部错误只记录，不能返回 S3/DSN/SQL driver 细节。

## 限制与中间件
- `http.MaxBytesReader` 限制请求体；JSON decoder 禁止未知字段。
- request ID 采纳合法外部 ID 或生成 UUID；写入响应 header 和日志。
- recover middleware 记录 stack，不向用户暴露。
- 写 API 在 `cfg.Instance.Writable=false` 时统一返回 503 `writer_unavailable`。

## 测试与验收
- 无 key/无权限/跨 project/database ID 猜测均被拒绝。
- 创建后 response 不出现 S3 key、DSN、凭据。
- 删除返回 202，重复删除和状态冲突返回确定 code。
- 错误 JSON 格式全路由一致；旧 `/v1/storage/*` 不混入新 `v1/projects` 组。
