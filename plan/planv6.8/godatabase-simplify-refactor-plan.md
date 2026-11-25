# godatabase 逻辑优化、精简与公共抽象计划（planv6.8）

> 日期：2026-11-24。
> 方法：用户指定的 `skills/simplify-code/SKILL.md` v2.2.0，采用 Assess / R-branch；结合 Go、后端资源与并发、结构重构、优化度量参考规范。
> **状态：评估与计划，尚未实施。** 本轮仅新增本文档，没有修改生产代码、测试、配置或依赖，没有执行数据库迁移、初始化、DDL、运行库操作。
> 目标：减少重复知识、明确资源和状态所有权、降低复杂流程的阅读成本；性能项必须先建立可重复基线。不以文件数、删行数或增加抽象层数作为成功指标。
> 其他模块计划的“激进策略”及后续授权不自动适用于 `godatabase`。

## 0. 阅读方式、范围与证据基线

### 0.1 决策摘要

建议按“特征测试 → 小范围等价清理 → 同包职责收敛 → 跨包共享机制 → 独立性能实验”的顺序实施。

- 先清理确实无意义的私有别名与测试噪声，随后处理重复规则和长流程；不要先创建一个大而全的数据库工具包。
- `filedb` 与 `ducklake` 都有快照、manifest 和远端对象操作，但状态模型、版本来源、同步时机和关闭条件并不相同。共享底层机制，不统一整个同步状态机。
- KV 的数据结构操作可以共享事务内的确定性子步骤，不适合用一个带许多开关的“万能命令执行器”替代现有分派。
- 注册表、写门、租约、同步调度和淘汰属于高风险区域。拆文件可以先做，改变锁范围、取消传播、调度或发布顺序必须单独建立确定性时序测试。
- 根包序列化、方言差异和分页辅助函数整体已较简洁；保留必要的显式分支，不为了表面重复引入反射、泛型框架或公共 API。
- 所有候选均为后续建议；“等价候选”不代表已经证明可以直接上线。公共契约或行为变化单列，未经确认不实施。

### 0.2 版本与工作区

- 基准修订：HEAD `dead32f`，提交摘要 `feat: 代码优化`。本文行号对应评估时工作区，不是永久定位；实施时以符号名重新定位。
- 评估开始时 `godatabase/` 没有 Git 已跟踪改动；仓库其他区域存在大量已暂存、未暂存及未跟踪改动。本轮不覆盖、不回滚、不提交这些改动。
- `plan/planv6.8/internal-simplify-refactor-plan.md` 已有修改；同目录其余三个模块计划当时未跟踪，均保持原样。
- 独立模块声明 Go `1.25.0`。没有执行 `go version` 或引擎版本查询，不把其他计划里的机器版本和测试结果当成本模块基线。
- 宿主为 macOS；没有验证 Linux、Windows、最低 Go 版本或不同 CGO/驱动构建组合。
- `godatabase/godb`、`godatabase/godb-cli`、`godatabase/main`、`godatabase/.session_tmps/` 命中忽略规则，未被 Git 跟踪；不读取二进制、不删除这些本地产物或临时数据，更不触碰 `.codebuddy/`。

### 0.3 规模与审查覆盖

只统计 `godatabase/` 下非隐藏目录的 `.go` 文件，包含注释与空行：

- 生产文件 65 个、12,086 行；测试文件 52 个、9,726 行。
- 源码匹配到 313 个顶层 `Test…` 入口、6 个 `Benchmark…` 入口；不是执行数量，也不等于覆盖率。
- `ducklake`：21 个生产文件、3,629 行；15 个测试文件、2,743 行。
- `kv`：11 个生产文件、2,491 行；9 个测试文件、1,789 行。
- `filedb`：7 个生产文件、1,334 行；3 个测试文件、959 行。
- `registry`：3 个生产文件、743 行；2 个测试文件、699 行。
- `cache`：2 个生产文件、466 行；3 个测试文件、584 行。
- `lease`：2 个生产文件、492 行；2 个测试文件、387 行。
- `blob`：2 个生产文件、473 行；1 个测试文件、199 行。
- `sqlguard`：2 个生产文件、579 行；3 个测试文件、426 行。
- `cmd`：1 个生产文件、678 行；1 个测试文件、156 行。
- 根包：7 个生产文件、791 行；4 个测试文件、775 行。
- 其余范围：`atomicfile`、`hashx`、`limit`、`singleflight`、`sqlval`、`sqlx`、`internal/testutil`、`bench`。

重点审查上述核心包生产流程、直接测试和必要的调用边界。宿主只按候选追踪相关调用，不是完整的宿主审计。已直接核对 `internal/dbbridge/dbbridge.go`、`internal/catalog/project_storage.go`、`internal/api/sql_handler.go` 等边界，以及 `planv6.8` 既有计划的相关保留项。

排除：业务 handler 重构、UI、其他独立模块实现、配置重设计、对象存储 SDK 替换、部署与真实数据修复。本文不承诺全仓死代码清零或缺陷清零。

### 0.4 实际执行与未执行的验证

已执行的均为只读检查：

1. `git status --short`，随后缩小到 `godatabase` 与 `plan/planv6.8`，记录工作区状态。
2. `git log -1 --format='%h %s'`，取得上述修订。
3. Python 标准库源码统计、函数入口索引、导入与直接引用检查；结果用于定位，不等同于编译器或完整调用图分析。
4. `git ls-files` 与 `git check-ignore` 检查上述产物归属。
5. `gofmt -l godatabase`：退出码 0，无输出；未使用 `-w`。
6. 源码导入扫描未发现 `godatabase` 导入 SimpleBase 宿主包；这只是静态文本检查，不代替未来的构建与依赖图验证。

**没有执行** `go build`、`go vet`、`go test`、race、coverage、benchmark、服务启动、CLI 数据命令、迁移或任何 DDL。理由：本轮是计划评估；现有真实引擎测试和 `bench/harness_test.go` 会通过 Factory 初始化并建测试表，DuckLake 扩展引导还包含 `INSTALL` / `LOAD`。不把测试名中的隔离意图直接当作已经完成的副作用验证。

读取不存在的候选路径未获得证据；后续通过文件发现定位真实文件。搜索结果为空时不据此认定符号不存在，关键引用已用源码或只读索引交叉检查。本文所有运行效果均为待验证，不写虚构的耗时、覆盖率、加速比或测试通过结论。

## 1. 冻结契约与重构边界

### 1.1 模块与公共 API

- 不导入 SimpleBase 宿主包；宿主类型由 `internal/dbbridge` 转换。
- 不新增第三方依赖，不修改 `go.mod` / `go.sum` 或升级引擎版本。
- 保留 `Spec`、`Factory`、`WriteHookFactory`、`PathResolver`、`Logger`、`Observer`、`AccessMode`、`QueryResult`、`SnapshotInfo` 和全部已导出入口。
- `README` 所说“公共 API 仍不稳定”不是删除 API 的授权；仓库无引用不等于外部无调用。
- `Logger`、`Observer`、`PathResolver`、`blob.Store` 的 nil 表示禁用能力。不要移除宿主将 typed-nil 转为真正 nil 接口的适配。
- 不把库内测试记录器移到生产根包；通用测试设施继续放 `internal/testutil`，避免反向依赖。

### 1.2 数据与访问安全

- 系统库只能经 `ducklake` 打开；`filedb` 必须拒绝 `Spec.IsSystem()`。
- S3 key 模板、项目 ID 规则、manifest / lease / instance-format JSON、快照信息 JSON 均为持久化契约。
- 不增加旧格式回退、隐式迁移、读时回写或自动 reset。
- 失租后拒绝继续写入/上传；manifest 冲突不得覆盖；失败路径保持 fail closed。
- 路径规范化、项目归属、软链接和根目录检查不是重复校验噪声；只能在证明保护对象相同后收敛。
- 错误、日志、观察器数据不可新增物理路径、DSN、密钥或正文泄露。
- 不能用“优化”名义绕过 SQL 守卫、写门、注册表或只读限制。

### 1.3 执行与错误语义

- `Query` 的 `maxRows <= 0` 仍表示不限制；超过上限返回错误，不静默截断；成功空结果的 nil/非 nil 形状不变。
- `Execute` 保留 `RowsAffected()` 失败时归零的现有行为。
- `Batch` 的事务模式失败回滚、返回 nil 结果；非事务模式保留已完成的结果及写入并停止后续语句。不能为减少循环统一失败语义。
- 错误身份、`errors.Is/As`、包装链、文案、日志次数及首个错误优先级均为兼容面。
- 析构、回滚、连接归还、关闭前同步、指标扣减和锁释放的相对顺序不能随抽函数变动。
- `context.Background()` 与请求 context 的替换会改变取消及收尾语义，不作为机械清理。

### 1.4 G-0：先记录根包分层规范与源码不一致

**位置**：`godatabase/AGENTS.md` 第 9 行、`README.md` 第 31 行；`spec.go` 第 11 行；`dialect.go` 第 7 行。

规范写明根包不 import 任何子包，但当前根包已经依赖 `blob` 与 `sqlx`：项目 ID 校验调用 `blob.ValidateProjectID`，方言引用调用 `sqlx.Ident/Literal`。这是已核对的规范/实现偏差，不等于当前存在 import cycle；不能据此宣称编译失败。

处理建议：

1. 本轮不擅自修改规则或现有依赖；新增抽象不得扩大此偏差。
2. 实施前确认目标分层。若严格恢复“根包不依赖子包”，需专门设计校验和 SQL 引用能力的归属，不能直接复制两份实现，也不能把问题转为导入另一个新子包。
3. 若希望允许根包依赖不回指根包的叶子包，必须先明确批准规则修改，并同步指南、README 与真实依赖检查。
4. 两种方案不能混用。本文其余局部候选不以放宽规范为前提；跨包抽象优先由 `filedb` / `ducklake` 等上层包使用。

**分类**：规范决策，非运行缺陷，P1 / 高边界风险 / 待确认。

## 2. 根包与叶子工具：小步精简及明确保留项

### S-01 私有常量别名与无断言测试噪声

**位置与证据**：

- `dialect.go` 第 58–67 行：`internalTablePrefix` 唯一用途是给公开 `InternalTablePrefix` 赋值；源码扫描只找到这两处。
- `coverage_test.go` 第 207–214 行：`TestAccessModeZero` 已断言零值字符串，随后 `_ = ErrQueryConcurrency`、`_ = ErrUnsupportedValue`、`_ = time.Now()` 不增加行为断言。`time` 在前面测试数据中仍有真实用途，不能盲删 import。

**方案**：把公开常量直接赋为现有字面量，删除私有转发常量；移除三个无断言表达式，保留整个零值测试和所有实际断言。

**保护**：公开常量名字、类型推断和值不变；不删除公共错误，不通过删测试美化覆盖率。

**验收**：公共调用方编译；`TestAccessModeZero`、`TestAccessModeString` 和方言/KV 相关测试维持行为；核对完整 diff 仅减少无意义间接层。

**分类**：等价候选，P3 / 低风险 / 源码已核实，运行未验证。适合作为首批小检查点，不值得单独扩大改动范围。

### S-02 路径校验与解析共享结果，而不是跳过校验

**位置**：`spec.go` 第 99–152 行，`EngineFromPath`、`ValidatePath`、`ValidateProjectPath`；调用包括 `filedb/factory.go` 的打开/快照路径、`filedb/path.go` 的本地路径解析、宿主 `internal/catalog/project_storage.go` 第 71–77 行。

**现状**：引擎推断先校验路径再取扩展名；项目归属检查校验路径后又生成标准路径比较。底层合法项目路径规则散落在调用组合中，但三个公共入口各自仍需要独立安全性。

**方案**：只有在需要调整路径规则时，考虑根包私有解析函数一次返回经验证的项目 ID / 引擎，让现有公开函数作为语义入口委托；返回值可用小私有结构体，保持具体错误发生顺序。不要暴露第二套“已校验路径”公共 API，也不要把文件系统软链接检查混进纯字符串解析。

**必须保留的反例**：

- 空串、反斜杠、NUL、绝对路径、重复 `/`、`.` / `..`、非法项目 ID、非 `database.duckdb|sqlite` 文件名。
- 同时非法的 `projectID` 与路径应保持原先首个错误；归属错误不能抢在格式错误之前。
- `systemProjectID == projectID != ""` 允许空路径；两个 ID 都为空不能误认为系统项目。
- `NormalizeEngine` 接受大小写与空白，`DialectFor` 当前不做相同规范化；不能顺手统一。

**测试**：现有 `TestNormalizeEngine`、`TestPathForProjectAndEngineFromPath`、`TestValidatePathRejectsUnsafe`、`TestValidateProjectPath`；新增表驱动锁定完整错误文本与组合非法输入。宿主 `wrapInvalid` 会拼接错误字符串，不能只测试 `errors.Is`。

**收益/取舍**：维护同一规则时减少多点变更；不是已证实的性能热点。如果新解析器比现有约 50 行更绕，保留现状。

**分类**：条件性等价提案，P2 / 中风险 / 暂不作为首批必做项。

### S-03 根包结果契约补特征测试，不强抽泛型集合工具

**位置**：`query.go` 第 30–153 行；`serialize.go` 第 22–169 行；`query_test.go`、`serialize_test.go`、`coverage_test.go`。

**建议**：后续涉及查询或序列化优化前，先补齐现有测试只判断长度或错误非空、没有锁定的行为：

1. 空查询结果、`SerializeRow(nil)`、`SerializeRows(nil)` 的 nil/非 nil 与 JSON 形状。
2. 查询恰好达到上限、超过一行、负上限、扫描/迭代错误、取消与 rows 释放。
3. 事务/非事务批次在第 0、第 N 条失败，结果形状、完整错误文案、后续语句未执行；commit 失败单列。
4. `json.RawMessage(nil)`、非法 JSON 的字符串回退、`uint64` / 大整数转字符串、时间时区与纳秒精度。
5. `duckdb.Union` 内部不支持值目前走 `mustSerialize` 文本回退，而普通 map/list 会返回错误，不合并成统一递归错误策略。
6. `duckdb.Map` 把键经 `fmt.Sprint` 变为字符串；不同原始键可能碰撞。不能改成排序、拒绝重复或“确定性最后值”而仍称等价。

**不做**：`SerializeRow` / `SerializeRows` 的两个短循环不抽成泛型 `Map`；整数 type switch 不换反射；事务与非事务两个循环不为去重增加错误策略回调；`Query` 已复用扫描指针并限制容量提示，不声称它仍在每行重复分配扫描指针。

**分类**：测试前置，P2 / 低测试风险；不修改现有输出断言来容纳新行为。

### S-04 保留已足够简洁的叶子能力

- `limit.Normalize` 是越界回默认值；`limit.Clamp` 是截到上限。例：`n=1000, def=20, max=100` 分别返回 20、100，不能合并。
- `sqlval.NullTime(&zeroTime)` 与 `NullTimeValue(zeroTime)` 不同：前者返回时间值，后者返回 NULL。不要统一空值策略。
- `sqlx.IsIdentByte` 面向 ASCII 词法扫描，`IsSafeIdent` 面向 Unicode 标识符验证，不能互换。
- `hashx.Short` 的 `n` 表示摘要字节数量而非十六进制字符数；保留非正数为空、超过 32 截断。
- `atomicfile.Write` 的固定 `.tmp`、失败清理、rename 错误原样返回属于当前行为。随机临时文件、fsync 或跨平台替换策略是单独可靠性设计，不能顺便改变。
- `DateBucketUnits` 是公开可变切片。当前格式 switch 不受它的运行时修改影响，不能以“唯一来源”名义让用户修改该切片就扩展 SQL unit 白名单。
- `IsMissingRelation` 和 `IsTransactionConflict` 匹配规则不同，不统一大小写或改错误分类。
- 叶子包的公开一行转发、`Execute` 对 `executeOn` 的包装、宿主 `dbbridge` 的 nil 适配都是有效边界，不以行数为理由删除。

## 3. 核心流程候选

本节按包列出经过源码取证的候选；所有实现、测试和性能实验均待后续执行。下文带目录的路径如未带仓库前缀，均相对 `godatabase/`；同一段落只写文件名时，沿用该段已标明的包目录（例如 DuckLake 段的 `local_state.go` 位于 `ducklake/`）。P1/P2/P3 表示维护收益优先级，不等于缺陷严重度。

### F-01 抽取快照生成内核，保留入口策略

**位置**：`filedb/remote.go` 第 432–503 行，`FileSyncer.snapshot`、`copyFileHashed`、`fileSHA256`；`filedb/snapshot.go` 第 16–88 行，`SnapshotFile`、`ValidateSnapshot`。

**事实**：两条入口都按引擎执行 SQLite `VACUUM INTO` 或 DuckDB `CHECKPOINT` 后复制。远端路径会移除既有 staging 目标且不调用完整 `ValidateSnapshot`；公开快照入口拒绝既有目标并做校验。DuckDB 复制已同时计算 SHA256，不能声称还有一次必然可删的重复哈希。

**实施步骤**：

1. 先补两入口的目标存在、引擎、取消和失败清理特征测试。
2. 提取“按引擎生成副本”私有函数，返回摘要/大小等真实已取得信息；是否还需 Stat 或读摘要由现有步骤决定。
3. 目录准备、覆盖政策、校验、元数据时间与清理策略留在各入口。
4. 首批只在 `filedb` 内共享，不创建跨 `ducklake` 的快照框架。

**保护/反例**：直接让远端入口调用 `SnapshotFile` 会改变覆盖行为与校验成本；反向合并会丢失公开拒绝覆盖契约。失败只能清理本次拥有的文件，不能误删已有目标。CHECKPOINT 后文件复制的一致性另见 B-08，不用抽函数掩盖。

**测试**：已有 `TestFileSyncerRoundTripColdStart`；建议补 `TestSnapshotEntryPointsPreserveDestinationPolicy`、`TestSnapshotFailureCleansOwnedArtifacts`，覆盖 SQLite/DuckDB 两引擎。

**分类**：P1 / 中风险 / 等价结构提案；先测后抽，不承诺性能收益。

### F-02 将 FileSyncer.Flush 分为捕获、准备、发布与成功记账

**位置**：`filedb/remote.go` 第 296–401 行，`FileSyncer.Flush`；`blob/blob.go` 第 33–56 行，`PutFiler` 与 `Store`。

**事实**：同一函数负责绑定/代次读取、快照、可选流式上传、整读回退、manifest 条件发布、本地水位与维护。现有关键顺序是上传 snapshot → `PutIfAbsent(manifest)` → 推进已同步代次；状态落盘和 prune 失败为后置 best-effort。

**方案**：引入包私有、不可变的本次任务参数（绑定、捕获的 `g0`、seq、epoch）；提取上传选择与发布子步骤，保持编排入口可以直接读出顺序。成功记账只能确认捕获的代次，不清掉上传期间新发生的写入。局部辅助函数不拥有整个同步器生命周期。

**边界**：本地状态写失败不能把已经发布成功改成失败重发；`Store` 未实现 `PutFiler` 时仍回退 `PutBytes`。项目级互斥、关闭/失租取消属于独立可靠性变更，见 B-06/B-07，不混入机械拆分。

**测试**：已有 `TestFileSyncerUploadFailureKeepsDirtyAndRetries`、`TestFileSyncerFlushKeepsDirtyForWritesDuringUpload`；建议补 `TestFileSyncerPutFileAndPutBytesEquivalent`，记录 PUT 顺序、次数、错误及水位。

**分类**：P1 / 高风险 / 等价职责拆分；依赖 F-01 的边界测试，性能再走 O-01。

### F-03 分离 Factory 的引擎策略与资源所有权

**位置**：`filedb/factory.go` 第 117–225 行 `Factory.open`、第 286–355 行 SQLite connector 与 `trackedConnector`。

**事实**：打开流程混合路径恢复、占用标记、远端恢复、引擎配置、连接池、Ping、同步绑定。SQLite PRAGMA 按连接执行，DuckDB 配置通过 `singleInit/initDone` 区分初始化作用域。多个失败路径需要释放已标记文件和连接资源。

**方案**：先提取同包引擎 connector 与初始化参数构造，保持显式的 SQLite/只读/DuckDB 分支；将临时目录计算放在同一个明确位置。打开编排保留一个清晰的资源所有权转移点：转交前失败由打开流程清理，转交后由 connector 关闭负责。不要为两种引擎创建可插拔 driver 注册框架。

**保护**：SQLite PRAGMA 不能改成全库只一次；DuckDB `lock_configuration` 不能改成每连接重复执行。临时目录白名单必须早于关闭外部访问和锁配置。脱敏包装、系统库拒绝、只读 connector 安全策略不变。

**测试**：已有 `TestProjectFileRoundTrip`、`TestOpenCorruptFileErrorHidesPhysicalPath`、`TestDuckDBUserDBRejectsExternalFileRead`、`TestSQLiteReadOnlyConnectorForcesQueryOnly`；建议补 `TestFactoryOpenFailureReleasesPath`、`TestTrackedConnectorInitScope`。

**分类**：P1 / 高风险 / 等价结构提案；先失败注入再移动资源释放。

### F-04 对象分页遍历去重，但分页修复单列

**位置**：`filedb/remote.go` 第 563–596 行 `latestManifestByList`、第 669–693 行 `prune`、第 705–732 行 `PurgeProject`；`blob/blob.go` 第 147–172 行内存分页/批删实现。

**事实**：latest 与 purge 有相似的逐页循环；latest 已只读取选出的最新 manifest。prune 当前只处理首批 1000 项，改变这一点会改变实际删除行为。

**方案**：先在 `filedb` 内抽每页访问助手，原样传递不透明 cursor，让 latest 保持最大 seq 归约、purge 保持逐页删除；helper 不解析 manifest，不先把所有 key 装入内存。prune 接入全分页是 B-09 的独立行为修复，不在首批等价抽取时自动完成。

**保护**：不能依据 MemoryStore 的“末 key 游标”推断真实服务 cursor；批删不超过既有上限；purge 前缀不扩大到上传文件区。边枚举边删除的后端语义需专门测试。

**测试**：已有 `TestLatestManifestByListReadsOnlyLatest`、`TestFileSyncerPurgeProjectKeepsFiles`、`TestMemoryListAndDeleteMany`；建议补不透明游标、空页但有 next、分页错误、不完整删除和多页保留样例。

**分类**：P2 / 中风险（prune 接入为高风险）/ 分离纯去重与行为修复。

### D-01 收敛同步延迟计算与计时器取消样板

**位置**：`ducklake/syncer_schedule.go` 第 143–193 行 `MarkDirty/scheduleLocked`、第 215–238 行 `ResumePeriodic`、第 336–351 行同步收尾重排程。

**事实**：三处计算 debounce 或 `min(Interval, MaxLag)`；多个关闭/解绑/暂停入口停止并删除 timer。失败至少一秒的退避是另一种策略。

**方案**：提取纯延迟策略和要求已持 `mu` 的停止 timer helper；各入口自己的是否允许调度条件留在原位置。文件无需再次细碎拆分，重点是规则只有一个实现。

**反例**：interval 不能因连续写入而不断延期；debounce 仍应重置。暂停周期任务不等于禁止手动 Flush。同步完成后的绑定身份检查不能移除。给计时器加代次识别属于 B-10 独立修复，不混入去重。

**测试**：已有 `TestPausePeriodicDefersSchedule`、`TestCatalogSyncerInflightWriteEventuallySynced`；建议补三入口策略表、interval 连续写不延期、失败退避、pause/resume/rebind。优先可控回调，不只使用 sleep。

**分类**：P1 / 中风险 / 等价候选；涉及回调身份的部分单列高风险。

### D-02 明确 local-state 地址与读改写所有权

**位置**：`ducklake/factory.go` 第 485–507 行 `AfterWrite`；`syncer_capture.go` 第 182–195 行；`syncer_prune.go` 第 118–144 行；`local_state.go` 第 30–70 行。

**事实**：三处读取整个 `LocalState`、修改不同字段再保存。`ReadLocalState/SaveLocalState` 接受 cache root 并追加 `databaseID/catalog`；`AfterWrite` 却传 catalog 的父目录，存在需独立验证的地址偏差，见 B-11。原子 rename 不等于整个读改写事务互斥。

**分两阶段**：

1. 先列“缓存根/实际 catalog 目录/具体 state 文件”三种地址，编写 local/remote、不同 DataRoot/CacheDir 的路径特征测试。
2. 纯结构抽取可统一到现有 `Layout` / 私有文件定位；修正实际路径、按 state 文件串行读改写和防止丢字段是独立行为修复。不要未经授权新增公开 `UpdateLocalState` API。

**保护**：snapshot、synced snapshot、manifest seq 与 prune progress 各有含义，不能对所有字段一律取 max。manifest 成功后才推进同步水位；状态保存失败仍仅记录警告。不能用全局 cacheDir 覆盖每库绑定路径。

**测试**：已有 `TestC4_WatermarkAndLocalStateAdvanceAfterManifest`、`TestC4_LocalStateFailureDoesNotFailSync`；建议补 `AfterWrite` 与 sync/prune 可控交错、不同目录布局和重启读取锚点。

**分类**：P1 / 高风险 / 先表征、后决策；不能以等价重构直接修复地址或并发语义。

### D-03 固化捕获与发布的数据边界，不合并锁域

**位置**：`ducklake/syncer_capture.go` 第 19–114 行 `captureCatalog`、第 117–210 行 `syncOnce`；`syncer_publish.go` 第 21–69 行 `publishSnapshot`。

**方案**：用小型私有结果结构表达捕获的 snapshot ID、engine、字节以及发布后的 seq/epoch/key，降低多个同类型返回值交错风险。保持 capture/publish/成功记账三段，而不是将它们放进一个长时间持锁函数。只有字段真正被多个步骤消费时才引入结构，不扩大公共 API。

**冻结顺序**：先借连接 → 再持写锁 → flush → 取 snapshot → COPY → DuckDB CHECKPOINT → 复核 → 读字节 → 释放锁 → 上传。锁内 DETACH 使用现有 `wctx`，不能切回普通 context 导致自锁。上传、manifest、内存水位、本地状态、观测先后不变。

**独立加固**：捕获后失租、snapshot PUT 与 manifest 之间失租、同 key 已存在但字节不同，见 B-07/B-12；这些不是结果结构重命名可以解决的。

**测试**：已有 `TestWriteGateSerializesFlushAndWrites`、`TestCatalogSyncFailureKeepsUnsyncedAgeAndManifest`、`TestC4_LostLeaseFlushDoesNotUpload`。建议补阶段阻塞、已存在对象 hash/size、CHECKPOINT/DETACH 失败及从已发布快照真正重开。

**分类**：P2 / 高风险 / 结构候选；不借机改变快照格式或发布协议。

### D-04 将 prune 拆成计划、受控动作与进度提交

**位置**：`ducklake/syncer_prune.go` 第 11–33 行 `schedulePrune`、第 45–114 行 `pruneVersions`、第 118–144 行进度读写。

**方案**：先保持原行为，把区间/保留引用计算、逐项处理、连续完成前缀提交分开。业务门禁继续显式存在，不抽一个隐去租约知识的通用“删除旧文件”组件。

**不可删除的防线**：manifest 永不删；保留窗口仍引用的老 snapshot 不删；先写孤儿账本再删对象；失败不能越过该 seq 推进。Stop timer 不等于正在执行的 prune 已停止。

**需要另行确认**：循环中失租停止、保留窗口缺 manifest 时是否 fail closed、原本 `break` 后返回 nil 的阶段失败是否对外返回 error，均改变现有行为。等价结构检查点保留当前错误政策，再开可靠性变更。

**测试**：已有 `TestMaintenanceGate`、`TestPruneRecordsOrphansAndGateBlocksLostLease`；建议用事件 Store 验证 Record→Delete，故障只推进连续前缀，重复引用、缺 manifest、关闭/发布交错与重启续扫。

**分类**：P1 / 高风险 / 结构提案与行为修复分开。

### K-01 合并集合删除后的元数据收尾

**位置**：`kv/hash.go` 第 163–170 行；`kv/set.go` 第 89–96、164–169 行；`kv/key.go` 第 97–108、127–140 行。

**事实**：Hash.Delete、Set.Delete 在实际删除大于零时均执行 `touchAfterWrite(-aff)` 再 `deleteIfEmpty`；Set.Pop 重复相同收尾，减量固定为 1。

**方案**：在 `KeyRepo` 提取私有 `afterCollectionDelete` 一类小函数，只封装已发生删除后的元数据处理。SQL、校验和外层错误包装留在各仓库。第一步不减少 SQL 次数，不跨事务。

**保护**：零删除不更新 version/mtime；正删除只更新一次；len 按真实删除计数；删除到空移除 key。Delete 缺失/过期返回零，Pop 缺失仍为 `ErrNotFound`，非法空参数与错误类型保持不同。

**测试**：已有 `TestHashDeleteAutoRemoveKey`、`TestSetBasic`、`TestSetPop`；建议补 `TestCollectionDeleteNoopPreservesMeta`、`TestCollectionDeleteDuplicateArgs`，比较 len/version/mtime 与数据同时变化。

**分类**：P1 / 中风险 / 适合较早实施的同包等价抽象。

### K-02 收敛 Scan 参数与截页规则，保留各自 SQL

**位置**：`kv/key.go` 第 367–404 行；`kv/hash.go` 第 257–299 行。

**事实**：空 pattern→`*`、非正 count→100、查询 count+1、截到 count、只有存在额外项才返回 next cursor 的规则重复。

**方案**：优先提取纯参数归一化；截页若返回容器不同，允许保留短循环，而不是新建泛型数据库扫描器。Key/Hash 的 SQL、rows 解码、TTL/类型检查仍在各仓库。

**保护**：严格 `> cursor` 与字典序不变；恰好 count 项且没有后续时 next 必须为空；Hash 游标来自有序页，不能遍历结果 map 生成。Hash 缺失错误不能改成空结果，Key 的类型/过期过滤不能漏。

**测试**：已有 `TestKeyScan`；建议补 `TestHashScanPagination`、`TestScanExactPageBoundary`、默认 count 与空 pattern。新 helper 若只压缩两行则取消截页抽象。

**分类**：P2 / 中风险 / 有限抽取。

### K-03 Hash 写路径避免重复元数据探测：作为性能/语义提案

**位置**：`kv/hash.go` 第 17–75 行 `Get/GetMany`、第 83–105 行 `Set`、第 203–211 行 `Incr`；`kv/key.go` 第 53–83 行守卫。

**事实**：写操作已经执行 `guardWrite`，随后调用公共 Get/GetMany 再次执行元数据校验；Set 为计算新增字段数还读取 value。

**方案**：保持公开读 API 完整校验；考虑私有“已守卫写事务内读取字段/存在性”方法，Set 只读 field，Incr 只读需要的 value。明确返回的错误包装不要变化。通过计数型执行器比较 SQL 次数与大 value 读取量。

**重要反例**：TTL 可能在第一次 `guardWrite` 与第二次 `getMetaOf` 之间到期，删除第二次校验可能改变当前结果；若使用不同时钟快照，也不能直接宣称等价。先表征临界 TTL，再决定保留二次校验还是明确批准写事务时间语义。重复 GetMany field 当前只填写第一次出现的位置，不能顺带“修正”公开返回。

**测试**：已有 `TestHashSetGetItems`、`TestHashIncr`；建议补过期重建、类型冲突、TTL 临界、整数解析/溢出、只更新旧字段仍更新版本。性能与错误覆盖分开验收。

**分类**：P1 / 高风险 / O-03 的待测提案，不属于首批等价删除。

### C-01 CLI 装配和命令职责拆分

**位置**：`cmd/godb-cli.go` 共 678 行；第 138–162 行 `env.openReg`，第 198–209 行 `cmdOpen`。

**事实**：两处都装配本地 Factory、可写 Registry、Acquire 与清理；`cmdOpen` 固定读写，`openReg` 按参数选模式，Acquire 失败的 Shutdown context 存在差异。

**方案**：先按参数/环境、资源装配、命令实现的真实边界作同包文件搬移；再提取小装配核心。只有确认失败清理 context 语义一致后才让 `cmdOpen` 直接复用 `openReg`，否则保留两层策略包装。

**保护**：保留 local-only、引擎选择、项目 ID 校验、Release→Shutdown 顺序、错误脱敏、JSON 字段、stdout/stderr 与退出码。不要调整环境变量配置来源或引入新 CLI 框架。

**测试**：已有 `TestSQLReadOnlyRejectsCTEDelete`、`TestSQLRWRoundtrip`、`TestInvalidProjectIDSanitized`；主要覆盖 openReg/spec，不代表 cmdOpen 输出已覆盖。建议补 `TestCmdOpenJSONContract` 与失败清理事件测试。

**分类**：P2 / 中风险 / 先纯搬移，后有限去重。

### R-01 合并 Handle 关闭骨架，但保留 flush/no-flush 意图

**位置**：`registry/handle.go` 第 150–172 行；`registry/registry.go` 第 269–279 行。

**事实**：两条关闭实现共享 `closeOnce`、置 closed、关 conn；差别是是否执行 `beforeClose`。关闭回调失败仍要关闭连接，且回调错误优先于 conn.Close 错误。

**方案**：私有关闭核心可接受明确的 flush 策略，现有命名包装表达意图；也可让 registry 的现有分派直接调用共享核心，选择间接层最少的方式。不要额外把首次 error 缓存到字段，否则重复 Close 的返回行为会变。

**保护**：失租的 Remove/no-flush 绝不能调用上传；不在 registry 全局锁下执行网络 Flush；`sync.Once` 不自动保证 observer 只扣一次，观测收尾仍留在单一出口。

**测试**：已有 `TestHandleQueryExecuteBatchAndHooks`、`TestObserverTracksOpenHandles`、`TestShutdownForceCloseActive`、`TestLease_ReleaseIdempotent`；建议补两模式交错、重复 Close、回调失败仍关连接。B-01/B-02 并发问题先建立回归边界。

**分类**：P2 / 高风险 / 条件性等价抽取。

### R-02 缓存单次候选扫描同时收集 size 与 mtime

**位置**：`cache/manager.go` 第 134–175 行 `Usage/dirSize`；`cache/evict.go` 第 66–120 行 `dirLatestMtime/collect`，第 223–258 行容量/数量限制。

**事实**：无 registry 最近访问记录时，同一目录先扫描 mtime，再扫描 size；EnsureCapacity 还会通过 Usage/Evict 重复遍历。

**方案**：先仅合并单次 Evict 的候选信息收集，一次 walk 返回 size 与最新 mtime。registry 的 `LastUsedOf` 优先级不变。不要首批跨 Usage/Evict 复用快照，更不加持久全局 LRU/大小缓存。

**风险**：mtime 读取容错与 size 读取失败可能不同，helper 不能统一吞错误；两次扫描改一次会改变并发文件变化时的观测时点，需要明确可接受快照语义。扫描后新 Acquire 仍须重新检查活跃状态，不能凭旧快照删目录。新增取消检查也要作为单独行为改善说明。

**测试**：已有 `TestEvictFreesSpace`、`TestEvict_SkipsActiveDatabases`、`TestUsage_NestedFiles`、`TestEvictCancelMidwayAndEnsureCapacityError`；建议补相同 mtime、不可读子项、扫描中变化、关闭同步失败保留目录。B-04 数量目标问题另行修复。

**分类**：P1 / 高风险 / O-02 性能提案；需要 I/O 计数及结果对照。

### R-03 singleflight 只合并登记，不合并等待政策

**位置**：`singleflight/singleflight.go` 第 27–61 行 `Do/TryDo`；第 86–111 行 `start/run`。

**事实**：nil 接收者、map 初始化、锁内查找/leader 登记相似，执行及收尾已经共享。

**方案**：如有足够净收益，提取返回 call/leader 的短私有登记函数；Do 负责等待，TryDo 仍立即返回 `ErrInFlight`。锁不跨等待或 `fn` 执行；不为抽两段代码增加额外状态结构。

**保护**：nil Group 直接执行；panic 仍唤醒等待者；Forget 后旧 leader 完成不能删除新 leader，保留身份比较。不将 registry opening 替换为此 Group，两者取消与错误重试不同。

**测试**：已有 `TestDoSharesResultAcrossWaiters`、`TestTryDoBusyThenIdle`、`TestLeaderPanicWakesWaiters`、`TestForgetDoesNotLetOldLeaderEraseNewLeader`、`TestNilGroupRunsDirectly`、`TestZeroValueUsable`。

**分类**：P3 / 中风险 / 可跳过；112 行模块已较紧凑，不为统一形式强改。

### R-04 将只用于 Acquire 的 ownerID 从代理字段降为局部变量

**位置**：`lease/lease.go` 第 65–79 行 `storeProxy`、第 213–236 行 `Acquire`。

**事实**：`ownerID` 在构造代理时写入，只用于构造 Payload 与一次 owner 比较；同包源码和测试引用核对未发现续约或其他路径读取。它不是完全无读取的“死字段”，而是不需要长期保存的临时状态。

**方案**：计算局部 ownerID，构造 payload 和比较使用此局部值，删除代理私有字段。只减少状态，不改变远端结构。

**保护**：`Payload.OwnerID`、`BootID` 和 instance/PID/boot 组合格式保持；不能只保留 PID 而误认重启前持有者。续约 context 不改绑到 Acquire 请求。

**测试**：已有 `TestAcquireFirstWriter`、`TestAcquireSecondInstanceBlockedWhileHeld`、`TestRestartSamePIDLogged`、`TestTakeoverAfterStopRenew`。

**分类**：P2 / 低风险 / 适合首批；不宣称显著内存收益。

### Q-01 SQL 守卫暂不结构统一，保留显式语义

**位置**：`sqlguard/classifier.go`（563 行）、`validator.go`（16 行）、`ducklake/gated_conn.go`，宿主敏感列词法检查。

补充核对：`ducklake/write_gate.go` 第 125–130 行已经调用 `sqlguard.Validate(q, ReadOnly)`，不存在需要合并的第二套 DuckLake 写门词法器。写门将校验不通过的非空 SQL 保守归为写操作并交驱动执行，空白 SQL 不取写锁；这与 API 的校验失败直接拒绝不同。不得把锁分类改成 SQL 授权。宿主敏感列识别仍是另一消费契约，不因主文件长就引入 parser 框架或公开 lexer API。

同包可选候选：`sqlguard/classifier.go` 第 227–255、326–389、403–425 行的跳过字符串/注释分支，可在差分表征后评估复用第 475–524 行现有私有 skip 原语；保持未闭合注释、四种 ASCII 空白、尾注释片段、quoted identifier 解码与首错优先级，不改变接受集合。该候选为高风险提案，不升级为直接实施项。

已有保护入口包括 `TestValidate_NestedBlockComments`、`TestValidate_StringEscapeSingleQuote`、`TestValidate_IdentifierEscapeDoubleQuote`、`TestValidate_ReadOnlyCTEGuard`、`TestValidate_ReadOnlyExplainGuard`、`TestValidateRejectsReservedIdentifiers` 与 `TestSplitStatements`。任何未来抽取都要比对三类调用方的接受/拒绝集合、首错文本和内部控制 SQL，不只比对 token 输出。

**分类**：skip / 保留既有安全边界；未来确有重复知识证据再提案。

### 3.1 建议优先级与依赖索引

- 第 0 批先表征：S-03、D-02，以及 B-01/B-02/B-03/B-07 的确定性时序；G-0 只阻塞涉及分层调整的候选，不阻塞局部精简。
- 第 1 批优先：S-01、R-04；二者局部且容易复核。
- 第 2 批优先：K-01、D-01 的纯策略去重、F-01、C-01 纯搬移。
- 第 3 批：F-02、F-03、F-04 的等价部分、D-03/D-04 结构部分、R-01；需完整生命周期测试。
- 条件项：S-02、K-02、R-03；若抽象收益小则明确取消。
- 独立性能/行为项：K-03、R-02、O-01 至 O-06 与 B-*，不与等价重构捆绑验收。

## 4. 公共抽象的目标与准入条件

### 4.1 先同包，后跨包

每次抽象必须回答四个问题：

1. 是否存在至少两个真实调用点，且维护的是同一规则，而不仅是代码长得相似？
2. 错误文本、首错顺序、资源所有者、取消语义是否能原样保留？
3. 调用者是否仍能直接读懂副作用顺序，而不需要理解一组布尔开关？
4. 新接口、回调和文件的复杂度是否小于被移除的重复？

任何一个答案不明确，先保留局部重复。跨包共享优先复用现有 `atomicfile` / `hashx` / `limit` / `sqlx` 能力；缺少能力时先尝试包私有函数。确需新增内部包必须列出依赖图，不能让根包继续增加对子包的依赖，也不能令叶子工具回指运行时。

### 4.2 值得共享的知识类型

- 完全相同的文件摘要计算、选用可选上传能力等确定性底层步骤，前提是错误与流式路径一致。
- 同一数据结构的键元数据维护、TTL 计算或剩余成员计数，前提是位于相同事务且 SQL 执行顺序不变。
- 同一种 catalog 元数据查询的构造，前提是默认 schema、标识符转义和错误边界一致。
- 同包中多处更新相同生命周期字段的收尾，前提是锁和资源所有权一致。
- 测试中的记录 Store/Observer 或阻塞时序控制，前提是已被多个包真实需要；优先完善 `internal/testutil`，不先搭测试框架。

### 4.3 明确不共享的部分

- 用户单文件库与 DuckLake catalog 的完整恢复、同步、版本推进和保留策略。
- 请求级 SQL 拒绝策略与连接级写锁策略不能统一；写门已经复用 `sqlguard`，这里保留的是对分类结果的不同处置，而非维护两套词法器。
- 用户库 DuckDB、SQLite 与系统 DuckLake 的打开、扩展安装、安全开关和连接池引导。
- 普通查询与事务批次的错误、局部成功语义。
- 本地缓存中 `system`、用户项目文件、legacy UUID 目录等不同布局的路径生成和保护判断。
- 面向生产的 Store 与 `MemoryStore` 的可变状态细节；不能因它常用于测试就删除复制、并发或分页契约。

### 4.4 建议的实施文件策略

- 保留公共文件与类型入口，新增文件只对应明确职责；不是每个 helper 一个文件。
- 大文件先做不改逻辑的搬移，再另一个检查点去重。避免把“搬了 600 行”包装成“优化了 600 行”。
- 测试迁移只改变所属文件和私有符号绑定，不弱化测试、不删除失败样例、不更新 golden 掩盖行为变化。
- 不预设 500 行硬限制；例如 563 行的词法分类器可以保留完整状态上下文，只有职责明确时才拆。
- 不新增兼容旧磁盘格式的代码；需要保留的是现有公共 API，而不是设计新的历史数据回退。

## 5. 性能实验设计：先证明瓶颈，再保留优化

所有以下工作负载均为**计划值，不是测量结果**。完整重构与性能变化分开对照，原始版本基线不能被中间版本覆盖。

### O-01 快照 I/O 与内存峰值

- 关注：`filedb` / `ducklake` 快照复制、计算摘要、读取上传及流式 `blob.PutFiler` 分支。
- 工作负载：小文件与逐步增大的快照（例如 1 MiB、64 MiB、256 MiB；机器容量允许才继续），分别使用支持/不支持 `PutFiler` 的测试 Store。
- 指标：读盘字节数、完整文件扫描次数、分配字节、峰值内存、上传次数、取消后继续工作的量；Go heap 与驱动原生内存分别解释。
- 保护：摘要与快照内容一致、错误先后一致、上传后再发布、冲突不覆盖、失败不推进本地版本。
- 停止：如果去掉一次读盘需要破坏校验顺序、扩大 Store API 或引入共享可变缓冲区，先作为独立设计，不实施投机性优化。

### O-02 缓存扫描与淘汰

- 工作负载：100 / 1,000 / 10,000 个目录，不同文件数、空目录、丢失目录、被保护目录、访问时间相同、淘汰中并发访问。
- 指标：遍历次数、`Stat` 次数、处理耗时、分配、候选选择结果与已淘汰字节数。
- 第一选择：同一轮内复用已经取得的信息，保持过滤和排序；不是先加全局大小缓存或 LRU 索引。
- 缓存容量实时性、软链接处理、相同时间的选择顺序不能悄悄改变；如果需要降低目录扫描频率，必须说明这是新鲜度变化。

### O-03 KV 多元素命令与 SQL 次数

- 工作负载：1 / 10 / 100 / 1,000 个元素，区分全新、全存在、重复参数、混合类型、过期键。
- 指标：每条命令 SQL 数、事务数、总耗时、分配、实际返回计数。
- 先采用计数型假执行器和临时 SQLite/DuckDB 对照，避免把驱动差异误归因于代码重构。
- 批量 `IN` / 批量插入会涉及参数上限、重复元素计数、错误定位与事务可见性；只有证明这些一致后才保留。
- 不把原本逐步提交操作包成一个事务，也不并行提交同一键的修改。

### O-04 注册表命中、冷打开和关闭竞争

- 工作负载：同一个热库多请求、不同库冷打开、打开失败、Acquire 与 CloseDatabase / Close 交错；用阻塞 FakeFactory 控制顺序。
- 指标：Factory 打开次数、等待/命中耗时、互斥锁持有时间、关闭次数、goroutine 收尾及引用计数。
- 任何“把 I/O 移出锁”方案都要定义 opening / closing 状态与等待者行为。若只是把锁等待换成重复打开或关闭复活，应判定失败。
- 不把 `singleflight` 的替换视为等价机械操作：首个调用的 context、等待者取消、panic 与发布失败的语义要逐项对齐。

### O-05 查询与序列化分配

- 工作负载：0 / 1 / 100 / 1,000 行、窄/宽列、嵌套 JSON、BLOB、DuckDB decimal / UUID / interval。
- 指标：`ns/op`、`B/op`、`allocs/op`，同时比较原始结果与 JSON；时间字段排除数值相等比较但保留计时范围。
- 当前 `Query` 已复用扫描指针，输出行值仍需独立所有权；不允许复用同一行缓冲导致历史结果被覆盖。
- 没有 profile 支持不增加 `sync.Pool`，不把 HTTP 序列化并入根包 Query，也不改变 `QueryResult`。

### O-06 复用现有 DuckLake 读放大基准的边界

现有 `bench/read_test.go` 的六个入口：`BenchmarkLogsQueryDense`、`BenchmarkLogsQueryInlined`、`BenchmarkMetricsSummaryDense`、`BenchmarkMetricsSummaryInlined`、`BenchmarkUserPointLookupDense`、`BenchmarkSingleWriteCost`。

它们对照 inlining 与小文件堆积，适合检测 DuckLake 初始化/维护变化对本地读写的影响，不是整个模块的通用性能基线，不能证明远端 COS/S3 延迟下降。`bench/harness_test.go` 有意内联同形宿主 SQL 以保持独立模块边界，不应为去重 import `internal/systemdb`。

### 5.1 通用测量纪律

- 同机器、同 Go/驱动版本、同数据、同运行模式；分别记录冷/热状态。
- 每组至少重复多次，报告所有结果或分布，不挑最好一轮。已有 `benchstat` 可用则使用，不为此安装依赖。
- 一次只测试一个主要假设；至少保留“原始 → 纯结构重构 → 优化”三个可区分结果。
- 不预先承诺百分比加速或减少多少行；没有稳定收益且复杂度上升就撤销该优化，不撤销用户原有改动。

## 6. 验证与实施门禁

### 6.1 环境安全门禁

未来执行前必须确认：

1. 测试只用 `t.TempDir`、内存 Store、fake 或一次性数据库，不读取真实配置，不连接已有运行库。
2. 已获准执行测试 fixture 的建表/初始化；这不包含服务 `init_db`、真实库 ALTER/DDL 或迁移授权。
3. DuckLake 扩展是否已经就绪；如需下载、写扩展目录或网络访问，明确记录。不能以测试 Skip 当成覆盖通过。
4. 预期文件写入位置明确。评估模式不运行会改 checkout 的检查；必要时由用户批准一次性副本验证。
5. 模块与宿主的当前已有失败分开记录。宿主正在修改，不能把它的基线问题算成此重构的新回归。

### 6.2 后续建议命令（本轮均未运行）

模块门禁，在 `godatabase/` 执行：

- `GOWORK=off go build ./...`
- `GOWORK=off go vet ./...`
- `GOWORK=off go test ./... -timeout 900s`
- `gofmt -l .`（检查，不自动写回无关文件）

按候选选择局部门禁：

- 根包/工具：`GOWORK=off go test . ./atomicfile ./hashx ./limit ./sqlval ./sqlx ./blob -count=1`
- KV/守卫/CLI：`GOWORK=off go test ./kv ./sqlguard ./cmd -count=1 -timeout 900s`
- 单文件库：`GOWORK=off go test ./filedb -count=1 -timeout 900s`
- DuckLake：`GOWORK=off go test ./ducklake -count=1 -timeout 900s`
- 并发基本门禁：`GOWORK=off go test -race ./registry ./cache ./lease ./singleflight -count=1 -timeout 900s`
- 如涉及同步并发，再执行：`GOWORK=off go test -race ./filedb ./ducklake ./kv -count=1 -timeout 900s`；先确认 CGO/驱动支持，记录失败和 Skip。
- 已有基准：`GOWORK=off go test -run='^$' -bench=. -benchmem -benchtime=20x -count=5 -timeout 20m ./bench`。实际耗时可能很长，应按待测入口缩小范围，不能因超时减少样例后宣称全量通过。

宿主边界门禁，在仓库根执行：

- `go build ./internal/... ./cmd/...`
- `go test ./internal/dbbridge ./internal/catalog ./internal/systemdb`
- 对象键、上传和 manifest 相关改动额外 `go test ./internal/objectstore`，保护 `internal/objectstore/testdata/keys.golden`。
- SQL/方言/序列化调用变化额外 `go test ./internal/api`；写路径涉及系统库保护时必须有拒绝用例。
- 最终集成按项目规范运行根模块 `go test ./...`；根模块的 `./...` 不包含独立模块测试，二者不能互相替代。

以上命令是实施验收清单，不是自动执行指令。不要运行服务来“顺便补迁移”。

### 6.3 必须补强的失败/边界场景

- nil Store、nil Logger/Observer、typed-nil 宿主输入；禁止在库内引入反射式 nil 处理掩盖宿主错误。
- 恰好上限、超过上限、0、负数、空集合、重复元素；输出 nil/空形状。
- 开库成功后任一步失败的资源关闭，Close 重复调用，Release 重复调用。
- 在读取、捕获、上传、发布、确认、落本地状态、清理各阶段注入一次失败，断言后续不该发生的动作没有发生。
- `PutIfAbsent` 冲突、manifest 版本变动、丢失对象、过期租约、失租后调用、父 context 取消。
- 缓存碰到保护目录、正在使用目录、路径逃逸、软链接、移除失败；错误与指标计数不变。
- KV 键不存在/过期/类型不匹配/删除到空/多个参数重复；事务内查询与更新的顺序。
- SQL 引号、注释、多语句、NUL、CTE、内部维护语句，守卫与写门不能只覆盖共同的 `SELECT 1`。

### 6.4 接受标准

一个候选只能在以下条件满足后标记完成：

- [ ] 输入/输出、错误、磁盘格式、资源顺序与相关调用方已盘点。
- [ ] 原始版本已有可复核基线，所有新失败被解释并修复或撤销本候选。
- [ ] 改动只覆盖一个逻辑目标；无依赖升级、配置扩张、无关格式化。
- [ ] 测试断言未弱化，必要的特征用例已补齐。
- [ ] 改动确实消除重复知识或让职责更明确；没有仅把复杂度转移到通用框架。
- [ ] 性能项有同工作负载对照，无收益的额外复杂度不保留。
- [ ] 运行未覆盖的平台、驱动、外部服务和 Skip 已逐项说明。
- [ ] `git diff` 确认未动用户既有改动，未自动迁移/DDL/清理产物。

## 7. 分批落地、依赖与回退

### 第 0 批：契约和可测试性

- 确认 G-0 的分层目标；未确定前不做新增根包依赖。
- 记录原始工作区、构建/测试基线、测试副作用和实际引擎版本。
- 按后续候选补特征测试，尤其错误文本、事件顺序、重复输入和取消。
- 交付：可重复的原始行为基线；不更改生产逻辑。

### 第 1 批：低风险局部精简

- S-01；以及核心包中已经确认的不可达私有分支、无调用私有字段/参数。
- 删除前再次查引用和间接注册；不删除导出成员，不扫描 ignored 目录后批量清理产物。
- 一项一个检查点，局部测试通过后复核 diff。

### 第 2 批：同包职责拆分与去重

- 先拆长文件职责，再抽同包共同知识；生产逻辑和测试可以跨文件原子迁移。
- 优先 `filedb` 同步流程、KV 同类命令、CLI 解析/生命周期；高风险锁边界保持原样。
- 文件纯搬移与行为重写分开，避免一次同时改文件位置、命名、错误和算法。

### 第 3 批：受控共享机制与并发生命周期

- 在同包边界已稳定的基础上，再评估快照辅助、目录扫描或关闭收尾共享。
- 不合并 `filedb` / `ducklake` 整体状态机；不把命令分派表升级成通用框架。
- 先用 FakeFactory / Store / 事件记录器复现交错，再动锁与同步状态；跑 race 与宿主边界门禁。

### 第 4 批：性能假设逐个验证

- 按实际 profile 从 O-01 至 O-06 中选择，不必全部实施。
- 每项保留原始端到端基线，单独记录结构重构收益与运行收益。
- 可观察行为变化转待确认项，不能混入等价优化。

### 第 5 批：最终审查与计划回填

- 执行模块全量、宿主受影响范围和平台门禁，记录各自退出状态，禁止用日志过滤命令的成功替代测试成功。
- 文档回填每项“完成 / 部分 / 保留 / 取消”的理由、实际命令及未验证边界；没有运行的一律维持未验证。
- 不在计划阶段预打完成勾；后续发现已足够简单的候选可取消，不以实现数量作为验收。

### 7.1 回退纪律

- 只回退当前候选引入的改动，不整文件 restore、不 hard reset、不覆盖并发编辑。
- 拆文件后发现逻辑回归，优先撤销该逻辑变更；是否保留纯搬移取决于它是否独立有价值。
- 不通过降低覆盖阈值、删失败测试、修改 golden、静默吞错误或增加兼容分支维持“通过”。
- 存储/租约/关闭时序无法建立可信验证时，停在提案阶段，而不是先上线观察。

## 8. 待确认项、保留项与最终交付说明

### 8.1 需要单独批准的变化类别

- 根包分层规则变更或公开 API 删除、签名修改、新增全局对象/共享缓存。
- 新的重试、超时、自动恢复、并行上传、批次原子性或取消策略。
- 磁盘 JSON/key、项目 ID、错误文本与分类、CLI 输出和退出行为变化。
- 修复审查中新发现的疑似缺陷；先有复现再定义预期，不伪装成“安全删分支”。
- 任何真实数据库初始化、迁移、ALTER/DDL、运行库修复或对象删除。

### 8.2 审查中发现的可靠性疑点：独立复现，不并入精简

以下全部为源码事实加静态推演，**本轮没有运行复现，也没有修复**。验证优先级与前面的维护收益优先级不同；涉及数据完整性或生命周期的先验证。建议测试名称均为拟新增，明确列出的现有名称才表示源码已有。

#### B-01 Handle 空闲通知的指针重读竞态

- **位置**：`registry/handle.go` 第 54–79 行 `notifyIdleLocked/waitIdle`；`registry/registry.go` 第 219–223 行 Release 回调。
- **事实**：等待者注册 channel 后再次检查 active，随后 select 前重新加载并解引用 `idleDone`；Release 可关闭该 channel 并把指针置 nil。
- **危险时序**：第二次 active 检查仍大于零 → 最后一个 Release 关闭并置 nil → waiter 执行 `*h.idleDone.Load()`。存在 nil 解引用窗口；多个等待者覆盖同一槽也需检查。
- **建议**：先以可控交错测试 `TestHandle_WaitIdleReleaseRace` 复现，再设计统一通知协议。只改为等待局部 channel 未必解决多个 waiter 覆盖，不宣称一行修复完整。

#### B-02 Shutdown 与在途 Factory.Open 的所有权交错

- **位置**：`registry/registry.go` 第 165–203 行 `openAndAcquire`、第 449–459 行 `Shutdown`。
- **事实**：Shutdown 收集已有 handle 并清空 entries；尚处 opening 的 entry 不在关闭集合。Open 成功后发布 handle 前未见 closed/entry 身份复查。
- **危险时序**：FakeFactory 阻塞 → Shutdown 完成 → 放行 Open → 返回新的 lease，连接可能脱离注册表关闭管理。
- **已有测试的真实边界**：`coverage_test.go` 第 147–155 行虽名为 `TestAcquireClosedDuringOpenCleanup`，实际是先 Shutdown 再 Acquire，没有在途 Open 交错。
- **建议**：补 `TestRegistry_ShutdownDuringOpen`，断言等待者错误、连接 Close 次数及 observer 归零；修复作为单独生命周期变更。

#### B-03 Lease.Stop 可能无法取消被远端 I/O 阻塞的续约

- **位置**：`lease/lease.go` 第 330–344、363–372 行。
- **事实**：renewLoop 持 `l.mu` 执行 `putIfAbsent(ctx)`；Stop 先取得同一锁再调用 cancel。
- **危险时序**：测试 Store 的 PUT 等 ctx.Done；Stop 等锁，取消信号无法送达，可能长期阻塞。真实后端若有外部超时可能最终返回，但不能据此认定取消链正确。
- **建议**：`TestLease_StopCancelsBlockedRenewal`。将 I/O 移出锁必须设计 Stop 与续约成功记账的代次复核，避免 Stop 后重新变为持租，不做机械缩锁。

#### B-04 缓存目录数量限制使用字节目标近似

- **位置**：`cache/evict.go` 第 244–258 行 `evictOverCount`。
- **事实**：超额目录数乘 1024 作为 Evict 字节目标；字节淘汰后仍传入旧 usage。
- **反例**：超额 3 个目录、目标 3072 字节，删掉一个大目录就达字节目标，目录数仍超额；若前一次字节淘汰已删除足够目录，旧 usage 可能触发多余淘汰。
- **建议**：补大小差异明显的确定性样例 `TestEnsureCapacity_CountLimitWithUnevenSizes`、`TestEnsureCapacity_NoExtraEvictionAfterByteTarget`。引入按数量停止条件属于行为修复，需确认 best-effort 契约。

#### B-05 FileSyncer 锚点探测上限不保证找到最新版本

- **位置**：`filedb/remote.go` 第 506–545 行 `latestManifest/manifestByAnchor`。
- **事实**：最多向后读 32 个 manifest；只要无 error 就返回 best，不因达到上限或 best=nil 回退 List。
- **反例**：本地 seq=1，远端连续到 100，最多返回 33；若对应 snapshot 已被 prune，恢复可能失败。锚点和紧邻后项缺失、后方仍有版本时，也可能返回 nil。
- **建议**：补 `TestLatestManifestAnchorBeyond32`、`TestLatestManifestMissingAnchorFallsBackToList`，按“未确定达到链尾”与“确实无远端数据”区分结果。不要简单放大 32 作为证明。

#### B-06 同项目并发 Flush 可能使用同一发布标识

- **位置**：`filedb/remote.go` 第 300–320、375–378、432–439 行。
- **事实**：锁只包围读取状态，两个 Flush 可能捕获相同 last/g0/epoch，使用相同 staging 路径、snapshot key 和 manifest seq。
- **危险时序**：后台 Flush 与显式关闭前 Flush 同时开始；staging 文件互相删除/覆盖，后一个 manifest 条件写冲突又将自己标为失租。
- **建议**：`TestFileSyncerConcurrentFlushSameProject`，分阶段阻塞捕获与上传；项目级串行化须定义等待 context、关闭、重新绑定行为。已有上传中写入测试不是两个 Flush 并发测试。

#### B-07 同步中失租与关闭的发布准入窗口

- **位置**：`filedb/remote.go` 第 140–170、296–311、341–391 行；`ducklake/syncer_capture.go` 第 138–160 行；`ducklake/syncer_publish.go` 第 21–69 行。
- **事实**：FileSyncer.Flush 在入口检查 lost，没有检查 closed，上传后到 manifest 前未见再次核对；CloseNoFlush 清零代次后在途 Flush 仍可能推进水位。DuckLake 的捕获前失租检查不能覆盖捕获后与两次远端写之间的全部窗口。
- **危险时序**：上传阻塞 → 标失租/关闭 → 放行上传 → 继续发布 manifest 或写成功水位。已发请求是否撤回取决于后端，不能用一次额外布尔检查宣称解决全部竞态。
- **建议**：`TestFileSyncerCloseRejectsFlush`、`TestFileSyncerLeaseLostDuringUpload`，以及 DuckLake capture 后失租、snapshot PUT 后失租。区分“停止准入新的请求”“取消在途请求”“等待在途退出”的契约。
- **现有边界**：`TestC4_LostLeaseFlushDoesNotUpload` 覆盖先失租再调用，不证明中途失租；需记录每次 PUT，而非仅看 manifest 是否存在。

#### B-08 DuckDB CHECKPOINT 后复制窗口的一致性

- **位置**：`filedb/remote.go` 第 446–450 行；`filedb/snapshot.go` 第 33–37 行。
- **事实**：两条路径在 CHECKPOINT 返回后复制主文件，本函数范围未见覆盖整个复制窗口的写互斥。
- **待验证点**：调用方或引擎是否已保证期间主文件不会变化；不能只凭 lack of lock 就宣布已发生损坏。SHA256 校验复制字节不等于证明事务一致性。
- **已有测试**：`TestFileSyncerDuckDBSnapshotConsistencyUnderConcurrentWrites`，本轮未运行。
- **建议**：先运行隔离压力测试并在复制期间构造可重复 checkpoint/写入交错；比较重新打开后的逻辑内容，不只校验 hash。选择同步写门或引擎快照能力需要另行设计。

#### B-09 FileSyncer prune 忽略 next cursor

- **位置**：`filedb/remote.go` 第 669–693 行，尤其第 677 行 List。
- **事实**：一次只取首批 1000 项，忽略后续 cursor。
- **反例**：对象数超过一页时不能保证本轮完成清理；多轮可能逐步清理不等于单轮策略完整。
- **建议**：`TestFileSyncerPruneMultiplePages`。接入 F-04 遍历助手前确认保留策略、删除批量和边列边删语义，不能在“去重”检查点静默扩大删除范围。

#### B-10 旧计时器回调可能删掉新 timer 注册

- **位置**：`ducklake/syncer_schedule.go` 第 181–193 行。
- **事实**：Stop 不等待已经启动的回调退出；回调拿锁后无条件删除 timers[dbID]。
- **危险时序**：旧回调已启动等待锁 → 新任务覆盖 timers[dbID] → 旧回调拿锁删除新注册项，后续取消/去重失去准确句柄。
- **建议**：用回调代次/身份对照，测试旧回调与 rebind、pause/resume、新定时器交错。身份判断加固与 D-01 策略去重分开提交。

#### B-11 local-state 地址误传与并发读改写

- **位置**：`ducklake/factory.go` 第 499–502 行；第 52–62、435–436 行布局；`local_state.go` 第 30–42 行；`syncer_capture.go` 第 182–195 行与 `syncer_prune.go` 第 136–141 行。
- **已核对的地址推导**：正常远端布局 catalog 目录是 `{cache}/{id}/catalog`；AfterWrite 将此目录当 cache root 传入，状态函数又追加 `{id}/catalog/local-state.json`，形成嵌套路径，而非同一水位文件。尚未运行完整调用场景，不宣称已经复现数据回滚。
- **并发疑点**：不同更新者读取旧状态后各自保存完整对象，没有共同覆盖整个读改写的锁；rename 仅保证单次文件替换，不能防字段覆盖，固定 `.tmp` 并发写还需检查。
- **建议**：先路径和重启恢复测试，再阻塞 read/write 的可控交错测试；修正调用方的具体 state 路径、建立清晰所有权，不增加旧错误目录自动迁移或回读兼容。

#### B-12 已存在的 DuckLake snapshot 对象未核对内容

- **位置**：`ducklake/syncer_publish.go` 第 22、35–47、57 行。
- **事实**：snapshot PutIfAbsent 返回已存在视为幂等成功；之后 manifest 的 size/hash 来自本次 data，而非读取已存在对象核对。
- **反例**：同 snapshot/epoch/key 对应不同生成字节，旧对象保持不变，新 manifest 却登记本次摘要，恢复时可能校验不一致。是否能在真实引擎重试中产生不同字节需实测，不能认定同 ID 必然同字节，也不能认定必然不同。
- **建议**：用 FakeStore 预置同 key 不同内容的确定性用例，再补真实捕获重试。校验失败应 fail closed，但增加 HEAD/GET 与改变冲突返回必须作为可靠性变更记录。

上述问题优先形成独立复现与决策单；没有复现结果或用户批准，不把它们标为已确认生产故障、已完成修复或等价优化收益。

### 8.3 明确保留

保留公共 API、nil 接口适配、系统库保护、双重写门、安全和路径校验、锁与事务边界、错误脱敏、内存 Store 拷贝语义、现有独立模块边界。保留尚未证明同义的小重复；不安装 deadcode/staticcheck 工具、不为静态告警升级依赖。

`internal-simplify-refactor-plan.md` 已将宿主敏感 SQL 词法器与 `godatabase/sqlguard` 的共享列为需核对语义的长期项；本计划不绕过该结论，也不将它直接升级为必做项。

### 8.4 本轮交付与证据级别

交付仅为本计划文件。源码和测试未改动，数据库未操作。候选基于源代码与直接测试阅读，属于“已取证、未运行验证”；性能收益均未测量。未宣称整个模块已经优化、已通过构建/测试，或不存在其他缺陷。

## 9. 完整并行审查回传后的补充与修订

> 本节补入首版落盘后收到的完整结果，日期仍为 2026-11-24。重复发现归入原编号，不重复计算。核心新增风险的源码已经主审复核；其余候选注明来自专项审查。全部未运行验证、未实施。涉及宿主现有工作区改动的发现只登记，不归因到 HEAD 或此次文档任务。

### 9.1 结论修订及优先级调整

1. **最高优先核实缓存真实布局**：新增 B-13，涉及合法八位项目目录未被识别而使整个 `projects/` 被视为空壳候选。它先于 R-02/O-02 的遍历优化；禁止拿真实缓存运行淘汰复现。
2. **宿主装配单列**：新增 B-14，`WriteGate` 可能只写入 Options 值副本。需要扩展到宿主的单独修复授权，不是 `godatabase` 内部去重。
3. **写门事实已纠正**：Q-01 与 §4.3 已明确 `isMutatingSQL` 复用 `sqlguard`，不再暗示存在两套 DuckLake 词法器。
4. **F-01 首批范围收窄**：优先只共享 DuckDB CHECKPOINT→copy/hash 核心。备份与远端的 SQLite 错误前缀、目录权限（0700/0755）、覆盖规则与完整校验不同；只有这些策略能在入口原样保留时再考虑 SQLite 部分。
5. **R-03 推荐暂不做**：完整并发审查认为 112 行 singleflight 的两入口差异已经清晰，登记 helper 收益不足。保留提案历史，不进入默认实施批次。
6. **B-08 保持历史未复现定位**：`plan/planv6.7/legacy-plan-consolidation-plan.md` 第 543–545 行已有 DuckDB 裸复制 R3 未复现记录。保留压力回归，不据静态推理新增全局写锁。
7. **O-01 区分两种上传契约**：`PutFiler` 可选分支属于 `filedb`；DuckLake snapshot 使用 `PutIfAbsent`。无条件 `PutFile` 绝不能替代不可变对象条件写；流式条件写需另议公共 API。

### 9.2 补充的等价精简与结构候选

#### F-05 删除只赋值、未读取的私有字段

- **专项证据**：`filedb/factory.go` 第 190–192、308–320 行 `trackedConnector.db`；`filedb/remote.go` 第 72–108 行 `FileSyncer.cacheDir`。
- **方案**：删字段及赋值，保留构造参数 cacheDir（仍用于 stateRoot）、`Factory.CacheDir` 及全部公开入口。实施前重新查同包测试、结构体字面量和间接引用。
- **收益**：减少虚假的资源所有权和重复配置状态，不宣称显著内存收益。
- **验证**：已有 `TestProjectFileRoundTrip`、远端冷启动/恢复测试，加独立模块与宿主调用方编译。
- **分类**：P3 / 低风险 / 可进入首批，与 R-04 分开小检查点。

#### F-06 两处复用已有 WriteEnsureDir

- **专项证据**：`blob/blob.go` 第 122–135 行 `Memory.DownloadFile`；`filedb/remote.go` 第 657–667 行 `writeLocalState`；`atomicfile/atomicfile.go` 第 24–29 行。
- **方案**：用现有 helper 替换同为 mkdir0755→Write0644 的组合；不新增工具。
- **保护**：仍先读取对象或 Marshal 再创建目录；缺对象不能留下新目录；原错误、权限、临时文件名与 rename 顺序不变。不能替换带流式哈希/fsync 的 `copyFileHashed`。
- **验证**：`TestMemoryRoundTrip`、远端恢复测试；补父路径为文件、rename 失败、对象缺失无建目录副作用。
- **分类**：P2 / 低风险 / 首批候选。

#### F-07 Memory 的数据与时间元数据单一所有者

- **专项证据**：`blob/blob.go` 第 61–119、137–172 行，两个 map 伴生维护 bytes 与 modified。
- **方案**：可改私有 entry 单 map，同一 RWMutex 范围不变，不新增公开类型。
- **保护**：Put/Get 继续复制 bytes；nil/empty 行为不变；冲突不更新时间；仅 PutIfAbsent 返回 ETag，Head/GetBytes 当前空 ETag 不顺手改变；DeleteMany 缺失/重复 key 静默。
- **验证**：`TestMemoryRoundTrip`、`TestMemoryPutIfAbsent`、`TestMemoryListAndDeleteMany`，补输入/输出别名修改、时间、ETag 和 race；1/1000/100000 keys 的性能仅为可选测量。
- **分类**：P2 / 中风险 / 结构收益优先，无实测不宣称加速。

#### K-04 Hash.Set 合并两份相同 INSERT

- **专项证据**：`kv/hash.go` 第 107–127 行。
- **方案**：存在 key 时先 UPDATE，受影响非零则 continue；剩余路径统一 INSERT。保留 GetMany 的新增数量计算、map 遍历顺序、RowsAffected 错误忽略政策及最后元数据步骤。
- **禁止**：改 UPSERT/批量 SQL/提前插元数据；现有 schema 没有可供随意改用 ON CONFLICT 的唯一约束。
- **验证**：`TestHashSetGetItems`；补同值更新、新旧字段混合、TTL/version、UPDATE/INSERT 错误与回滚。
- **分类**：P2 / 低至中风险 / 比 K-03 更适合先做。

#### K-05 只共享整数解析与溢出纯规则

- **专项证据**：`kv/string.go` 第 178–193 行与 `kv/hash.go` 第 208–228 行。
- **方案**：提取 TrimSpace→十进制 ParseInt→int64 溢出检查的私有纯函数，仍返回同一 `ErrValueType`；不合并数据库流程。
- **反例/保护**：`" +001 "` 接受、`"1e2"` 拒绝；Max+1/Min-1 拒绝，0+Min 允许。String 写失败返回 next+err，Hash 写失败返回 0+err；String Len=nil 不能经 touchAfterWrite 变0，浮点和 ZSet 规则不合并。
- **验证**：`TestStringIncr`、`TestHashIncr`；补两实现共享极值表、溢出无写、TTL/version 不漂移。
- **分类**：P2 / 低风险 / 首批可选。

#### K-01 扩展证据：Hash/Set/ZSet 删除三类同构

原 K-01 的收尾抽取可先实施；专项审查补齐 `kv/zset.go` 第 71–103 行与 Hash 第 141–173、Set 第 67–99 行完整同构证据。后续可用包私有固定类型/表列描述共享完整删除规则，不能接受用户表名或扩展为 CRUD 框架。两层抽取若只增加转发，应保留一个清晰层次。

新增验收：三类型两引擎矩阵，空参数优先 ErrArgument、重复成员按实际 RowsAffected、过期返回0但不物理删除、零命中 version 不变、最后元素删除 key、错误前缀与 TTL 采样顺序不变。不得以 `guardWrite` 替代 `getMetaOf`。已有 `TestZSetIncrAndDelete`、`TestCrossTypeGuard` 补入 K-01 的测试索引。

#### D-05 驱动写门执行骨架：fixture 先行

- **专项证据**：`ducklake/gated_conn.go` 第 91–131、189–280 行的六个执行入口。
- **方案**：先以 fake driver 覆盖结果/rows 两族，再评估私有共享执行函数；泛型/闭包若增加理解和分配成本就保留原样。
- **冻结**：ErrSkip 先返回；纯读不抢锁、不重试；legacy Stmt 的 Background context；txHeld/context 标记免重入；非 nil result/rows 与 error 同时返回原样；释放时机仍是执行返回而不是 Rows.Close。
- **验证**：`TestWriteGateSerializesFlushAndWrites`、`TestRetryOnConflict`、`TestIsMutatingSQL`；补六入口、prepared/legacy、Begin/Commit/Rollback 失败和调用次数。
- **分类**：P2 / 高风险 / 不列自动执行。

#### D-06 锚点失效回退规则只有一份实现

- **专项证据**：`ducklake/local_catalog.go` 第 48–53 行与 `syncer_publish.go` 第 88–91 行。
- **方案**：私有 helper 只共享 ChainBroken 且 anchor>0 时从0重探的规则；保留公开 ReadLatestManifest 缺锚失败，调用方锚点来源与内存 seq 命中零 GET 快路径。
- **反例**：普通 GET/解码错误不能变成回退；公开 API 不能自动重探；不因此声称解决异常稀疏链问题（B-23）。
- **验证**：`TestReadLatestManifestLongChain`、`TestReadLatestManifestBrokenChainFailsClosed`、`TestEnsureLocalCatalogLocalAheadNotOverwritten`；补两调用方一致请求序列和普通错误不重探。
- **分类**：P2 / 中风险 / 先特征测试。

#### R-05 并发表征测试事件化，作为生命周期改动前置

专项审查发现：`registry/registry_test.go` 第 81–98 行的 waiter 同错测试实际只测串行失败重试；`coverage_test.go` 第 147–156 行并未覆盖 Open 中 Shutdown；`lease/gate_test.go` 第 93–114 行 started 在 fn 之前发送，不证明两个 fn 已并行；`singleflight` 部分测试靠 sleep 假定等待者注册；`cache/more_test.go` 部分错误分支只忽略结果或 Logf。

保留原有效断言、校正测试名称，再通过 entered/release/done、阻塞 Factory/Store 确定时序，timeout 仅防挂死。不新增测试公共 API 或第三方框架；不把新增修复预期冒充旧行为表征。P1 / 中风险，前置于 R-01/R-02、D-02 和租约缩锁。

#### R-06 缓存及租约按职责分层，不先统一规则

- 缓存 `cache/evict.go` 第 87–217 行可分候选收集/排序/执行；Usage 与 Evict 当前不是同一目录口径，先修复获批的 B-13 后再谈共享。未知目录不等于空目录，nil Registry 的注释与现行为也不能按注释直接改默认。
- 租约 `lease/lease.go` 可将 payload/storeProxy/probeMax 与 Manager/Lease 生命周期同包分离。ValidFor 持 manager 锁等待单租约锁、renew 持租约锁做 PUT 存在跨项目队头阻塞假设；缩锁需处理旧 lease 替换、Stop 不复活与 onLost 锁外执行，另作性能/可靠性试验。
- Registry 的获取发布/回收关停也可同包拆文件，但不同回收入口不得压成 flags 大函数；旧关闭与新打开身份问题见 B-15。
- 分类：P2 / 高风险；拆文件不算速度收益。冻结 JSON/key/epoch/TTL+Grace 观察窗、首错与 no-flush 语义。

### 9.3 新增独立可靠性疑点及复现计划

本节 B-13、B-14、B-16、B-17 经主审读取关键实现复核；其余依据完整专项审查回传，实施前仍需重新定位与独立复现。全部未测试、未修复，不作为等价候选收益。

#### B-13 合法短项目 ID 可能导致整棵 projects 被误当空壳淘汰

- **主审复核**：`cache/evict.go` 第 41–61 行只识别 UUID 或名为 projects 的目录；第 139–159 行再次用同一函数遍历 projects 子目录，识别结果为空就把整个 projects 加入候选，ID为空；第 178–192 行关闭该 ID 后 RemoveAll。
- **真实布局**：`spec.go` 第 83–96 行、`filedb/path.go` 第 15–26 行是 `{cache}/projects/{8位ID}/database.ext`，合法 ID 不应改为 UUID。
- **危险推演**：目录里有活跃 `abcd1234`，却未识别任何子库；候选变成整个 projects，跳过真正项目的 IsActive，可能删除多个库及同步状态。不是已发生数据丢失的证据，但应最高优先验证。
- **安全复现**：仅 `t.TempDir` 建合法短ID与未知目录，fake registry 标活跃、记录收到的 ID；断言根目录绝不成为空壳候选，活跃库不删。再测项目数量口径，Usage 当前按一级目录计数不等于真实项目数。
- **独立设计**：CloseDatabase 返回后到 RemoveAll 前还可能发生新 Acquire；删除前重查一次 IsActive 不是原子保证，需要同 key 关闭/删除排他或占位。不得直接对用户缓存运行 Evict。

#### B-14 宿主 WriteGate 赋给 Options 值副本

- **主审复核**：`internal/app/app_lease.go` 第 47 行按值接收 regOpts，第 79 行赋 WriteGate；`internal/app/assemble_storage.go` 第 281–288 行仍把原 regOpts 传 registry.New。
- **影响范围**：条件为 writable、远端数据库、租约启用且条件写探针通过；代码配置了租约 manager 并不证明 registry 得到写门。
- **复现**：补装配级 fake 测试，验证真实构造的 Registry 读写 Acquire 会触发 Gate，不只直接测 `newLeaseWriteGate` 闭包。
- **权限边界**：宿主在原请求外，且已有工作区改动；只记录，不修改。修复需按 `internal/AGENTS.md` 与宿主计划另批，不能偷偷夹进库重构。

#### B-15 旧关闭/解绑可能清掉重新绑定的同 ID 资源

- **专项证据**：Registry 摘除 entry 后锁外关闭期间可重开；`ducklake/syncer_schedule.go` 第 122–139 行 Unbind 先 Flush 再按 ID 删除状态，而第 102–108 行允许同 ID 新 Bind；`Factory.BeforeClose` 第 512–518 行没有使用 sqlDB 身份。
- **复现**：阻塞旧 BeforeClose/Flush，绑定新连接，再放行旧 Unbind，验证新 binding/timer/dirty 保留。和 B-02 一起设计 opening/closing 代际，不通过全局锁包网络 I/O 解决。

#### B-16 KV Update panic 缺少事务回滚收尾

- **主审复核**：`kv/executor.go` 第 166–176 行 BeginTx 后仅对 fn 返回 error 显式 Rollback，无 defer；View 第 200–205 行已有 defer，但两者不能因此合成模板。
- **复现**：一次性数据库，fn panic 由测试外层 recover，Background context 下验证连接归还、未提交写入回滚及后续 Update 可前进。panic 传播保持原状，可靠性修复只增加必要清理，不改成吞 panic 返回 error。

#### B-17 KV TTL 的 victim 集合随时间/查询重新计算而漂移

- **主审复核**：`kv/ttl.go` 第 16–18 行扫描一次时间与 LIMIT；第 55–64 行各 DELETE 再次取时间、重算无 ORDER BY 的 LIMIT 子查询，仅依据首次扫描选择数据表。
- **反例**：首次只有 string 过期，最后 hash 也跨过 TTL；最终元数据删除可能包含 hash，却没有执行 hash 数据表清理。大于上限时，无序 LIMIT 的集合也不能假设固定。
- **复现**：注入逐步时钟与两类数据，验证同一确定 victim 集合的数据行/元数据一起删除、最大1000个及正确计数。固定 victim 是独立行为修复，不只是抽 SQL 字符串或全局冻结所有 KV 时钟。

#### B-18 GetMany 重复输入只填首次位置

- **专项证据**：`kv/string.go` 第 140–155 行、`kv/hash.go` 第 58–73 行 name→单索引只保存首次；宿主 MGET/HMGET 使用返回 values。
- **反例**：已存 a=v，输入 a,a 推演为 `[v,nil]` / `[true,false]`。现有 `TestStringGetManySetMany` 未覆盖重复。
- **复现/决策**：补重复+缺失+乱序及二进制测试；是否改为 name→多索引/按输入重建须独立授权，并明确重复返回 bytes 的别名所有权。不在 K-03 中顺手改变。

#### B-19 Set.Diff 去重改变首集合也出现在减数中的结果

- **专项证据**：`kv/set.go` 第 264–288 行，全体 key 去重并移除 rest 中的 first。
- **反例**：Diff(a,a) 应核对是否当前返回 a 而不是空，DiffStore 同样影响；现有 `TestSetAlgebra` 不覆盖同 key。
- **复现**：同 key、重复减数、缺失、空集合、目标与输入同 key；修复独立，不统一全包 dedupe 策略。

#### B-20 EXPLAIN 注释插入可能绕过只读纵深检查

- **专项证据**：`sqlguard/classifier.go` 第 163–189 行在非标识符处返回 nil，EXPLAIN 与选项间的注释可能跳过 ANALYZE 拒绝策略。
- **验证**：增加含注释的 EXPLAIN/ANALYZE 只读判定回归，同时冻结无注释及普通 EXPLAIN 的行为；仅做本地解析测试，不对真实服务尝试绕过。修复另立安全行为变更，不伪装 skip helper 去重。

#### B-21 CLI 脱敏、参数顺序与空库读取契约缺口

- **专项证据**：`cmd/godb-cli.go` 第 71–97 行空 GODB_DATA_ROOT 用作 ReplaceAll 模式可在字符间插入标记，cache root 未纳入；第 288–306 行标准 flag.Parse 在首位置参数停止，与 usage 的后置 value/ttl 不一致；第 316–331、382–396 行只读 KV 未先 HasSchema。
- **测试计划**：分别补 sanitize 空/默认/绝对/cache 路径 golden、真实 argv 前置/后置 flag 对照、新 SQL-only 库首次 KV 读。不能用只读路径 EnsureSchema 解决缺表。
- **相关边界**：已有 CLI 测试绕过真实命令函数；`TestSQLReadOnlyRejectsCTEDelete` 的底层 Execute 失败也不证明 SQL 守卫生效。CLI 6条命令的 stdout/stderr/exit 合同需重新建立。
- **授权**：选择文档约定或支持交错 flags、补脱敏、空库结果政策分别确认，不作为纯拆文件附带修复。

#### B-22 CLI bench 的退出与参数校验

- **专项证据**：`cmd/godb-cli.go` 第 566–573 行 helper 内 os.Exit 绕过 cmdBench cleanup；第 530 行及 benchLoop 的负 ops 可能造成 make panic。
- **复现**：一次性子进程/可控执行入口，验证负值、运行中故障、退出码和清理事件；不直接在当前服务运行 benchmark。将 Exit 移到顶层会改变 defer 与错误输出，单独设计。

#### B-23 DuckLake 有限点探测未必能证明远端没有 manifest

- **专项证据**：`ducklake/manifest.go` 第 138–151 行缺 seq1 后只探2/4/8等；第 95–133 行以连续链存在性进行定位。
- **异常状态反例**：只残留 seq3，或 seq1/3 存在而2缺失，有限探测可能推断为空或返回旧 seq。现有断链测试删1但保留2..5，不能覆盖这些形态。
- **复现/决策**：内存 Store 布置稀疏残留、链中缺口和有效锚点；明确部署磁盘契约允许的外部破坏边界，需 List/可信锚时单独审批。D-06 纯回退提取不宣称修复它。

#### B-24 instance-format 远端清洁检查忽略后续分页

- **专项证据**：`ducklake/instance_format.go` 第 223–237 行只 List 前100项并忽略 next。
- **反例**：第一页均为跳过的 `_system/`，第二页才有项目数据，可误判 clean。
- **复现**：fake 两页、空页但有next、第二页错误；只有所有页面确认干净才能写格式标记。增加遍历会改变是否准许初始化，是独立 fail-closed 修复，不在当前任务执行初始化。

#### B-25 租约发布与 StopAll 的进一步生命周期交错

- **专项证据**：`lease/lease.go` 第 269–276 行先放入 leases 再设置 cancel；StopAll 可在间隙观察 cancel=nil。StopAll 后 Acquire 未显式拒绝时，可能返回 held=true 但续约马上因 stopCh 已关闭退出。
- **复现**：发布屏障、阻塞 Store、StopAll/Acquire 前后排列；与 B-03 合并设计关闭准入与代次，不把锁全部删掉。ValidFor 缩锁也需同一契约。

#### B-26 DuckLake 路径脱敏需要独立安全验收

- **专项证据**：`ducklake/factory.go` 第 91、399–400 行错误含目录；第 472–482 行摘要只特判 SECRET，部分 ATTACH/SET 临时目录可能进入摘要；`instance_format.go` 第 89–90、251、308–331 行错误路径亦需审查。
- **计划**：用 fake logger/失败注入检查双开、引擎不匹配、boot SQL、格式标记错误，断言物理路径/DSN/密钥不进入日志或对外错误。改错误文本是明确安全行为变化，不作为等价重构直接实施，也不输出真实敏感路径作示例。

### 9.4 性能矩阵与验证门禁补充

- DuckLake capture 写锁当前覆盖 staging `ReadFile`；只有证明 COPY/CHECKPOINT/DETACH/前后 snapshot 复核后 staging 不再变化，才试验将文件读取移到锁外。另独立试验下载后流式 hash+计数，保持 checksum 先于删WAL/rename和错误优先级。计划1/16/128MiB、并发写/sync、gate时长、写p95和峰值堆；未测量。
- 现有 dense/inlined 并非所有负载逐一对照，users 缺 inlined 对照；LocalSyncer fixture 不模拟远端同步 flush。保留现有 benchmark 名称，补同数据量与显式 flush 后的对照；只 seed logs 后附测空 metrics 的数字不能当 metrics 小文件曲线。
- Set/ZSet 多成员添加有 N+1 假设，但逐项失败可能留下事务内部分效果；批量一条 SQL 的原子失败不同，不能只因多数调用方最终 rollback 就宣称等价。空/nil BLOB、重复 score、参数上限和调用者捕获错误后继续提交都需表征。
- 若正式纳入宿主 B-14，补 `go test ./internal/app` 的装配级回归；涉及 F-01 快照核心补 `go test ./internal/backup`，而非仅 filedb 自测。以上命令本轮未运行。
- 验证顺序更新：优先隔离复现 B-13/B-14、B-01/B-02/B-03/B-11 和 KV 数据一致性问题；批准的修复与纯重构分批，之后再测目录扫描/缩锁/批量化。
- 不回收 `dbMutexes/connSchemaStates` 全局记录来凑内存优化：旧 Store 仍持 writeMu 时清表再 New 可能形成同 DB 多把写锁，必须先明确连接所有权。

### 9.5 补充交付状态

完整回传已去重整合，四个审查任务均已停止并确认关闭。只修改本计划文档；没有修改 `godatabase` 或宿主实现，没有运行测试、构建、基准、服务、迁移或 DDL。首版的源码/格式检查结果仍仅对应其声明范围；新增可靠性问题未复现，不据此宣称生产故障已确认。
