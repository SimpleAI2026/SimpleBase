# godatabase

`godatabase` 是可独立使用的数据库运行时库，模块路径为 `github.com/linkxzhou/SimpleBase/godatabase`。它不 import SimpleBase 根模块，也不依赖宿主的 catalog、鉴权、配置或指标实现。SimpleBase 通过 `internal/dbbridge` 注入这些宿主能力，其他平台也可以直接实现同样的接口。

用户库是项目级单文件数据库：每个项目一个 `.duckdb` 或 `.sqlite` 文件，SQL、集合文档和 KV 共用。系统库使用 DuckLake。相对数据库路径为 `{projectID}/database.duckdb` 或 `{projectID}/database.sqlite`，引擎由扩展名确定。

> 模块已可脱离 SimpleBase 使用，但公共 API 仍不稳定。仓库内通过根 `go.mod` 的 `replace => ./godatabase` 引用。

## 包结构

```text
godatabase.go 等         根包：Spec / Factory / Registry 访问模式 / Query / Dialect / 序列化 / 错误
blob/                    对象存储接口、S3 key 布局与内存实现
filedb/                  DuckDB / SQLite 单文件数据库、快照恢复与 manifest 同步
ducklake/                DuckLake catalog、快照链、写门与维护能力
registry/                进程内单写注册表、访问租约与空闲淘汰
cache/                   缓存目录用量、容量限制与 LRU 淘汰
kv/                      键值数据结构、TTL、扫描与事务
lease/                   写租约与写门
sqlguard/                SQL 拒绝清单、只读检测与多语句拦截
sqlx/                    SQL 字面量和标识符转义
sqlval/                  database/sql 参数值归一化
atomicfile/              同目录临时文件 + rename 原子写
hashx/                   SHA-256 十六进制摘要
limit/                   分页参数归一化
singleflight/            泛型按 key 单飞
internal/testutil/       库内测试共用的 RecordingLogger / RecordingObserver
bench/                   系统库读路径基准
```

根包不 import 任何子包，调用方按需 import 子包。`atomicfile`、`hashx`、`limit`、`singleflight` 和 `sqlval` 是公开包，宿主与其他平台应直接复用这些实现，不再维护同名副本。

## 独立使用

宿主提供数据库规格、Factory、Registry 和可选适配器：

```go
spec := godatabase.Spec{
    ID: "db1", ProjectID: "abcd1234", Kind: godatabase.KindUser,
    Status: godatabase.StatusReady, DatabasePath: "abcd1234/database.duckdb",
}
factory := &filedb.Factory{
    DataRoot: dataDir,
    CacheDir: cacheDir,
    Syncer: filedb.NewLocalSyncer(),
}
reg := registry.New(factory, nil, registry.Options{Writable: true}, nil, nil)
lease, err := reg.Acquire(ctx, spec, godatabase.ReadWrite)
if err != nil {
    return err
}
defer lease.Release()
result, err := lease.Handle.Query(ctx, godatabase.Statement{SQL: "SELECT 1"}, 100)
```

| 接口 | 用途 | nil 语义 |
| --- | --- | --- |
| `PathResolver` | 为缺失 `DatabasePath` 的用户库补全路径，可执行宿主归属校验 | 跳过补全 |
| `Logger` | 与 zap 方法签名一致的最小日志接口 | 静默 |
| `Observer` | 句柄、缓存和 catalog 同步指标 | 不采集 |
| `blob.Store` | 对象存储边界；库不绑定 AWS SDK | 仅本地模式 |

不要把 nil 具体指针装入上述接口；这会产生非 nil 接口并绕过 nil 判断。

## 安全与磁盘契约

- 所有写接口必须从 `registry` 获取 `ReadWrite` handle，不得绕过注册表直接构造 DSN。
- 数据库物理路径不得进入 API 响应、日志或审计。
- 系统库只通过 `ducklake` 打开，用户库只通过 `filedb` 打开；`filedb` 必须拒绝系统库。
- 项目 ID 固定为 8 位 `[A-Za-z0-9-]`，对象键使用内部 ID，不拼接用户可控字符串。
- S3 key 布局以及 manifest、lease、instance-format 的 JSON 字段是磁盘契约，不得随代码清理修改。
- 不提供旧数据兼容、回填或迁移；无法识别的数据必须 fail closed 并要求 reset。
- 写租约丢失后停止上传，manifest 冲突不得覆盖。
- 单写保证仅限单进程；部署层必须固定副本数为 1。

## 验证

```bash
cd godatabase
GOWORK=off go build ./...
GOWORK=off go vet ./...
GOWORK=off go test ./... -timeout 900s
```

`ducklake` 和 `lease` 的真实引擎及租约测试较慢。修改对象键前还必须运行根模块的 `go test ./internal/objectstore`，其金标测试逐字节锁定 key 布局。
