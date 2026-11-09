<!-- status: completed -->
<!-- verified: 2026-07-19 go build/test ./internal/... ./cmd/... 通过 -->

# Plan 1：工程基线、依赖与服务装配

## 目标
将当前仅注册 S3 上传接口的 `cmd/main.go`、`server/server.go` 改造成可装配的单实例 `simplebased`。本计划不实现业务 API，只建立可启动、可关闭、可注入依赖的骨架。

## 当前代码与处理
- 修改 `cmd/main.go`：删除直接 `echo.New()`、`server.NewServer()`、`RegisterStorageRoutes()` 的启动逻辑。
- 冻结 `server/server.go`、`server/storage.go`，不再向旧接口增加功能；后续由 `internal/api` 替代。
- 新建 `cmd/simplebased/main.go` 作为正式入口；可暂时保留 `cmd/main.go` 为调用该入口的兼容启动器，最终删除。
- 修改 `go.mod`：加入官方 `turso.tech/database/tursogo`；不要在此阶段移除旧 Raft/SQLite 依赖，待 Plan 10 删除。

## 目录骨架
```text
cmd/simplebased/main.go
internal/app/app.go
internal/config/config.go
internal/config/validate.go
internal/api/router.go
internal/api/health.go
internal/observability/logger.go
internal/observability/metrics.go
```

## 配置类型（`internal/config/config.go`）
```go
type Config struct {
    HTTP       HTTPConfig       `mapstructure:"http"`
    Instance   InstanceConfig   `mapstructure:"instance"`
    Database   DatabaseConfig   `mapstructure:"database"`
    S3         S3Config         `mapstructure:"s3"`
    Catalog    CatalogConfig    `mapstructure:"catalog"`
    Auth       AuthConfig       `mapstructure:"auth"`
    LLM        LLMConfig        `mapstructure:"llm"`
    Limits     LimitsConfig     `mapstructure:"limits"`
    Observability ObservabilityConfig `mapstructure:"observability"`
}
type HTTPConfig struct { Address string; ReadTimeout, WriteTimeout, IdleTimeout time.Duration }
type InstanceConfig struct { ID string; Writable bool }
type DatabaseConfig struct { CacheDir string; IdleTimeout time.Duration; MaxOpen int; MaxIdle int }
type S3Config struct { Endpoint, Region, Bucket, Prefix, AccessKey, SecretKey, KMSKeyID string; ForcePathStyle bool }
type CatalogConfig struct { DatabaseID string }
type AuthConfig struct { APIKeyHashSecret string }
type LLMConfig struct { Providers map[string]ProviderConfig }
type LimitsConfig struct { MaxRequestBytes int64; MaxQueryRows int; QueryTimeout time.Duration; MaxConcurrentQueries int }
```

### 必须实现的函数
```go
func Load() (Config, error)                    // 环境变量/配置文件解析，不打印密钥
func (c Config) Validate() error               // 聚合字段错误；Writable 时要求 S3 配置完整
func (c Config) Redacted() map[string]any      // 仅用于启动日志
func New(ctx context.Context, cfg config.Config) (*App, error)
func (a *App) Start() error
func (a *App) Shutdown(ctx context.Context) error
func NewRouter(deps Dependencies) *echo.Echo
```

`App` 的依赖顺序必须是：`S3 client → catalog DB → database factory/registry → auth → usage → LLM gateway → API router`。任意一步失败时反向关闭已创建资源。

## `App` 骨架
```go
type App struct {
    httpServer *http.Server
    catalog    *catalog.Service
    registry   *registry.Registry
    llm        *llmgateway.Service
    closer     []io.Closer
}
type Dependencies struct { Config config.Config; Catalog *catalog.Service; Registry *registry.Registry; LLM *llmgateway.Service; Auth *auth.Service; Usage *usage.Service }
```

## 单实例启动约束
1. `cfg.Instance.Writable` 为 false 时禁止注册写 API。
2. 写实例启动记录 `instance_id`、版本、S3 bucket/prefix（不记录密钥）。
3. 不实现“分布式锁”；仅记录并检查本进程内状态。部署清单必须固定可写副本数为 1。
4. `/health/live` 只检查进程；`/health/ready` 必须检查 catalog 与 S3，Writable 模式下 S3 失败不可 ready。

## 路由占位
`NewRouter` 先注册：`GET /health/live`、`GET /health/ready`、`GET /metrics`。Plan 5/6/8 再挂载业务路由。统一中间件顺序：request ID → recover → access log → body limit → auth（业务组）→ project context → error mapper。

## 测试与验收
- `Load` 对空地址、空缓存目录、Writable 缺 S3 bucket、非法 timeout 返回确定错误。
- `New` 任一依赖构建失败时不泄露文件句柄/连接。
- SIGTERM 调用 `Shutdown`，停止接收新请求并关闭 registry、catalog、LLM HTTP client。
- `go test ./cmd/simplebased ./internal/app ./internal/config ./internal/api` 通过。

## 完成标志
单实例可启动，`/health/live` 与 `/health/ready` 可用；尚不暴露旧 storage API、新数据库 API 或 LLM API。
