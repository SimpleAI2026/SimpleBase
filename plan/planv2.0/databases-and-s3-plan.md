# 数据库管理（DuckLake-only）与 S3 对象存储计划

> **状态**：核心已落地；DeleteDatabaseHandler 已移除；不做 MinIO e2e  
> **日期**：2026-09-16  
> **决策**：Turso 丢弃 · DevMode 用户文件本地盘 · 软删同步清 S3  
> **范围**：Databases / SqlConsole / DataManager / S3Manager 对应的后端能力收敛与补齐  
> **契约权威**：[`proto-http.md`](./proto-http.md) §3.1–§3.4、[`proto.http`](./proto.http)  
> **引擎设计权威**：[`db-ducklake-plan.md`](./db-ducklake-plan.md)（本文件不重复 DuckLake 内部细节，只定义产品边界、退役清单与验收）  
> **前端现状**：[`ui-plan-v2.md`](./ui-plan-v2.md)（Databases / SqlConsole 已接 API；S3Manager 已接，体验项待补）

---

## 0. 一句话目标

**用户库只走 DuckLake；用户对象存储只走 AWS S3 标准协议（aws-sdk-go-v2）。**  
历史 Turso / local-SQLite 用户库链路从配置、装配、descriptor、文档中移除；前端契约（§3.1–§3.4）不变，后端实现对齐契约并去掉双轨。

---

## 1. 背景与现状

### 1.1 前端已依赖的能力（不可破坏）

| 页面 | 契约 | 前端状态 | 后端状态（摘要） |
|---|---|---|---|
| Databases | §3.1 创建/列表/详情/open/close/删除；backups/restore=501 | 已接 | Handler 已存在；水位/`snapshot`、软删异步清理需按 DuckLake 语义收口 |
| SqlConsole | §3.2 query / execute / batch | 已接 | 已通；需保证**仅** DuckLake 路径，`durability` 语义正确 |
| DataManager | §3.3 collections/documents（隐式第一库） | 已接 | 已通；多库歧义见 §6.2 / 前端 F9，本计划不改路径 |
| S3Manager | §3.4 list / upload / delete / presign | 已接 | `FileStore` + aws-sdk-v2 已有；DevMode 落本地盘；生产需标准 S3 协议可测 |

### 1.2 后端双轨问题（必须收敛）

用户库引擎仍残留：

| 路径 | 位置 | 问题 |
|---|---|---|
| `engine=ducklake`（默认） | `internal/database/ducklake` | 目标唯一路径；Phase 1/2 已部分落地 |
| `engine=turso` | `internal/database/turso*`、`tursofactory.go` | 历史 Serverless SQLite；descriptor 仍带 `turso_storage` |
| `engine=local` | `localfactory.go` | 裸 sqlite3 文件库，与 DuckLake 语义分叉 |
| memory factory | `memoryfactory.go` | 测试/遗留；不应再作为产品用户库选项 |

配置层仍接受 `database.engine ∈ {ducklake, local, turso}`（`internal/config`），与「只用 DuckLake」冲突。

### 1.3 两条 S3 语义（必须分清，禁止混用）

| 平面 | 用途 | 实现 | 前端感知 |
|---|---|---|---|
| **A. 用户对象存储** | S3Manager：用户上传文件 | `objectstore.FileStore`（`NewS3FileStore` / DevMode `NewLocalFileStore`） | §3.4 `/s3/objects`、`/s3/presign` |
| **B. DuckLake 数据面** | 库的 Parquet / catalog 同步 | DuckLake `DATA_PATH` + httpfs + `CatalogSyncer` + `objectstore.BlobStore`/`Client` | 仅间接：`durability`、`snapshot` 水位、degraded |

本计划要求：

- **平面 A**：对外行为严格按 **AWS S3 API**（ListObjectsV2 / PutObject / DeleteObject / HeadObject / PresignGetObject）；endpoint 可指向 AWS、MinIO、或兼容 COS（path-style），但客户端只用 aws-sdk-go-v2，不引入第二套 SDK。
- **平面 B**：同样经 S3 协议写对象；**不得**再依赖 Turso 管理 data/ 前缀。descriptor 从 `turso_storage` 迁到 DuckLake 语义字段（见 §4.3）。

---

## 2. 需求（硬性）

### 2.1 数据库 = DuckLake only

1. **唯一用户库引擎**：运行时只装配 DuckLake Factory；配置删除或拒绝 `local` / `turso`。
2. **生命周期 API** 与 §3.1 一致：create → open/close → query/execute/batch → soft-delete；`status` 九态语义与前端 tag 对齐。
3. **SQL API** 与 §3.2 一致：参数化 `?`、sqlguard、无 `last_insert_id`、`durability ∈ {committed_local, synced_s3}`。
4. **文档 API** 继续落在 DuckLake 表模型（§3.3）；本轮不改「隐式第一库」路径（另立前端提示 / 后续 §6.2）。
5. **退役**：删除或剪除 Turso/local 用户库代码、测试双轨、文档与迁移入口中的「可选引擎」表述（迁移工具若保留，仅为「外部数据导入 DuckLake」，不再保留双写运行时）。

### 2.2 S3 对象存储 = AWS S3 标准协议

1. S3Manager 读写经 `FileStore` → aws-sdk-go-v2，操作语义对齐 S3：
   - List：`ListObjectsV2`（prefix、MaxKeys≤1000）
   - Upload：`PutObject`（multipart 请求体由 HTTP 层解析后写入）
   - Delete：`DeleteObject`
   - Presign：`PresignGetObject`，TTL=15m（契约固定）
2. **项目隔离**：物理 key = `{projectID}/{userKey}`；响应剥前缀（已有，保持）。
3. **兼容性目标**：官方 AWS S3；本地/CI 用 **MinIO**（S3 兼容）做 e2e；腾讯云 COS 等仅作为「S3 兼容 endpoint + path-style」配置验证，不写厂商专用 SDK。
4. DevMode：可继续 `LocalFileStore`（磁盘模拟），但接口与错误码与 S3 实现一致；生产 / 非 DevMode **禁止**静默落到非 S3 实现。

### 2.3 明确不做（本计划边界）

- 不实现 backups/restore（保持 §3.1 的 501），除非另开计划对齐 DuckLake `COPY FROM DATABASE`。
- 不新增 FaaS / WebSocket Logs / metrics summary（见 proto §3.8）。
- 不改 §3.9 设置页后端。
- 不在本计划引入第二数据库引擎「过渡双跑」——允许短迁移窗口，但默认分支与新部署只开 DuckLake。

---

## 3. 目标架构（产品视角）

```
┌──────────── UI (Vite) ────────────┐
│ Databases / SqlConsole / Data / S3 │
└───────────────┬───────────────────┘
                │ /v1/projects/:p/...
┌───────────────▼───────────────────┐
│ API: database / sql / data / s3   │  ← 契约冻结：proto-http §3.1–3.4
└───┬─────────────┬───────────┬─────┘
    │             │           │
    ▼             ▼           ▼
 Registry     DuckLake     FileStore
 (租约/池)    Factory      (平面 A)
    │             │           │
    │             ▼           ▼
    │      DATA_PATH +     aws-sdk-go-v2
    │      CatalogSyncer   S3 API
    │      (平面 B)           │
    └─────────────┴───────────┘
                  │
            同一套 S3 凭据/bucket
            （可不同 prefix）
```

### 3.1 前缀约定（建议固化到 keys 文档）

| 用途 | 建议物理前缀 | 说明 |
|---|---|---|
| 用户文件（平面 A） | `{env}/files/{projectID}/...` | 现有 FileStore 行为；API 只暴露相对 user key |
| DuckLake catalog 版本 | `{env}/ducklake/{dbID}/catalog/...` | CatalogSyncer（见 db-ducklake-plan） |
| DuckLake DATA_PATH | `{env}/ducklake/{dbID}/data/` | Parquet / 内联 flush 后文件 |
| 库 descriptor | `{env}/databases/{dbID}/descriptor.json` | **去掉 turso_storage**，改为 ducklake 字段 |

具体 key 字符串以 `objectstore/keys*.go` 现网实现为准，本计划要求在退役阶段做一次「Turso 字段 → DuckLake 字段」迁移说明（见 §4.3）。

---

## 4. 工作包拆解

### 4.1 WP-DB-1：引擎收敛（配置 + 装配）

**目标**：进程内不可能再打开 Turso/local 用户库。

- `config`：`database.engine` 仅允许 `ducklake`（或缺省即 ducklake）；非法值 Validate 失败。
- `app.New`：删除 `EngineLocal` / `EngineTurso` 分支；DevMode 与生产均 `DuckLakeFactory`（DevMode DATA_PATH 本地盘，已有）。
- 环境变量 / `.env.example` / `config.example.yaml`：去掉 turso/local 选项说明。
- **验收**：`SIMPLEBASE_DB_ENGINE=turso` 启动失败；默认 DevMode 下 Databases+SQL+Data 全绿。

### 4.2 WP-DB-2：代码退役清单

删除或移出主构建（可先 `go` build tag 隔离，最终删目录）：

| 退役项 | 说明 |
|---|---|
| `internal/database/turso/` | 整包 |
| `internal/database/tursofactory.go` | |
| `internal/database/localfactory.go` | DevMode 不再需要裸 sqlite 用户库 |
| `internal/database/memoryfactory.go` | 若仅服务旧测试，改为 DuckLake 测试工厂或 table-driven fake |
| go.mod 中 libsql/turso 依赖 | 以 `go mod tidy` 后无引用为准 |
| jobs/备份里的 `TursoVersion` 等字段 | 改为 DuckLake snapshot / 删除未用任务 |
| 文档：`docs/migration-guide.md`、README 中「多引擎」表述 | 改为「仅 DuckLake」+ 外部导入说明 |

**不退役**：

- Catalog 元数据库仍可用 SQLite（平台库，不是用户 DuckLake）；与「用户库 DuckLake-only」不冲突。
- `LocalFileStore`：仅 DevMode 平面 A；或测试 fake。

### 4.3 WP-DB-3：Descriptor / 元数据去 Turso 化

- `objectstore.DatabaseDescriptor`：`turso_storage` → 例如 `ducklake_storage { bucket, region, endpoint?, data_prefix, catalog_prefix }`（字段名最终实现时定，计划要求**语义**对齐 DuckLake，而不是保留 Turso 命名）。
- 冷启动 / 打开库：校验 DATA_PATH 与 descriptor 一致（沿用 db-ducklake-plan Phase 2）。
- 旧 descriptor 读入：若仍含 `turso_storage`，**拒绝打开**并给出明确错误（或一次性迁移工具改写）；默认不做静默双读长期兼容。

### 4.4 WP-DB-4：API 行为与契约对齐（Databases + SQL）

对照 `proto-http.md` 逐条核对（实现阶段用测试锁定）：

| 项 | 要求 |
|---|---|
| 创建后 `status` | `creating` → 就绪后 `ready`（或经 `opening`）；前端已覆盖九态 |
| `GET :id` 的 `snapshot` | 有 Syncer 时注入 `last_synced_snapshot` / `sync_lag`；列表无此字段 |
| `close` | 204 无 body；释放租约，不删 S3 |
| `delete` | 202 + `deleting`；异步清平面 B 前缀；完成后 `deleted` |
| `execute.durability` | 本地 commit 且未同步 → `committed_local`；Syncer 确认 → `synced_s3` |
| backups/restore | 保持 501 `not_implemented`（可无 request_id，前端已容忍） |
| sqlguard | DuckDB 方言；拒绝 ATTACH/CALL/读库外路径等（见 ducklake plan §4.5） |

### 4.5 WP-S3-1：用户对象存储（平面 A）协议与可测性

- 生产路径：仅 `NewS3FileStore` + aws-sdk-go-v2；`ForcePathStyle`、自定义 `Endpoint` 继续由 config 注入。
- **MinIO e2e**（必做）：docker 起 MinIO → 配置 endpoint/path-style/key → §3.4 四接口 + 前端 S3Manager 手测清单。
- Presign：必须返回可直接 GET 的 URL；MinIO/AWS 均验证；DevMode LocalFileStore 的 presign 若无法真开，需在 API 层明确行为（保持现有或返回 501 并写进契约——**实现前在 proto-http 补一句**，避免前端静默坏链）。
- 错误映射：S3 NoSuchKey → 业务 not_found / ok 幂等删除；超时与 5xx 不泄漏凭据。

### 4.6 WP-S3-2：DuckLake 数据面（平面 B）与平面 A 隔离

- 用户文件 prefix **不得**与 ducklake data/catalog prefix 重叠。
- 删除数据库异步任务只删平面 B（+ descriptor），**不**删 `{projectID}/` 下用户文件（除非产品另定义「删项目」）。
- S3 不可用时：平面 A 接口失败可见；平面 B Syncer 进入 degraded + 指标，不把写成功谎报为 `synced_s3`。

### 4.7 WP-FE-ALIGN：与前端的协作项（本计划跟踪，代码可前后端分 PR）

| 项 | 负责 | 说明 |
|---|---|---|
| Databases / SqlConsole 真联调 | 双方 | `./build.sh dev` + Dev Key |
| S3 上传进度 / 体积预校验 | 前端（ui-plan F7） | 后端已有 413 |
| DataManager「第一库」提示 | 前端（F9） | 后端本轮不改路径 |
| proto-http DevMode presign 行为 | 文档 | 若 LocalFileStore 不能真预签名，写清 |
| 本计划链接进 ui-plan「配套文档」 | 文档 | 见文末 |

---

## 5. 分阶段实施（建议）

> 与 `db-ducklake-plan.md` Phase 编号对照：本计划的 Phase D/S 是**产品交付切片**；引擎内部细节仍以 ducklake 计划为准。

### Phase D0 — 基线冻结（0.5d）

- 用 `proto.http` + DevMode 跑通 §3.1–§3.4 现状基线，记录已知 501/缺口。
- 列出仓库内所有 `turso` / `EngineLocal` / `TursoStorage` 引用（rg 清单写入本计划附录或 PR 描述）。

### Phase D1 — DuckLake-only 收敛（先于大删）

- WP-DB-1：配置与装配只留 DuckLake。
- 测试：原 turso/local 单测删除或改写为 DuckLake；`go test` 相关包绿。
- **门禁**：默认 `./build.sh dev` 下 Databases 创建 → SqlConsole 建表插入查询 → DataManager CRUD。

### Phase D2 — 退役与 descriptor

- WP-DB-2 + WP-DB-3：删包、改 descriptor、更新文档与 example env。
- 迁移说明（已拍板）：**不支持** Turso 原地升级或官方迁移工具；存量丢弃。descriptor 仍含 `turso_storage` 的库一律拒绝打开并返回明确错误。

### Phase D3 — 契约加固（API）

- WP-DB-4：handler/registry 测试锁定 status、snapshot、durability、204/202/501。
- 软删：**同步**清理平面 B（data/catalog 前缀 + descriptor），成功后标 `deleted`；不引入 jobs 依赖完成本轮。

### Phase S1 — 用户 S3 协议验收

- WP-S3-1：MinIO e2e + 可选 AWS/COS endpoint 冒烟。
- 确认 S3Manager 四操作与 key 校验、项目隔离。

### Phase S2 — 双平面隔离与故障语义

- WP-S3-2：prefix 文档化；故障注入（停 MinIO）验证 degraded / 错误码。
- 与 ducklake Phase 2 验收对齐：杀进程窗口、冷启动仅凭 S3 恢复（若 Phase 2 未完成，本阶段至少完成平面 A 与「配置层」隔离，平面 B 跟 ducklake 计划并行）。

### Phase D4 — 收尾

- README / deployment / proto-http 勘误（引擎唯一、S3 协议、DevMode 行为）。
- ui-plan 验收项：DevMode 下 Databases / SqlConsole / DataManager / S3Manager 真数据可用。

---

## 6. 验收标准（DoD）

### 6.1 功能

1. 配置无法选择非 DuckLake 用户库引擎；二进制 / 源码无 turso 用户库路径。
2. §3.1–§3.3 在 DuckLake 上行为与 `proto-http` 一致（含 durability / snapshot 字段规则）。
3. §3.4 在 MinIO（S3 兼容）上四接口 e2e 通过；presign URL 可下载。
4. 平面 A 与平面 B prefix 隔离；删库不误删用户文件。
5. `./build.sh dev` 一键联调上述页面。

### 6.2 质量

**自动化用例（后端，跳过 MinIO e2e）**

| 计划项 | 测试 |
|---|---|
| 创建后 ready | `catalog.TestPlan_CreateDatabaseBecomesReady`、`api.TestPlan_CreateDatabaseHTTPReturnsReady` |
| 同步软删清平面 B、不误删平面 A | `catalog.TestPlan_DeleteDatabaseSyncPurgesPlaneBOnly` |
| creating 可删 / DevMode 无 purger | `catalog.TestPlan_DeleteFromCreatingAllowed`、`TestPlan_DeleteWithoutPurgerStillMarksDeleted` |
| open 救卡住的 creating | `api.TestPlan_OpenDatabasePromotesCreatingToReady` |
| 删库 HTTP 返回 deleted | `api.TestPlan_DeleteDatabaseHTTPReturnsDeleted` |
| descriptor v2 / 拒 turso engine / 拒 v1 | `objectstore.TestPlan_*`、`TestDescriptorValidateRejectsV1Turso` |
| 无 turso/local factory / 无 DeleteDatabaseHandler | `objectstore.TestPlan_NoTursoUserDatabasePackage` |
| 配置拒非 ducklake | `config.TestValidateRejectsLegacyEngines` |

运行：`go test ./internal/catalog/ ./internal/objectstore/ ./internal/api/ ./internal/config/ -run Plan_ -count=1`

- 相关包单测绿；**不做 MinIO e2e**（产品确认）。
- 日志/指标不输出 AccessKey/Secret；S3 错误不回显原始签名串。

### 6.3 文档

- 本计划 + proto-http 勘误 + ducklake 计划「退役」章节勾选完成。
- `config.example.yaml` / `.env.example` 与真实加载行为一致（避免再出现「YAML 不加载」过时注释）。

---

## 7. 风险与决策点

| 风险 | 影响 | 建议决策 |
|---|---|---|
| 存量 Turso 数据 | 无法被 DuckLake 直接打开 | **已决：丢弃**；拒绝打开旧 descriptor，无官方 migrate |
| COS 与 AWS 预签名差异 | 前端「打开/下载」失败 | e2e 矩阵含 path-style + 虚拟主机；失败则文档标注仅支持 MinIO/AWS |
| DevMode LocalFileStore（已决继续本地盘）presign | 与生产行为不一致 | 契约写明 DevMode 本地 URL / 或前端禁用「外链打开」 |
| 过早删 turso 包导致测试塌方 | 进度风险 | D1 先断装配，D2 再删包 |
| DataManager 隐式第一库 | 多库踩坑 | 前端提示；后端改路径放到独立小计划 |

**已拍板（2026-09-16）：**

| # | 决策 | 含义 |
|---|---|---|
| 1 | **存量 Turso：丢弃** | 不提供原地升级/迁移工具；旧 Turso 库不可打开，需业务侧自行导出后在 DuckLake 重建 |
| 2 | **DevMode 用户文件：继续本地盘** | 平面 A 在 `dev_mode` 下仍用 `LocalFileStore`；非 DevMode 必须走 aws-sdk-go-v2 S3 |
| 3 | **软删清理：同步删 S3** | `DELETE database` 进入 `deleting` 后，在请求路径内（或同进程紧随）同步删除平面 B 前缀与 descriptor，再标 `deleted`；不依赖独立 jobs 队列完成本轮交付 |

实现时以上三条视为需求，不再列为开放问题。

---

## 8. 与现有文档的关系

| 文档 | 关系 |
|---|---|
| [`db-ducklake-plan.md`](./db-ducklake-plan.md) | 引擎/Syncer/维护/SQL 方言的详细设计；本计划消费其 Phase 1–4，并把 Phase 4「退役」提前到产品主路径 |
| [`proto-http.md`](./proto-http.md) / [`proto.http`](./proto.http) | HTTP 契约；本计划不改变路径与字段，除非 DevMode presign 需勘误 |
| [`ui-plan-v2.md`](./ui-plan-v2.md) | 前端页面与联调验收；本计划提供后端收敛前提 |
| [`ui-settings-chat-plan.md`](./ui-settings-chat-plan.md) | 无关；不阻塞 |

---

## 9. 建议排期（粗估，人天）

| 阶段 | 粗估 |
|---|---|
| D0 基线 | 0.5 |
| D1 收敛装配 | 1–2 |
| D2 删包 + descriptor | 1–2 |
| D3 API 加固 + 软删 | 1–2 |
| S1 MinIO e2e | 1 |
| S2 隔离与故障 | 1 |
| D4 文档收尾 | 0.5 |
| **合计** | **约 6–10** |

（不含完整 ducklake Phase 2/3 压测与生产迁移工具；那些仍跟 `db-ducklake-plan.md`。）

---

## 10. 下一步

1. ~~确认 §7 三个产品决策~~（已完成）。  
2. ~~链入 `ui-plan-v2.md` 配套文档~~（已完成）。  
3. 你确认「开始实现」后，按 **Phase D1 → S1 → D2 → D3（同步软删）→ S2 → D4** 开干（可与 ducklake Phase 2 并行，但 **D1 门禁优先**）。
