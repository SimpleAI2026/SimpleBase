<!-- status: completed -->
<!-- verified: 2026-07-19 objectstore/turso 测试通过 -->

# Plan 2：S3 边界、对象命名与 Turso 持久层适配

## 目标
将 `internal/s3` 从通用文件上传/AppendObject 工具收敛为平台配置与元数据辅助层；数据库数据对象完全由 Turso 管理。SimpleBase 绝不解析、拼接或修改 Turso 在 `data/` 下的对象格式。

## 当前代码与处理
- 保留并审查 `internal/s3/s3.go` 的 client 初始化与 `Upload`/`Download` 基础能力。
- 不在数据库链路调用 `s3_append.go`、`s3_select.go`；标记为旧 storage API 专用，Plan 10 删除或移出。
- 修改 `internal/s3/s3_config.go`：不要让 `AccessKey/SecretKey` 成为唯一认证方式；支持 AWS default credential chain/工作负载身份。
- `internal/s3/default.go` 的环境变量全局单例不得被新服务直接使用；改由 `config.Config` 显式注入。

## 新目录与接口
```text
internal/objectstore/client.go
internal/objectstore/keys.go
internal/objectstore/descriptor.go
internal/objectstore/health.go
internal/database/turso/dsn.go
internal/database/turso/factory.go
```
```go
type Client interface {
    PutJSON(ctx context.Context, key string, value any, opts PutOptions) error
    GetJSON(ctx context.Context, key string, dst any) error
    Head(ctx context.Context, key string) (ObjectInfo, error)
    DeletePrefix(ctx context.Context, prefix string) error // 仅删除已软删库，由后台任务调用
    Check(ctx context.Context) error
}
type KeyBuilder struct { RootPrefix, Environment string }
func (k KeyBuilder) DatabasePrefix(tenantID, databaseID string) string
func (k KeyBuilder) DescriptorKey(tenantID, databaseID string) string
func (k KeyBuilder) StateKey(tenantID, databaseID string) string
func (k KeyBuilder) BackupPrefix(tenantID, databaseID, backupID string) string
func (k KeyBuilder) CatalogPrefix() string
```

## 对象约定
```text
{root}/{env}/catalog/...
{root}/{env}/tenants/{tenantUUID}/databases/{databaseUUID}/
  descriptor.json              # SimpleBase 写，创建后只允许兼容升级
  state.json                   # SimpleBase 写，运维状态
  data/                        # 只允许 Turso/libSQL 读写
  backups/{backupUUID}/...     # 官方备份能力或导出的独立恢复点
```

`Descriptor` 必须包含：`format_version`、`tenant_id`、`project_id`、`database_id`、`created_at`、`turso_storage`（不含密钥）、`data_prefix`、`status`。禁止用用户输入 name 作为 object key；name 仅存在 catalog。

## Turso 适配边界
官方 `tursogo` 的 S3 连接参数/DSN 格式必须以实现时的官方文档为准，集中在 `internal/database/turso/dsn.go`，不得散落在 handler 或 model：
```go
type StorageConfig struct { Endpoint, Region, Bucket, Prefix, KMSKeyID string; ForcePathStyle bool }
type OpenOptions struct { DatabaseID string; CachePath string; Writable bool; Storage StorageConfig }
func BuildDSN(opts OpenOptions) (string, error) // 只编码官方已验证字段
func Open(ctx context.Context, opts OpenOptions) (*sql.DB, error) {
    dsn, err := BuildDSN(opts)
    if err != nil { return nil, err }
    db, err := sql.Open("turso", dsn)
    // 设置连接池；PingContext；失败 Close
    return db, err
}
```
`BuildDSN` 的单元测试只断言本项目拼装字段、URL 转义、禁止路径穿越和不可记录密钥；对真实 Turso 语义使用集成测试，不臆造参数名。

## 核心流程
1. 创建库时由 catalog 生成 UUID、`KeyBuilder` 生成前缀。
2. `PutJSON(descriptor)` 成功后才允许传给 Turso 打开/创建。
3. `turso.Open` 使用专属缓存目录 `{cacheDir}/{databaseID}` 和专属 `data/` 前缀。
4. 打开后写入 state 的 `last_opened_at`/`last_verified_at`；state 写失败只记录 degraded，不伪造数据库提交结果。
5. 删除只改 catalog 为 deleting；Plan 7 的 worker 在保留期后调用 `DeletePrefix`。

## 安全要求
- 所有 `key` 必须由 `KeyBuilder` 产生；HTTP 层不能接受原始 S3 key。
- bucket 私有、TLS、SSE-KMS/等价加密、版本控制和最小 IAM 是部署前置条件。
- `Client.Check` 只做最小 Head/List/权限验证；不能枚举其他 tenant。
- 错误中去除 endpoint query、AccessKey、SecretKey、预签名 URL。

## 测试与验收
- `KeyBuilder` 对 UUID、prefix 清洗、tenant 隔离有表驱动测试。
- fake object store 测试 descriptor put/get 与错误映射。
- 集成测试：删除本地缓存后按官方 Turso S3 配置重开数据库；S3 拒绝写时 API 不回成功。
- 禁止新增 database 对 `AppendObject`、`SelectObject` 的调用。
