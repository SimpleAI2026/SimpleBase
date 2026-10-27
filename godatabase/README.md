# godatabase

SimpleBase 的数据库运行时库，独立 Go 模块 `github.com/linkxzhou/SimpleBase/godatabase`（根 `go.mod` 用 `replace => ./godatabase` 引用）。**不得 import 根模块任何包**（含 `internal/...`）。

用户库是**项目级单文件数据库**：每个项目一个 `.duckdb` 或 `.sqlite` 主文件（SQL、集合文档、KV 共用），相对路径形如 `{projectID}/database.duckdb`，引擎由扩展名决定且创建后不可更改。系统库使用 DuckLake。

> 状态：API 不稳定，随 SimpleBase 发布（模块版本固定为 `v0.0.0` + `replace`）。

```text
godatabase.go 等         根包：AccessMode / Factory / Spec / Status / Logger / Observer / PathResolver /
                         引擎与路径纯函数 / Dialect / Query / 行序列化 / 错误
blob/                    对象存储接口 Store + ObjectInfo + 错误 + KeyBuilder（S3 key 布局）+ 内存实现
filedb/                  用户库 Factory：打开 DuckDB / SQLite 文件、路径安全、S3 快照 + manifest + 项目租约
ducklake/                系统库专用 DuckLake Factory：catalog 同步、manifest 链、实例格式标记
registry/                进程内每项目文件唯一 writer：Acquire / Lease / 空闲淘汰
cache/                   缓存目录 LRU、路径防穿越
kv/                      键值数据结构、TTL、扫描与事务
lease/                   项目文件写租约与写门
sqlguard/                SQL 执行边界：拒绝清单、只读检测、多语句拦截
sqlx/                    SQL 字面量 / 标识符转义
internal/                库私有：atomicfile / hashx / singleflight / limit
```

依赖方向（库内禁止成环）：

```text
registry → filedb / ducklake / cache / lease → blob → godatabase（根）→ sqlx
kv → registry,  sqlguard → sqlx
```

根包 `godatabase` **不 import 任何子包**（不做 facade 转发），否则子包反向 import 根包会成环。使用时按需 import 子包。

## 脱离 SimpleBase 单独使用

库不认识租户、项目目录或 Prometheus。宿主提供四样东西：

```go
// 1. 要打开的库
spec := godatabase.Spec{ID: "db1", ProjectID: "abcd1234", Kind: godatabase.KindUser,
    Status: godatabase.StatusReady, DatabasePath: "abcd1234/database.duckdb"}

// 2. 引擎 Factory（本地盘；S3 模式另传 Syncer）
f := &filedb.Factory{DataRoot: dir, CacheDir: cache, Syncer: filedb.NewNopSyncer()}

// 3. 单写注册表。resolver / logger / observer 都可为 nil
reg := registry.New(f, nil, registry.Options{Writable: true}, nil, nil)

// 4. 取连接
lease, err := reg.Acquire(ctx, spec, godatabase.ReadWrite)
defer lease.Release()
res, err := lease.Handle.Query(ctx, godatabase.Statement{SQL: "SELECT 1"}, 100)
```

| 接口 | 用途 | 传 nil |
|---|---|---|
| `PathResolver` | 为缺失 `DatabasePath` 的用户库补全路径（宿主在这里做租户归属校验） | 跳过补全 |
| `Logger` | 日志（签名与 zap 一致） | 静默 |
| `Observer` | 指标（句柄数、缓存、catalog 同步） | 不采集 |
| `blob.Store` | 对象存储（S3 / 内存）；库不内置 S3 实现，不依赖 aws sdk | 仅本地模式 |

**不要把 nil 的具体指针赋给这些接口**：会得到非 nil 接口，库内的 `x != nil` 判断失效并在调用时 panic。SimpleBase 的适配层 `internal/dbbridge` 对此有专门用例。

`godatabase.RecordingLogger` / `RecordingObserver` 是可断言的内存实现，供测试使用。

## 约束

- 所有写接口只能从 `registry` 取 `ReadWrite` handle，禁止绕过它直接构造 DSN。
- 数据库文件路径只在服务端使用：不出现在 API 响应、日志与审计中。
- **系统库（`Spec.Kind == "system"`）只通过 `ducklake` 打开**；`filedb` 拒绝打开系统库，用户库也不得使用 `ducklake`。
- **项目 ID 规则固定为 8 位 `[A-Za-z0-9-]`**（`blob.ValidateProjectID`），不可参数化。
- **S3 key 布局与 manifest JSON 字段是磁盘契约**，不得更改（金标 `internal/objectstore/testdata/keys.golden` 逐字节锁定，测试在根模块 `internal/objectstore`，因为金标经根的 `KeyBuilder` 转发覆盖库的 key 布局）。
- 没有任何旧数据兼容：不兼容旧布局、不提供回填迁移；遇到不认识的历史数据一律 fail closed，要求 reset。
- 单写保证仅限单进程；部署层必须固定副本数为 1。

## 测试

```bash
cd godatabase && GOWORK=off go build ./... && GOWORK=off go vet ./...
GOWORK=off go test ./... -timeout 900s
```

`ducklake`（约 100 秒）与 `lease`（约 30 秒）用例较慢，默认的 10 分钟超时在慢机器上不够，请放宽。
