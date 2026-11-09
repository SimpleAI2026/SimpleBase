# godatabase Agent 指南

嵌套独立模块 `github.com/linkxzhou/SimpleBase/godatabase`。概览见 [`README.md`](README.md)，仓库级规范见根 [`AGENTS.md`](../AGENTS.md)。

## 硬性约束

1. **不得 import 根模块任何包**（含 `internal/...`）。宿主类型（`catalog` / `observability` / `objectstore` / `auth`）一律经 `internal/dbbridge` 转换后再传入。
2. **依赖只允许** `duckdb-go/v2`、`go-sqlite3`、`google/uuid`、`zap`。不引入 `prometheus`、`aws sdk`、`echo`。新增依赖须用户明确要求；`go.mod` 版本与根模块保持一致，不单独升级。
3. **根包不 import 任何子包**（否则成环）；子包可以 import 根包。
4. **S3 key 布局与 manifest / lease / instance-format 的 JSON 字段是磁盘契约**：不得改动字段名、key 模板或 `blob.ValidateProjectID` 的规则。修改前先跑根模块 `go test ./internal/objectstore`（金标 `internal/objectstore/testdata/keys.golden` 逐字节锁定 key 布局）。
5. **系统库只经 `ducklake` 打开，用户库只经 `filedb` 打开**；`filedb` 对 `Spec.IsSystem()` 必须拒绝。
6. 没有旧数据兼容层：不要新增「兼容旧格式 / 回退旧路径 / 读取时回写历史状态」的分支，不兼容就 fail closed 并提示 reset。
7. 日志与错误文本不得包含 `DatabasePath`、DSN、数据根目录或缓存目录（`filedb.Factory.Open` 的错误已脱敏，新增路径要保持）。
8. 写路径、租约、manifest 冲突都必须 fail closed：失租后停止上传，manifest 冲突不得覆盖。

## nil 语义

`Logger` / `Observer` / `PathResolver` / `blob.Store` 的 nil 表示「关闭该能力」。库内判断一律写 `x != nil`，**因此宿主绝不能把 nil 的具体指针装进接口**。

## 修改后的验证

```bash
cd godatabase && GOWORK=off go build ./... && GOWORK=off go vet ./...
GOWORK=off go test ./... -timeout 900s
cd .. && go build ./internal/... ./cmd/... && go test ./internal/dbbridge ./internal/catalog ./internal/systemdb
```

新增 `Spec` 字段、`Observer` 方法或 `PathResolver` 语义时，同步修改 `internal/dbbridge` 并补对应用例；`godatabase` 内的 fake 实现统一用 `RecordingLogger` / `RecordingObserver`。
