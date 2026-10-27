# PocketBase 对标能力差距与补齐计划（planv6.3）

> 状态：已完成（P0、P1、P2，2026-10-27）
> 实施说明：
> - P0/P1 数据面、备份、HTTP 边界、Scoped Key、Realtime、文件与日志设置已落地；P2-1～P2-6 的后端、控制台 UI、SDK、CLI 与文档已补齐。
> - 系统库按现行最终态 `internal/systemdb/schema.go` 管理，不再追加版本化迁移；`migrate status|plan` 改为只读检查最终态表与待执行 DDL，不提供 down。
> - 验收：DuckDB/SQLite 双引擎测试与真实 DuckDB HTTP 端到端通过；前端 810 项测试通过，分支覆盖率 95%、函数覆盖率 95.06%；JS SDK、Go SDK、`gofunction`、`godatabase`、`gosandbox`、`gocloudagent` 测试通过。
> - 已知无关基线问题：根目录 `go test ./...` 仍会因既有 `skills/simplify-code-eval-std/evals/files/20-go-minimal` 缺少 `readName` 失败；`internal/...`、`cmd/...` 与本次相关包均通过。
> 证据基线：
> - PocketBase：`thirdparty/pocketbase`，提交 `9d281cca`（2026-10-23），CHANGELOG 顶部版本 v0.40.6，Go 1.27 + 纯 Go SQLite（`modernc.org/sqlite`）+ goja。
> - SimpleBase：工作区 HEAD，系统库迁移 v1–v50（`internal/systemdb/migrate.go`），用户库已切换为「项目级单文件」（`plan/planv6.2/project-single-file-database-plan.md`）。
> 所有结论均可按 §14 的证据索引回到源码核对。

---

## 1. 背景、目标与定位判断

### 1.1 触发点

PocketBase 是「单文件后端即服务（BaaS）」的极简范式：一个可执行文件同时提供 SQLite 库、集合建模、记录 REST API、实时订阅、文件管理、用户认证、管理台、备份恢复与 JS 扩展。SimpleBase 是「多租户云数据库平台」：项目隔离的单文件库 + 云函数 + 定时任务 + 云沙盒 + LLM 网关 + 云 Agent。

两者都自称「一个后端」，但对标后发现：**SimpleBase 在平台侧（多租户、S3 持久、指标、审计、配额、LLM/Agent）显著强于 PocketBase；在应用侧（数据建模 / 记录 API / 实时 / 备份 / 认证闭环）明显弱于 PocketBase**。本计划就是把应用侧缺口逐项列出并给出可实施方案。

### 1.2 定位差异（决定「哪些能力不该抄」）

| 维度 | PocketBase | SimpleBase |
|---|---|---|
| 部署形态 | 单机单进程，`pb_data` 本地目录 | 单写实例 + S3 在线持久层，容器化部署 |
| 租户模型 | 单应用；靠 `collection` 隔离业务表 | `tenant → project → 默认数据库`，物理文件隔离 |
| 「用户」概念 | 两层：superuser（运维）+ auth collection 的终端用户 | 三层：superuser/超管、admin（全局只读）、user（项目成员） |
| 扩展模型 | goja 跑 JS hooks / JS 迁移 | 编译期类型安全的 Go 云函数（SSA 解释器，不调用 `go build`） |
| API 面 | `/api/collections/{c}/records` 通用 CRUD | `/v1/projects/:p/databases/:d/...` 资源化 API |
| 备份 | `VACUUM INTO` + zip，内置 cron 与 S3 备份 | 无（仅 S3 版本控制 + `reset`） |
| 浏览器直连 | 默认 CORS `*`，为前端直连设计 | 无 CORS，理论上只给服务端/CLI 用 |

结论：**不能把 PocketBase 当功能清单直接照搬**。本计划采用四档判定——✅ 已有 / ⚠️ 部分 / ❌ 缺失 / ⛔ 不适用（附理由），并在 §6 集中说明不采纳项。

### 1.3 分析范围

- PocketBase 全量：`apis/`（24 个 handler 文件）、`core/`（集合/字段/记录/认证/备份/迁移/Settings）、`tools/`（search / filesystem / mailer / cron / security / picker / archive …）、`plugins/`（jsvm / migratecmd / update）、`ui/src`（Svelte 控制台）、`migrations/`。
- SimpleBase 全量：`internal/api/router.go`（901 行）、`internal/auth`、`internal/catalog`、`internal/database`、`internal/objectstore`、`internal/cronjob`、`internal/sbcli`、`gofunction/`、`gosandbox/`、`gocloudagent/`、`ui/src`、`docs/`、`packages/{js-sdk,go-sdk}`。

### 1.4 结论摘要（一页纸）

**89 项能力对照结果：✅ 19 / ⚠️ 17 / ❌ 48 / ⛔ 5。**

按「对 SimpleBase 用户的实际影响」排序，真正需要补的只有 4 个 P0 和 4 个 P1：

| 编号 | 缺口 | 影响 | 优先级 |
|---|---|---|---|
| P0-1 | 文档数据面太弱：无分页、无 filter/sort/fields、无单条 GET、无批量 | 集合 API 只能当 demo 用，做不了真实应用后端 | P0 |
| P0-2 | 集合没有字段元数据与校验 | 任意外形 JSON 入库，无法建模、无法安全查询 | P0 |
| P0-3 | 没有备份与恢复 | 数据安全底线缺失，误删即不可恢复 | P0 |
| P0-4 | 没有 CORS / gzip / 限流 / 安全响应头 / 真实 IP 解析 | 浏览器直连不可用；任意客户端可打满单写实例 | P0 |
| P1-1 | API Key 只有 6 个权限位（`database:read/write/admin`、`llm:invoke`、`project:admin`、`user:admin`），无资源/动作/字段级范围 | 无法把 Key 安全下发给第三方或前端 | P1 |
| P1-2 | 没有记录级实时订阅 | 做不了协作编辑、通知、看板刷新 | P1 |
| P1-3 | 文件能力单薄（无 MIME 校验、无代理下载、无私有直链、无目录语义） | 不能当应用的文件后端 | P1 |
| P1-4 | 设置散落在 modal，日志无统计 | 运维体验差，容量/错误趋势看不见 | P1 |

P2 另列 6 项（导出、视图集合、事件触发器、迁移工具、OpenAPI/API 预览、数据浏览器 UI），按需排期。

**不采纳 5 类 12 项**（§6），核心是：goja JS hooks（已有更强的 Go 云函数）、auth collection 全家桶（终端用户体系属产品决策，见 §7 D1）、自更新/ACME/www 重定向（部署层已解决）、迁移 down（DuckLake DDL 不可回滚）、多实例 notify watcher（单写实例模型）。

---

## 2. 能力对照矩阵

判定列描述 **SimpleBase 现状**；PocketBase 实现列给出可复用的设计参考与源码位置。

### 2.1 数据模型与集合（11 项：✅1 / ⚠️2 / ❌8）

| # | 能力 | PocketBase 实现（证据） | SimpleBase 现状（证据） | 判定 |
|---|---|---|---|---|
| 1 | 集合与字段元数据注册表 | `core/collection_model.go`、`core/fields_list.go` | 只有物理表，无任何元数据（`internal/api/data_handler.go:ListCollections` 直接读 `information_schema.tables`） | ⚠️ |
| 2 | 字段类型体系（text/number/bool/date/email/url/editor/select/file/relation/json/password/geoPoint/autodate） | `core/field_*.go`，注册表 `core/field.go:67` | 无 | ❌ |
| 3 | 字段校验（必填、长度、正则、唯一、默认值、白名单） | `core/field_text.go`、`field_number.go`、`field_select.go`；集合级 `core/collection_validate.go` | 无；`data` 列是自由 JSON 文本 | ❌ |
| 4 | 关系字段与级联删除 | `core/field_relation.go`（`CollectionId/CascadeDelete/MinSelect/MaxSelect`）、`core/record_query_expand.go` | 无 | ❌ |
| 5 | 视图集合 + SQL dry-run | `core/view.go:82,138`（`CreateViewFields`/`DryRunView`）、`POST /api/collections/meta/dry-run-view` | 无（`sqlguard` 只放行 SELECT，但没有视图集合概念） | ❌ |
| 6 | 索引管理与校验 | `core/collection_model.go`（`Indexes`）、`tools/dbutils/index.go`、`core/collection_validate.go:525` | 无 API；只能在 SQL 控制台手写 `CREATE INDEX` | ❌ |
| 7 | 集合 schema 导入导出与差异确认 | `core/collection_import.go`、`PUT /api/collections/import`、`ui/src/settings/sync/*` | 无 | ❌ |
| 8 | 集合级 API 规则（list/view/create/update/delete） | `core/collection_model.go:358-362`、规则求值 `tools/search/filter.go:40` + `core/record_field_resolver.go` | 只有路由级权限位（`auth.Require`），无集合级规则 | ❌ |
| 9 | 自动时间戳（onCreate/onUpdate） | `core/field_autodate.go`（`RecordInterceptor` 写入） | 仅 `created_at`（`data_handler.go:createCollectionSQL`），无 `updated_at` | ⚠️ |
| 10 | 集合截断 / 重命名 | `DELETE /api/collections/{c}/truncate`、`PATCH /api/collections/{c}` | 无（`CreateCollection` 之外没有 DDL 面） | ❌ |
| 11 | 写入时自动建表（隐式 schema） | 集合必须先建，记录不能隐式建表 | 有：`CreateDocument` 先 `CREATE TABLE IF NOT EXISTS` | ✅ |

**判定**：SimpleBase 的「集合」目前只是「一张 `(id, data, created_at)` 的薄表」，语义接近「带 id 的 JSON 抽屉」，远不是数据模型。

### 2.2 文档 / 记录 API（11 项：✅0 / ⚠️3 / ❌8）

| # | 能力 | PocketBase 实现（证据） | SimpleBase 现状（证据） | 判定 |
|---|---|---|---|---|
| 1 | 分页（`page/perPage/totalItems/totalPages/skipTotal`） | `tools/search/provider.go:56,105-230` | 无；`ListDocuments` 固定 `ORDER BY created_at DESC` + 硬上限 1000（`data_handler.go:117`） | ❌ |
| 2 | `filter` 表达式（比较、逻辑、`?` 通配、多值匹配、时间宏） | `tools/search/filter.go`（`BuildExpr`、`manyVsManyExpr`）、`tools/search/identifier_macros.go` | 无；只有 SQL 直查 | ❌ |
| 3 | `sort` 多字段 / 多值排序 | `tools/search/sort.go`、`provider.go:154-172` | 无 | ❌ |
| 4 | `fields` 字段裁剪（含 `:excerpt(...)` 修饰） | `tools/picker/pick.go:31`、`picker/modifiers.go` | 无；固定返回 `id` + `data` 全量展开 | ❌ |
| 5 | `expand` 关联展开（`_via_`、≤6 层） | `core/record_query_expand.go:24-65` | 无 | ❌ |
| 6 | 单条 `GET /records/{id}` | `apis/record_crud.go:30` | 无（只能列表里找） | ❌ |
| 7 | 部分更新（PATCH）与字段修饰符（`+`/`-`/`:each`/`:isset`） | `core/field.go:125-154`、`forms/record_upsert.go` | 只有整体替换 `PUT`（`UpdateDocument` → `SET data=?`） | ⚠️ |
| 8 | 批量写单事务（`POST /api/batch`，含 multipart 文件） | `apis/batch.go`（`Settings.Batch.Timeout` 默认 3s） | 只有 SQL batch（`/databases/:id/batch`），文档批量为 N 次请求 | ⚠️ |
| 9 | 软删 / 回收站 | 无（PocketBase 也是硬删） | 硬删 + 删表无 API | ⚠️（对等） |
| 10 | 端点级动态 body limit（按字段 `CalculateMaxBodySize` 求和） | `core/field.go` 接口、`apis/record_crud.go:31` | 全局 `limits.max_request_bytes`（1MB）+ S3/envd 例外 | ❌ |
| 11 | 每集合路径级限流 | `collectionPathRateLimit`（`apis/file.go:45`） | 无 | ❌ |

**判定**：第 1–7 项合起来就是「一个可用数据 API 的最小集」，当前一条都不满足，这是最大的产品缺口（P0-1）。

### 2.3 认证与用户（13 项：✅5 / ❌8）

| # | 能力 | PocketBase 实现（证据） | SimpleBase 现状（证据） | 判定 |
|---|---|---|---|---|
| 1 | API Key 认证（哈希存储、可吊销、按资源签发） | 无（PB 只有 superuser / record token） | `internal/auth/api_key_repository.go`（HMAC-SHA256 + `revoked_at`） | ✅ |
| 2 | 控制台 JWT access + refresh | `core/record_tokens.go`、`apis/record_auth_refresh.go` | `internal/auth/session.go`、`sys_user_sessions` | ✅ |
| 3 | 密码哈希 | bcrypt（`core/field_password.go:322`） | argon2id（`internal/auth/password.go`，更强） | ✅ |
| 4 | 角色与权限位 | superuser 二元 + 集合规则 | `superadminl1/admin/user` + 6 个权限常量（`internal/auth/principal.go`） | ✅ |
| 5 | 会话管理与吊销 | `record.tokenKey` 轮换 | `sys_user_sessions` + `Logout` | ✅ |
| 6 | 密码找回（一次性 token + 邮件） | `apis/record_auth_password_reset_*.go`、`core/record_tokens.go:99` | 缺失；管理员只能 `PATCH /users/:id` 手改 | ❌ |
| 7 | 邮箱验证 | `apis/record_auth_verification_*.go` | 缺失（`sys_users.email` 存在但未验证） | ❌ |
| 8 | MFA / TOTP 二次验证 | `core/mfa_model.go`、`apis/record_helpers.go:148 wantsMFA` | 缺失 | ❌ |
| 9 | OAuth2 第三方登录（30+ provider、PKCE、state） | `tools/auth/*`、`apis/record_auth_with_oauth2*.go` | 缺失 | ❌ |
| 10 | 邮箱 OTP 登录 | `core/otp_model.go`、`apis/record_auth_otp*.go` | 缺失 | ❌ |
| 11 | 邀请 / 项目成员/角色 | 无（PB 也无；靠 collection 规则自行实现） | 只有 owner（`sys_project_owners`），`TeamSupported:false` | ❌ |
| 12 | 模拟登录（superuser 代登录某记录） | `apis/record_auth_impersonate.go` | 缺失 | ❌ |
| 13 | IP/CIDR 白名单（superuser 面） | `core/settings_model.go:125` + `apis/middlewares.go:310` | 缺失 | ❌ |

**判定**：SimpleBase 的平台侧认证比 PocketBase 更工程化（argon2id、refresh 会话、API Key 哈希、委托 JWT）；缺的全是「面向终端用户的账号闭环」，其前置条件是 §7 D1 的终端用户体系决策。

### 2.4 文件与存储（10 项：✅2 / ⚠️2 / ❌6）

| # | 能力 | PocketBase 实现（证据） | SimpleBase 现状（证据） | 判定 |
|---|---|---|---|---|
| 1 | 上传 / 列表 / 删除 | `apis/file.go`、`core/field_file.go` | `internal/api/s3_handler.go`（multipart，50MiB） | ✅ |
| 2 | 预签名下载 | `apis/file.go:44`（file token） | `GET /s3/presign`（固定 15 分钟，`s3_handler.go:190`） | ✅ |
| 3 | 服务端代理下载 / 流式 | `apis/file.go:download`（支持 `?thumb=`） | 无；`local` 模式下只能靠 presign 伪装（`objectstore/localfilestore.go`） | ❌ |
| 4 | MIME 嗅探与类型白名单 | `core/field_file.go:104-138`（`MimeTypes`）+ `filesystem` 校验 | 直接信任 multipart 头（`s3_handler.go:UploadObject`） | ❌ |
| 5 | 缩略图（`WxH`/`WxHt`/`WxHb`/`WxHf`，含像素上限防解压炸弹） | `tools/filesystem/filesystem.go:608-736`、`PB_THUMBS_MAX_SOURCE_PIXELS` | 无 | ❌ |
| 6 | 私有文件 token（可配 TTL、绑定资源） | `core/record_tokens.go:146 NewFileToken` | ⚠️ 只有 S3 presign，TTL 写死 15 分钟，不可绑定 | ⚠️ |
| 7 | 目录语义（`delimiter` / 前缀浏览） | 无（PB 按记录+字段组织） | 仅 key 前缀（`List(prefix)`），`ValidateFileKey` 明确无目录概念 | ⚠️ |
| 8 | 记录关联文件字段（自动删除孤儿文件） | `core/field_file.go` `RecordInterceptor` | 无（文件与文档无关联） | ❌ |
| 9 | 内容去重 / 内容寻址 | 无 | 无 | ❌（对等） |
| 10 | 文件访问限流 | `collectionPathRateLimit` | 无 | ❌ |

### 2.5 实时（5 项：✅0 / ⚠️3 / ❌2）

| # | 能力 | PocketBase 实现（证据） | SimpleBase 现状（证据） | 判定 |
|---|---|---|---|---|
| 1 | 记录变更订阅（SSE `GET /api/realtime`） | `apis/realtime.go:37`、`tools/subscriptions/*` | 无；`internal/api/sse.go` 只有 `beginSSE` 工具函数 | ❌ |
| 2 | 断线续传 / 事件 ID | `PB_CONNECT` 首帧 + `clientId` | ⚠️ 仅 Agent run 有（`run_events.go` 环形缓冲 2000 帧） | ⚠️ |
| 3 | 订阅级 filter + 逐条鉴权 | `apis/realtime.go:856 realtimeCanAccessRecord` | 无 | ❌ |
| 4 | 连接配额 / 心跳 / 空闲与最长时限 | `realtime.go:69-70`（idle 5min / max 30min） | ⚠️ Agent run 有 idle TTL 10m 与 `maxRuns=512` | ⚠️ |
| 5 | WebSocket 通道 | 无（PB 用 SSE） | ⚠️ 仅沙盒终端（`e2b_terminal.go`） | ⚠️ |

### 2.6 平台与运维（19 项：✅5 / ⚠️3 / ❌9 / ⛔2）

| # | 能力 | PocketBase 实现（证据） | SimpleBase 现状（证据） | 判定 |
|---|---|---|---|---|
| 1 | 备份 / 恢复 API | `apis/backup.go:19-24`、`core/backup_create.go:67`（`VACUUM INTO` + 边备份边同步） | 无；`docs/ops/deployment.md` 明确「独立 backups/restore API 已移除」 | ❌ |
| 2 | 定时备份 + 保留数量 | `core/settings_model.go:476-490`（`Cron` + `CronMaxKeep`） | 无 | ❌ |
| 3 | 日志查询 | `GET /api/logs`（`apis/logs.go:19`） | `GET /v1/projects/:p/logs`（`system_handlers.go:47`） | ✅ |
| 4 | 日志统计 / 图表 | `GET /api/logs/stats` + `ui/src/logs/logsChart.js` | ⚠️ 只有 `LogLevelStats`（Agent 工具内部用），无趋势 API | ⚠️ |
| 5 | 日志保留策略 | `Settings.Logs.MaxDays/MaxDataSize/MinLevel` | `sys_log_retention` + `GET/PUT /logs/retention` | ✅ |
| 6 | 日志级别 / 内容上限 / IP 与 authId 采集开关 | `settings_model.go:548-558` | ⚠️ 有级别与保留天数，无内容上限与 PII 开关 | ⚠️ |
| 7 | 邮件发送（SMTP + HTML 模板 + 纯文本副本） | `tools/mailer/*`、`mails/record.go`、`apis/settings.go:18` | 无 | ❌ |
| 8 | 全局限流规则（按标签/路径/方法分类） | `apis/middlewares_rate_limit.go`、`settings_model.go:177-185` | 无；只有并发闸门（`max_concurrent_queries` 等）与配额 | ❌ |
| 9 | CORS（含预检） | `apis/middlewares_cors.go`，默认 `*` | 无 | ❌ |
| 10 | gzip | `apis/middlewares_gzip.go`（仅静态资源） | 无 | ❌ |
| 11 | 安全响应头 | `apis/middlewares.go:288 securityHeaders` | 无 | ❌ |
| 12 | 内置 TLS / ACME / www 重定向 | `apis/serve.go:134-150`（autocert） | 由反向代理/Ingress 承担（`docs/ops/deployment.md`） | ⛔ |
| 13 | 迁移 down / 回滚 | `core/migrations_runner.go:83-91`（`down [n]`） | 只能前进；DuckLake DDL 无事务保证 | ⛔ |
| 14 | 迁移 automigrate + `migrate` CLI | `plugins/migratecmd/*` | 无（`ApplySystemMigrations` 只能 up） | ❌ |
| 15 | 健康检查 | `GET /api/health` | `/health/live` + `/health/ready`（分存活/就绪，更强） | ✅ |
| 16 | Prometheus 指标 | 无 | 22 个 `simplebase_*` 指标（`internal/observability/metrics.go`） | ✅ |
| 17 | 审计与配额 | 无（只有 activity log） | `sys_operations` + `sys_project_quotas` + `GET /audit`、`/quota` | ✅ |
| 18 | 首次安装向导 | `apis/installer.go`（30 分钟 token 打开 UI） | ⚠️ bootstrap 超管 + `must_change_password`，无向导 | ⚠️ |
| 19 | 系统告警邮件（备份失败等） | `core/system_alert.go` | 无 | ❌ |

### 2.7 扩展与自动化（8 项：✅3 / ❌2 / ⛔3）

| # | 能力 | PocketBase 实现（证据） | SimpleBase 现状（证据） | 判定 |
|---|---|---|---|---|
| 1 | 服务端函数扩展 | goja JS（`plugins/jsvm`），无编译期检查 | Go 云函数（`gofunction/`）+ 不可变版本 + 调用流水 | ✅ |
| 2 | JS / TS hooks | `plugins/jsvm/binds.go`（`$app/$dbx/$apis/$http/...`）+ `types.d.ts` | 无（等价物是 gofunction，不采纳 goja） | ⛔ |
| 3 | 事件触发器（DB 变更 / 文件事件 / webhook → 函数） | `core/base.go` 的 `OnRecord*` hook 体系 | 无；云函数只有 HTTP 与 cron 两种触发 | ❌ |
| 4 | 定时任务 | `tools/cron` + `GET/POST /api/crons` | `internal/cronjob`（cron/interval/once + 运行记录 + 手动触发，更强） | ✅ |
| 5 | 自更新 | `plugins/update`、`plugins/ghupdate` | 无（镜像 + 滚动升级） | ⛔ |
| 6 | 管理台 UI 扩展注入 | `core.ServeEvent.UIExtensions`、`apis/extensions.go` | 无（Vue 控制台不开放第三方注入） | ⛔ |
| 7 | 云沙盒 / LLM 网关 / 云 Agent | 无 | `internal/sandbox`、`internal/llmgateway`、`gocloudagent/` | ✅ |
| 8 | MCP 客户端/服务端 | 无 | 无（仅计划 `plan/planv6.2/gocloudagent-mcp-plan.md`） | ❌ |

### 2.8 控制台 UI（12 项：✅3 / ⚠️4 / ❌5）

| # | 能力 | PocketBase 实现（证据） | SimpleBase 现状（证据） | 判定 |
|---|---|---|---|---|
| 1 | 集合管理页（侧栏 + 创建/编辑弹窗） | `ui/src/collections/*` | ⚠️ `ui/src/components/databases/CollectionPanel.vue` + `CreateCollectionModal.vue`（只能建与看） | ⚠️ |
| 2 | 记录表格（筛选、排序、分页、列选择、搜索历史） | `ui/src/records/recordsList.js`（33KB）+ `recordsSearchbar.js` | ❌ 无 | ❌ |
| 3 | Schema 设计器（字段编辑器/索引/规则/视图/Auth 选项） | `collectionUpsertModal.js`（38KB）+ `collectionFieldsTab.js` | ❌ 只有建表/加列表单（`SchemaPanel.vue`） | ❌ |
| 4 | SQL 控制台 | `ui/src/sql/pageSQLConsole.js` | ⚠️ 并入 `Databases.vue` | ⚠️ |
| 5 | 日志页与统计图 | `pageLogs.js` + `logsChart.js` | ⚠️ `Logs.vue` 有列表与级别统计，无趋势图 | ⚠️ |
| 6 | 备份管理与恢复 UI | `ui/src/settings/backups/*` | ❌ 无 | ❌ |
| 7 | 设置中心（应用/邮件/存储/备份/Cron/限流） | `ui/src/settings/**` | ⚠️ `SettingsModal.vue`（连接/模型/存储三块），入口是 `?settings=1` | ⚠️ |
| 8 | API 预览 / Playground（端点文档 + cURL/JS 片段） | `ui/src/apiPreview/*`、`docsList/docsView/...` | ❌ 无 | ❌ |
| 9 | Realtime 调试台 | `docsRealtime.js` + `apiPreviewModal.js` | ❌ 无 | ❌ |
| 10 | 监控大盘 | 无 | ✅ `Dashboard.vue` + `/metrics/summary|trend` | ✅ |
| 11 | 用户管理 | `settings/application/superuserAccordion.js` | ✅ `Users.vue`（superadminl1） | ✅ |
| 12 | KV 浏览器 / 沙盒终端 / 函数编辑器 / Agent 会话 | 无 | ✅ `KeyValue.vue`、`SandboxTerminal.vue`、`GoMonacoEditor.vue`、`AgentManager.vue` | ✅ |

---

## 3. P0 缺口详细设计

### 3.1 P0-1 文档数据面 v2

#### 现象与影响

`GET /v1/projects/:p/databases/:d/data/collections/:collection` 当前实现（`internal/api/data_handler.go:102-134`）：

```sql
SELECT id, data FROM <collection> ORDER BY created_at DESC   -- 硬上限 1000，无分页
```

调用方拿到 1000 条全量 JSON，既不能翻页、不能按条件过滤、不能排序、也不能只取需要字段。任何超过演示规模的应用都做不了。

#### 参考实现（PocketBase）

- 查询参数在 `tools/search/provider.go:105-230` 统一解析：`page/perPage/skipTotal/filter/sort/expand/fields`。
- filter 编译到 SQL 在 `tools/search/filter.go:40`（`FilterData.BuildExpr`），值一律走占位符。
- 字段裁剪 `tools/picker/pick.go:31`（纯内存裁剪，不影响 SELECT）。
- 响应形状 `{page, perPage, totalItems, totalPages, items}`（`provider.go:56`）。

#### 目标契约

```text
GET    .../data/collections/:collection
         ?limit=50            # 1..200，默认 50
         &cursor=<opaque>     # keyset 分页；与 offset 互斥
         &offset=0            # 兼容旧调用，≤100000
         &sort=-created_at,title
         &filter=status="open" && priority>=2
         &fields=id,title,status
         &count=1             # 需要 totalItems 时才扫计数
GET    .../data/collections/:collection/documents/:id
POST   .../data/collections/:collection/documents          # 单条
POST   .../data/collections/:collection/documents/batch    # {documents:[...]} 单事务
PATCH  .../data/collections/:collection/documents/:id      # JSON merge patch；null 删除键
PUT    .../data/collections/:collection/documents/:id      # 保持现状：整体替换
DELETE .../data/collections/:collection/documents/:id
DELETE .../data/collections/:collection                    # 新增：truncate（ProjectAdmin）
```

响应（v2）：

```json
{
  "items": [{"id": "...", "created_at": "...", "updated_at": "...", "data": {"title": "..."}}],
  "next_cursor": "eyJjIjoiMjAyNi0xMC0yNlQwMzowMDowMFoiLCJpIjoiLi4uIn0",
  "total": 128,
  "request_id": "..."
}
```

> 兼容策略：`rows` 字段保留为 `items` 的别名一个版本周期，SDK 升级后删除；`js-sdk`/`go-sdk` 的 `collections.listDocuments` 先加新字段，不改旧字段。

#### filter 语言（受限子集，不做完整 PocketBase 表达式）

```text
filter   = term { ("&&" | "||") term }
term     = "(" filter ")" | field op value
field    = <sys_collection_fields 中注册的字段名>          # 不接受任意 JSON path
op       = "=" | "!=" | ">" | ">=" | "<" | "<=" | "~"(contains) | "!~" | "in" | "is null" | "is not null"
value    = string | number | true | false | null | [ v1, v2, ... ]
```

**安全要点（关键设计）**：字段名必须命中 P0-2 的字段注册表，编译时由注册表给出固定 SQL 片段与类型，配合参数化值：

| 字段类型 | 生成的 SQL 片段（以 DuckDB 为例） | 绑定 |
|---|---|---|
| text / email / url / select | `json_extract_string(data, '$.<f>')` | `?` |
| number | `CAST(json_extract(data, '$.<f>') AS DOUBLE)` | `?` |
| bool | `CAST(json_extract(data, '$.<f>') AS BOOLEAN)` | `?` |
| date | `CAST(json_extract_string(data, '$.<f>') AS TIMESTAMP)`（RFC3339） | `?` |

`~` 使用 `LIKE '%' || ? || '%'` 并转义 `%`/`_`；`in` 展开为 `IN (?, ?, ...)` 且受元素数上限（32）。未注册字段、运算符与类型不匹配、超长 filter（> 2KB）一律 400 `invalid_filter`，错误信息只回字段名不回 SQL。

排序：`sort=-created_at,title` → `ORDER BY <created_at> DESC, <json_extract(title)> ASC, id ASC`，字段同样必须来自注册表；`created_at/updated_at/id` 是内置可排序字段。

分页：keyset 优先。`created_at` 为 RFC3339Nano 字符串，可直接比较；cursor = base64(`{"c":created_at,"i":id}`)，条件 `(created_at < ?) OR (created_at = ? AND id < ?)`。`offset` 保留但走 `LIMIT/OFFSET`，并受 `schemaRowsMaxOffset` 同量级上限约束。

#### 落点文件

| 文件 | 改动 |
|---|---|
| `internal/api/collection_query.go`（新增） | 解析 `limit/cursor/offset/sort/filter/fields/count`，编译为 SQL + 参数 |
| `internal/api/collection_filter.go`（新增） | filter 词法/语法分析（手写递归下降，约 500 行，无新依赖） |
| `internal/api/collection_pick.go`（新增） | `fields` 裁剪（点号嵌套、`id` 强制保留） |
| `internal/api/data_handler.go` | `ListDocuments` 改走新查询器；新增 `GetDocument`/`PatchDocument`/`BatchDocuments`/`TruncateCollection` |
| `internal/database/dialect.go` | `Dialect` 增加 `JSONExtractText(path)`, `JSONExtractNumber(path)`, `JSONExtractBool(path)`, `CastTimestamp(expr)` |
| `internal/database/{duckdb,sqlite}` 方言实现 | 两套 JSON 函数差异收敛到这里（DuckDB `json_extract*`、SQLite `json_extract` + `json_type`） |
| `internal/api/router.go` | 新路由；全部写路由挂 `writeGate` 并补进 `internal/api/write_gate_test.go` 的 `writeRoutes` 清单 |

#### 边界与风险

- **扫描成本**：JSON 列上的 filter 必然全表扫描。用 `limits.max_filter_scan_rows`（默认 50000，可配）在应用层估算：`count(*)` 超限直接返回 `filter_scan_limit_exceeded` 并提示建索引/收窄条件，而不是让单写实例被一条查询拖死。
- **索引优化不做在 P0**：DuckDB 只支持列索引（不支持表达式索引），SQLite 支持表达式索引，双引擎无法统一。统一方案是「影子列 + 应用层双写」，单独立项（§5 之外，见 §7 D5）。
- **排序 NULL 语义**：两引擎默认 NULL 排序方向不同，统一为「NULL 视为最小」并在 SQL 里显式 `NULLS FIRST/LAST`。
- **`count=1` 的代价**：DuckDB 上 `count(*)` 扫描不便宜，默认关闭，文档写明。

#### 验收

1. 100 万文档的集合，`limit=50&cursor=...` 翻 3 页结果集不重叠、不漏。
2. `filter` 覆盖 6 种类型 × 8 种运算符；注入尝试（字段名带 `'`、值带 `--`、filter 含 `;`）全部 400，且 SQL 日志中不出现原始值。
3. `fields=id,title` 返回对象只含两键；`id` 不可被裁掉。
4. 批量 100 条中第 51 条非法 → 整体回滚，`failed_index=50`。
5. 只读实例（`instance.writable=false`）下新增写路由全部 503。

---

### 3.2 P0-2 集合元数据注册表与字段校验

#### 现象与影响

集合 = 物理表，`data` 是自由 JSON（`createCollectionSQL`：`(id VARCHAR, data VARCHAR, created_at VARCHAR)`）。后果：
- 无法回答「这个集合有哪些字段、什么类型」；
- P0-1 的 filter/sort 无类型可依（自由度换来的就是不安全查询）；
- 前端无法生成表单/表格列；
- 无法做任何校验（错误数据会在写入时静默接受）。

#### 参考实现

- 字段接口与可选能力（`core/field.go:70-185`）：`SetterFinder`/`GetterFinder`、`RecordInterceptor`、`MaxBodySizeCalculator`。
- 集合校验器（`core/collection_validate.go:44-71,477,525`）：字段/规则/索引一次性校验，冲突时给出可读错误。
- 集合导入支持 `deleteMissing`（`core/collection_import.go:37`），配合 UI 的 diff 确认（`collectionChangesConfirmationModal.js`）。

#### 方案选择（重要）

PocketBase 是「一个字段一列」。SimpleBase 采用 **JSON 单列 + 元数据注册表**，理由：

1. 双引擎（DuckDB / SQLite）与单文件模型下，频繁 `ALTER TABLE` 风险高（DuckDB DDL 无事务保证，`internal/systemdb/migrate.go` 注释已声明逐语句执行）。
2. 改字段类型/删字段在列模型下需要重建表（PocketBase 也是 `safety copy`），在单写实例 + 大表场景成本高。
3. 元数据足以支撑校验、查询编译、前端渲染、索引规划（后续影子列）。
4. 取舍：牺牲列级索引与列级统计下推，用 `max_filter_scan_rows` 兜底，索引方案留待 §7 D5 决策。

#### 系统表（v51 / v52）

```sql
CREATE TABLE IF NOT EXISTS sys_collections (
    id VARCHAR NOT NULL,
    project_id VARCHAR NOT NULL,
    name VARCHAR NOT NULL,
    schema_mode VARCHAR NOT NULL DEFAULT 'json',   -- json | typed（typed 预留，本期不实现）
    status VARCHAR NOT NULL DEFAULT 'active',      -- active | deleted
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS sys_collection_fields (
    id VARCHAR NOT NULL,
    project_id VARCHAR NOT NULL,
    collection VARCHAR NOT NULL,
    name VARCHAR NOT NULL,
    type VARCHAR NOT NULL,              -- text|number|bool|date|email|url|select|json
    required BIGINT NOT NULL DEFAULT 0,
    unique_flag BIGINT NOT NULL DEFAULT 0,
    default_json VARCHAR NOT NULL DEFAULT '',
    options_json VARCHAR NOT NULL DEFAULT '{}',   -- {min,max,pattern,values,max_select,max_size}
    hidden BIGINT NOT NULL DEFAULT 0,             -- 对外不可见（密码/内部标记）
    readonly BIGINT NOT NULL DEFAULT 0,           -- 仅服务端写入（如 created_by）
    order_index BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
```

`(project_id, name)` 与 `(project_id, collection, name)` 的唯一性由应用层保证（DuckLake 表不建 PK/UNIQUE，现有 `sys_*` 一致），单写实例使该保证成立。

#### 字段类型（收敛到 8 种）

| 类型 | 校验 | 查询比较 |
|---|---|---|
| `text` | `max`（默认 5000）、`min`、`pattern` | 字符串相等/`~` |
| `number` | `min`/`max`/`only_int` | 数值比较 |
| `bool` | 必须 JSON bool | 相等 |
| `date` | RFC3339 字符串，`min`/`max` | 时间比较 |
| `email` | 格式 + `except_domains`/`only_domains` | 字符串 |
| `url` | 格式 + 域名白/黑名单 | 字符串 |
| `select` | `values[]`、`max_select`、单/多值 | `=` / `in` / `~` |
| `json` | `max_size`（默认 64KB） | 不支持比较（只能 `is null`） |

刻意不做：`relation`（需要 expand，P2 随关系能力一起做）、`file`（P1-3 之后）、`password`（终端用户体系未定）、`geoPoint`、`editor`（富文本清洗放前端）。

#### API

```text
GET    .../data/collections                              # 列表附带 field 数量与文档数
GET    .../data/collections/:collection/meta             # 读 schema；未注册返回 404 schema_not_registered
PUT    .../data/collections/:collection/meta             # 覆盖式写入 schema（含字段增删改）
       ?dry_run=1                                        # 只校验现有数据，不落库
DELETE .../data/collections/:collection/meta             # 注销 schema（保留数据）
```

`PUT .../meta` 语义：
- 未注册 → 注册，同时校验物理表是否存在（不存在则 `CREATE TABLE IF NOT EXISTS`）。
- 已注册 → 全量替换字段表（`order_index` 按数组顺序）。
- 删除字段不删数据（`data` 里的旧键保留，只是不再校验/可查）。
- `dry_run=1` 时扫描最多 `max_filter_scan_rows` 条文档，返回 `{invalid: [{id, field, reason}], scanned: N, truncated: bool}`。

#### 校验时机

- `CreateDocument` / `PatchDocument` / `BatchDocuments`：**严格模式**——字段类型、required、长度、pattern、select 取值、未知键拒绝（`unknown_field`）。
- `UpdateDocument`（PUT 整体替换）：同上。
- 未注册 schema 的集合：**宽松模式**（现有行为不变），保证旧数据与旧 SDK 不破。
- `unique_flag`：写前 `SELECT ... WHERE json_extract_string(data,'$.f') = ?` 查重（单写实例下无并发窗口），命中返回 409 `duplicate_value`。

#### 落点文件

| 文件 | 改动 |
|---|---|
| `internal/collection/model.go`（新增包） | `Collection`/`Field`/`Schema` 领域模型、`ValidateDocument`、`MergePatch` |
| `internal/collection/fieldtypes.go`（新增） | 8 种类型的校验器与比较语义（单一事实来源） |
| `internal/collection/repository.go`（新增） | `sys_collections`/`sys_collection_fields` 读写 |
| `internal/catalog/sql_repository.go` | 复用系统库连接，或在 `internal/systemdb` 增加集合元数据查询 |
| `internal/api/collection_meta_handler.go`（新增） | `meta` 四个端点 |
| `internal/api/data_handler.go` | 写路径调用校验器；`ListCollections` 改为合并元数据 |
| `internal/systemdb/migrate.go` | v51 / v52（只定义，不自动执行，见 §9） |
| `ui/src/components/databases/*` | 集合字段的展示与新建时可选字段定义（P1 再做完整设计器） |

#### 边界与风险

- **旧集合**：已有物理表但无元数据 → 宽松模式；提供 `POST .../meta:infer`（P2）从现有数据抽样推断字段，避免用户手填。
- **性能**：每条写请求多一次元数据查询 → 走进程内 LRU 缓存（键 `projectID|collection`，TTL 30s，写入 `PUT meta` 时失效），与 `principalCacheTTL` 同风格。
- **批量校验**：批量写用一次 schema 读 + 逐条校验，不做 N 次查询。

#### 验收

1. 注册 schema 后写入类型错误文档 → 400 `field_type_mismatch`，`data` 未落库。
2. `required` 缺失、`select` 越界、`max` 超长、`pattern` 不匹配各有独立错误码与字段名。
3. `unique_flag` 字段重复写入 → 409；并发同值写入（10 goroutine）只有 1 条成功。
4. `dry_run=1` 不改变任何表与文档。
5. 未注册集合仍可按旧行为读写（回归用例需保留）。

---

### 3.3 P0-3 备份与恢复

#### 现象与影响

`docs/ops/deployment.md` 明确「独立 backups/restore API 已移除」，当前只有「S3 版本控制 + 系统库独立 + `reset`」。用户误删项目等于数据永久丢失（`DELETE` 数据库会清 S3 前缀）。这是**数据安全底线**，优先级等同功能缺口。

#### 参考实现

- PocketBase `core/backup_create.go:67`：`VACUUM INTO` 生成快照，同时监听备份期间新增/删除文件，避免长时间锁写。
- 备份产物 + 保留（`Settings.Backups.Cron/CronMaxKeep`）+ 上传外部备份包（`apis/backup_upload.go`）+ 下载走 file token（`apis/backup.go:22`）。

SimpleBase 不能直接照搬 `VACUUM INTO`（DuckDB 无此语法；且用户库可能是 duckdb 或 sqlite）。改为**引擎级 Snapshotter 接口**，并在仓库里复用已验证的跨库复制手法：`internal/database/ducklake/catalog_syncer.go:564-592` 已经用 `ATTACH <target> AS sb_catalog_backup` + `COPY FROM DATABASE ... TO ...` + `CHECKPOINT` 做过 catalog 一致性复制，同一手法可用于项目单文件快照。

#### 设计

```go
// internal/database/snapshot.go（新增）
type Snapshotter interface {
    // Snapshot 在持有写租约期间生成一致性副本，dst 为本地绝对路径。
    Snapshot(ctx context.Context, lease Lease, dst string) (SnapshotInfo, error)
}

type SnapshotInfo struct {
    Engine       string // duckdb | sqlite
    FormatVer    int
    SizeBytes    int64
    SHA256       string
    CompletedAt  time.Time
}
```

- `duckdb`：`ATTACH '<dst>' AS sb_snap` → `COPY FROM DATABASE main TO sb_snap` → `CHECKPOINT sb_snap` → `DETACH sb_snap`。
- `sqlite`：`VACUUM INTO '<dst>'`（要求目标不存在）→ 校验 `PRAGMA integrity_check`。
- 两者都在 `registry` 的 `ReadWrite` 租约内执行（与写互斥，时间可预期）；快照完成后再计算 SHA-256。

服务与存储：

```text
{backup.dir}/{projectID}/{UTC-timestamp}.{duckdb|sqlite}
{backup.dir}/{projectID}/{UTC-timestamp}.manifest.json   # SnapshotInfo + project/engine/instance id
```

S3 模式（`storage.database=s3`）额外上传到 `{s3.prefix}/_backups/{projectID}/...`，本地只作暂存；上传成功后才写 manifest，失败则整份丢弃（避免「有 manifest 无对象」）。

配置（追加到 `config.example.yaml`）：

```yaml
backup:
  enabled: false
  dir: "./backups"
  upload_to_s3: false        # storage.database=s3 时建议 true
  schedule: ""               # 复用 utils/crontab；空 = 不自动备份
  keep_last: 7               # 每项目保留份数
  keep_days: 30              # 超过天数清理
  max_per_project: 14        # 硬上限，防磁盘打满
  snapshot_timeout: 10m
```

API（全部 `ProjectAdmin`，写路由挂 `writeGate`）：

```text
GET    /v1/projects/:p/backups                 # 列表（id/time/size/engine/status）
POST   /v1/projects/:p/backups                 # 立即备份
GET    /v1/projects/:p/backups/:id/download    # 流式下载（不返回物理路径）
DELETE /v1/projects/:p/backups/:id
POST   /v1/projects/:p/backups/:id/restore     # 见下方恢复流程
POST   /v1/projects/:p/backups/upload          # 上传外部备份包（P2）
```

恢复流程（必须防「恢复一半」）：

1. 校验 `ProjectAdmin` + `writeGate` + 审计（只记 backupID 与 hash）。
2. `registry.Close(projectID)` 关闭句柄并等待引用计数归零；关闭失败 → 409 `project_busy`。
3. 校验 manifest 的 SHA-256/大小/engine 与当前项目一致；不一致 → 409 `snapshot_mismatch`。
4. 下载到 `{cache_dir}/projects/{pid}/.restore-tmp`（S3 模式），`integrity_check`（sqlite）或 `ATTACH` 试读（duckdb）。
5. 原子替换：旧文件 rename 到 `.restore-old` → 新文件 rename 到权威路径 → 删除 `.restore-old` → 清 `local-state.json`/manifest → 重开句柄。
6. 任一步失败：回滚到 `.restore-old` 并返回错误；句柄状态恢复为恢复前。

定时器在 `internal/backup/scheduler.go`（30s tick，与 `cronjob/scheduler.go` 同风格但独立，避免污染 `sys_cron_jobs` 语义），到点后对全部 `ready` 项目顺序执行快照（并发 1，避免抢占写租约）。

#### 落点文件

| 文件 | 改动 |
|---|---|
| `internal/database/snapshot.go`、`filedb/snapshot_{duckdb,sqlite}.go` | 引擎级快照实现 |
| `internal/backup/service.go` | 创建/列表/删除/保留清理/审计 |
| `internal/backup/restore.go` | 关闭→校验→替换→重开的状态机 |
| `internal/backup/scheduler.go` | 定时备份与清理 |
| `internal/api/backup_handler.go` | 5 个端点 + 流式下载 |
| `internal/api/router.go` | 挂载 + `write_gate_test.go` 的 `writeRoutes` 补录 |
| `internal/config/config.go`、`config.example.yaml` | `backup.*` |
| `internal/sbcli/commands.go` | `backup list|create|restore|delete`（后三个非只读） |
| `cmd/simplebase` | `simplebase backup` 透传 |

#### 边界

- **系统库不在本期范围**：系统库是 DuckLake 形态（catalog + parquet），快照语义不同。本期用户库优先，系统库备份列入 §7 D3 决策。
- **一致性 vs 停机**：快照在写租约内完成，期间该项目的写请求排队（不是报错）。文档必须写明「大库快照会造成秒级写延迟」。
- **不自动执行 DDL**：`sys_backups`（v53）只写进迁移列表，由用户正常启动或手动执行（§9）。

#### 验收

1. 1GB 项目（两引擎各一）备份 → 恢复 → 文档数与抽样内容一致。
2. 备份期间并发 50 个写请求全部成功（排队），无一条丢失。
3. 破坏 manifest 中的 SHA-256 → 恢复被拒绝，原文件不变。
4. 恢复到一半 kill 进程 → 重启后项目仍可用（`.restore-old` 兜底）。
5. `keep_last`/`keep_days`/`max_per_project` 三者同时生效，磁盘不超限。
6. 备份/恢复的审计与日志中不出现物理路径、S3 key、文档内容。

---

### 3.4 P0-4 HTTP 边界补齐

#### 现象与影响

`internal/api/router.go:212-290` 的中间件链只有 requestID / perf / Recover / accessLog / BodyLimit，**没有 CORS、gzip、限流、安全响应头、真实 IP 解析**（全仓 grep 0 命中）。后果：

- 浏览器前端无法直连（哪怕带了 Bearer）；
- 单写实例没有任何速率保护，一个循环就能打满；
- 反向代理后所有请求的 IP 都是同一个（限流与审计失真）；
- 响应缺安全头，嵌 iframe/嗅探类问题无防护。

#### 参考实现

`apis/base.go:19-57` 的中间件顺序；`middlewares_cors.go` / `middlewares_gzip.go` / `middlewares_rate_limit.go` / `middlewares.go:288 securityHeaders` / `:310 superuserIPsWhitelist`。

#### 设计

1. **CORS**：`middleware.CORSWithConfig`（Echo 内置，不新增依赖）。
   ```yaml
   http:
     cors:
       enabled: false
       allowed_origins: []        # 精确 origin，禁止 "*"
       allowed_methods: [GET, POST, PUT, PATCH, DELETE, OPTIONS]
       allowed_headers: [Authorization, Content-Type, X-Request-Id]
       exposed_headers: [X-Request-Id]
       allow_credentials: false
       max_age: 600
   ```
   默认关闭（保持现状，服务端到服务端无需 CORS）；文档写明「浏览器直连必须显式配置 origin，且不要用 `*` 配 `allow_credentials`」。
2. **gzip**：`middleware.GzipWithConfig`，跳过 `text/event-stream`（SSE 会被缓冲破坏实时性）、`/s3/presign` 重定向、以及 `Content-Type` 为大文件下载的响应。
3. **安全响应头**：自写 `securityHeadersMiddleware`：
   `X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY`、`Referrer-Policy: no-referrer`、`Permissions-Policy: geolocation=(), microphone=(), camera=()`；SPA 路由（`internal/web`）额外下发 CSP（注意 Monaco worker 与沙盒终端 WebSocket 需要 `worker-src`/`connect-src` 放行）。
4. **限流**：自实现 token bucket（无新依赖），维度 `projectID + principalID + IP`，规则按「路径前缀 + 方法类」匹配：
   ```yaml
   limits:
     rate_limit:
       enabled: false
       rules:
         - match: "/v1/projects/*/databases/*/query"   # 读类
           max: 300
           window: 10s
         - match: "/v1/projects/*/databases/*/execute" # 写类
           max: 60
           window: 10s
         - match: "/v1/auth/login"
           max: 10
           window: 1m
         - match: "/v1/projects/*/v1/chat/completions"
           max: 60
           window: 1m
       burst: 2                  # max 的倍数
       exempt_sse: true          # Agent run / realtime SSE 不计入
   ```
   命中返回 `429` + `Retry-After`，错误码 `rate_limited`，指标 `simplebase_api_rate_limited_total{rule}`。
5. **真实 IP**：`http.trusted_proxies: []string`（CIDR），非空时 `e.IPExtractor = echo.ExtractIPFromXFFHeader(TrustOption...)`；为空则用 `RemoteAddr`（现状）。写进审计的 IP 必须来自该解析结果。
6. **`X-Request-Id` 回写响应头**（若当前只在 body 中，补 header 便于前端/网关串联）。

#### 落点文件

| 文件 | 改动 |
|---|---|
| `internal/api/middleware_http.go`（新增） | CORS / gzip / securityHeaders / requestIDHeader |
| `internal/api/middleware_ratelimit.go`（新增） | token bucket + 规则匹配 + 指标 |
| `internal/api/router.go` | 插入顺序：requestID → 真实 IP → CORS → securityHeaders → Recover → accessLog → gzip → rateLimit → bodyLimit；CORS 必须在 Recover 之前（预检也要有头） |
| `internal/config/config.go`、`config.example.yaml` | `http.cors.*`、`http.trusted_proxies`、`limits.rate_limit.*` |
| `internal/observability/metrics.go` | `api_rate_limited_total` |
| `docs/ops/deployment.md` | 反向代理/浏览器直连配置章节 |

#### 验收

1. 预检 `OPTIONS` 返回正确 `Access-Control-Allow-*`，未允许 origin 不返回放行头。
2. SSE 响应无 `Content-Encoding`，代理链不改写（`curl -N` 可见逐帧）。
3. 触发限流返回 429 + `Retry-After`，且并发 1000 请求时进程内存与 goroutine 不增长（bucket 表按时间淘汰）。
4. YAML 里 `allowed_origins: ["*"]` + `allow_credentials: true` → 启动被 `Validate()` 拒绝（防呆）。
5. 反向代理头伪造：`trusted_proxies` 为空时 `X-Forwarded-For` 不生效。

---

## 4. P1 缺口详细设计

### 4.1 P1-1 细粒度 API Key 策略（替代「照抄 collection rules」）

**为什么不是照抄 PocketBase 规则引擎**：PB 的规则围绕「终端用户」构建（`@request.auth.*`、`@request.body.*`），SimpleBase 的主体是项目级 API Key 与管理员，行级表达式的实际用途有限；而「把 Key 安全下发给第三方/前端」才是真实需求。因此做 **Scoped API Key**，把 PB 的「限制谁能读写哪张表的哪些字段」目标用契合自身模型的方式实现。

存储（v54）：

```sql
CREATE TABLE IF NOT EXISTS sys_api_key_policies (
    key_id VARCHAR NOT NULL,
    project_id VARCHAR NOT NULL,
    resources_json VARCHAR NOT NULL DEFAULT '[]',  -- ["database:*","collection:orders","object:public/","function:report","cron:*"]
    actions_json VARCHAR NOT NULL DEFAULT '[]',    -- ["read","write","invoke","presign"]
    hidden_fields_json VARCHAR NOT NULL DEFAULT '[]', -- 响应中剔除的字段（如 ["secret","owner_email"]）
    readonly_fields_json VARCHAR NOT NULL DEFAULT '[]',
    expires_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL
);
```

- 无 policy 的 Key = 现行行为（完全向后兼容）。
- 校验在 `auth.Require` 之后、handler 之前：`require(auth.DatabaseRead, WithPolicy(...))` 或统一的新中间件 `policyGate`（资源从路由参数推断，动作由方法推断）。
- `hidden_fields` 在响应序列化阶段剔除（P0-1 的 `fields` 裁剪器复用）。
- 越权返回 403 `key_scope_denied`，不回显允许范围（防枚举）。

工作量 3–4 天。**落地前置**：P0-2 的字段注册表（否则「隐藏字段」无字段名可校验）。

### 4.2 P1-2 记录级实时订阅（SSE）

**范围明确**：只覆盖**文档 API 写路径**产生的事件。SQL 直写（`/query`、`/execute`、云函数直连库）**不产生事件**——这是单文件引擎 + 单写进程下的诚实边界，必须写进文档，不能让用户以为「改了库就能收到推送」。

端点：

```text
GET  /v1/projects/:p/realtime                       # SSE，首帧 {"event":"connect","client_id":"..."}
POST /v1/projects/:p/realtime/subscriptions         # {"collections":["orders"],"events":["create","update","delete"]}
```

事件帧：

```json
{"id":"42","event":"record.update","collection":"orders","record_id":"...",
 "action":"update","changed":["status"],"at":"2026-10-26T03:00:00Z"}
```

- 默认不带文档正文；`?include_data=1` 且 Key policy 允许时才带（防敏感数据外泄）。
- 实现复用 `internal/api/run_events.go` 的既有模式：进程内 hub + 环形缓冲（2000 帧）+ 单调 `id` + `replay(after)` 续传 + `gap` 错误帧 + 订阅者容量 16（满则丢弃并记指标）。
- 心跳 15s；空闲 5 分钟、最长 30 分钟（对齐 PB）；每项目连接上限 `limits.realtime_max_conns`（默认 200）。
- 权限：订阅前用与读接口相同的权限位校验；`POST subscriptions` 必须校验每个集合对当前 Key 可读。

新包 `internal/realtime/`（`hub.go`、`hub_test.go`）。写路径埋点：`CreateDocument`/`PatchDocument`/`UpdateDocument`/`DeleteDocument`/`BatchDocuments` 成功后 `hub.Publish`（失败不发布）。

工作量 5–6 天（含续传与背压测试）。

### 4.3 P1-3 文件能力

| 子项 | 设计 | 备注 |
|---|---|---|
| MIME 嗅探 | 服务端读前 512B 用 `http.DetectContentType`，与扩展名/声明类型交叉校验；`files.allowed_mime` 白名单（项目级设置） | 拒绝不一致时 415 `mime_mismatch` |
| 代理下载 | `GET .../s3/objects/content?key=` 流式转发（`Content-Length`/`ETag`/`Range` 支持），`local` 模式唯一可用路径 | 与 presign 并存 |
| 私有直链 | `POST .../s3/link` 签发 `{key,exp}` 的 HMAC token（`files.token_secret` 仅 env），`GET /files/{token}` 校验后流式返回；TTL 可配（默认 15m，上限 24h） | 复用 `hashx` + 常量时间比较 |
| 目录语义 | `GET .../s3/objects?prefix=x&delimiter=/` 返回 `{folders:[], objects:[]}`；前端据此做树 | 仍是 key 前缀，无真实目录 |
| 去重 | 可选：上传时按 SHA-256 查 `sys_s3_objects`，命中则复用物理对象（引用计数） | P2 评估，删除语义需引用计数 |
| 缩略图 | **需要引入 `golang.org/x/image`，与「不新增依赖」冲突**，见 §7 D4；不批准则降级为前端 Canvas 预览 | 若做，必须加像素上限防解压炸弹（PB 的教训） |

工作量：嗅探 + 代理下载 + 私有直链 + 目录语义 ≈ 4 天；缩略图另计 2–3 天（含依赖评审）。

### 4.4 P1-4 日志统计与设置中心

1. `GET /v1/projects/:p/logs/stats?from&to&group_by=level|hour|path`：在 `sys_log_events` 上聚合，返回 `[{key,count}]`；限额（时间窗 ≤ 30 天），指标不进 Prometheus（避免高基数）。
2. `GET /v1/projects/:p/logs/export?from&to&format=csv`：流式 CSV（字段固定，脱敏保持现状）。
3. `/console/settings` 从 redirect 改为真实页面（`SbTabsPanel`）：连接、模型、存储、日志与保留、文件白名单、备份、限流；`?settings=1` 保留为重定向兼容（`DefaultLayout` 现有逻辑删除）。
4. 日志页加趋势图（复用 `TrendChart.vue`）。

工作量 4 天（后端 1.5 + 前端 2.5）。

---

## 5. P2 缺口详细设计（按需排期）

| 编号 | 能力 | 要点 | 参考 | 估算 |
|---|---|---|---|---|
| P2-1 | 数据导入导出 | `POST .../data/collections/:c/import`（CSV/NDJSON，流式、逐批校验、可选 `dry_run`）；`GET .../export?format=csv|ndjson`（流式 + `Content-Disposition`）。上限 `limits.max_import_bytes`（默认 128MiB），超限 413 | `apis/batch.go` 的单事务思路 | 4d |
| P2-2 | 视图集合 + dry-run | `sys_collections.schema_mode='view'` + `view_query`；`POST .../collections/meta/dry-run-view` 用 `LIMIT 20` 试跑并推断列。仅允许单条 SELECT，交由 `sqlguard.Validate(ReadOnly)` + 多语句拦截 | `core/view.go:82,138` | 4d |
| P2-3 | 事件触发器（DB/文件 → 云函数） | `sys_function_triggers`（v55）：`kind=record_change|object_event|http`、`target`（集合/前缀）、`events`、`function`、`filter`、`enabled`。写路径 → 进程内队列 → 调用 `gofunction`（并发上限复用 `max_concurrent_go_runs`，失败重试 3 次退避，落 `sys_go_func_invokes`）。**不覆盖 SQL 直写** | PB `core/base.go` hook 体系 | 6–8d |
| P2-4 | 迁移工具 | `ApplySystemMigrations` 增加 `Status()`/`Plan()`（只读）；CLI `simplebase migrate status`；**不做 down**（DuckLake DDL 不可回滚，改为「迁移前自动快照」，复用 P0-3） | `core/migrations_runner.go`、`plugins/migratecmd` | 2d |
| P2-5 | OpenAPI / API 预览 | 路由元数据表（`internal/api/route_meta.go`，与 `router.go` 同源，测试锁定一致性）→ `GET /v1/openapi.json`；控制台新增 API 预览页（端点 + cURL/JS/Go 片段 + 试跑） | `ui/src/apiPreview/*` | 5d |
| P2-6 | 数据浏览器 UI | `/console/data` 从 redirect 改为真实页面：集合侧栏 + 记录表格（filter 输入带字段补全、排序、列显隐、行内编辑、导出）+ Schema 设计器（字段表单/类型/校验/顺序）+ `dry_run` 差异确认 | `recordsList.js`、`collectionUpsertModal.js` | 10–12d |

---

## 6. 明确不采纳的能力（含理由）

| # | PocketBase 能力 | 不采纳理由 |
|---|---|---|
| 1 | goja JS hooks（`plugins/jsvm`） | 已有 Go 云函数：编译期类型安全、性能更好、版本不可变可回滚。再引入一套 JS 运行时只会分散扩展模型。 |
| 2 | auth collection（终端用户体系：`auth-with-password`/OAuth2/OTP/MFA/邮箱验证/密码重置） | 这是**产品决策**（§7 D1），不是技术缺口：SimpleBase 现在是「开发者/运维平台」，没有「应用终端用户」概念。若做，需要 SMTP、邮件模板、终端用户表、per-project token 秘钥，是一整块新子系统。 |
| 3 | 30+ OAuth2 provider（`tools/auth`） | 依赖 2；且多租户下每项目托管第三方 client secret 的密钥管理成本高。 |
| 4 | 自更新（`plugins/update`、`ghupdate`） | 部署形态是镜像 + 滚动升级，二进制自更新与不可变镜像冲突。 |
| 5 | 内置 ACME/TLS、www→non-www 重定向 | TLS 在 Ingress/反代终止（`docs/ops/deployment.md`），进程内 ACME 会让证书与网关冲突。 |
| 6 | 多实例 `notify_watcher`（`.notify` 目录跨进程同步） | SimpleBase 是「单写实例」模型；多副本属架构级变更（见 `README.md` 部署约束），不在能力补齐范畴。 |
| 7 | 迁移 `down`/回滚 | DuckLake DDL 逐语句执行、无事务保证，down 脚本极易造成半迁移。替代方案：迁移前自动快照（P0-3）+ `migrate status`（P2-4）。 |
| 8 | superuser 独立集合 + `impersonate` | 已有三层角色与 `sys_users`；模拟登录会绕过审计主体归属，与「审计可追溯到发起人」的现有约束冲突。 |
| 9 | 管理台 UI 扩展注入（`extensions.js`） | Vue 控制台不开放第三方脚本注入（XSS 面），需要扩展就走云函数或独立前端。 |
| 10 | 首次安装向导（installer + 30 分钟 token） | 已有 bootstrap 超管 + `must_change_password`，容器化环境下开浏览器不适用。 |
| 11 | Batch API 的 multipart 混排（`requests.N.file`） | 文件走独立上传端点 + `key` 引用更清晰；混排会让 body limit 与错误定位复杂化。 |
| 12 | `VACUUM INTO` 单一备份实现 | 双引擎，必须用引擎级 Snapshotter（P0-3）；`VACUUM INTO` 只作为 SQLite 分支实现。 |

---

## 7. 待决策项（需要产品/架构拍板）

| 编号 | 决策 | 选项 | 影响 |
|---|---|---|---|
| D1 | 是否做「项目级终端用户」（PB auth collection 等价物） | A 不做（推荐，本期）；B 只做最小集（邮箱+密码+验证+重置，无 OAuth/MFA）；C 全量 | 决定 §2.3 的 8 项 ❌ 是否变任务；B 约 12–15d，还需 SMTP（§2.6 #7） |
| D2 | 是否做行级规则表达式（`@request.auth` 风格） | A 不做（推荐，用 P1-1 Scoped Key 覆盖 90% 场景）；B 做受限版（字段级 + 主体级） | B 需先有 D1 的主体概念 |
| D3 | 系统库是否纳入备份 | A 不纳入（本期，靠 S3 版本控制）；B 纳入（DuckLake catalog + parquet 快照，8–10d） | 影响 P0-3 范围 |
| D4 | 缩略图是否引入 `golang.org/x/image` | A 不引入（前端 Canvas 降级）；B 引入（需批准「不新增依赖」例外） | 影响 P1-3 工期 |
| D5 | 集合是否改「类型化物理列」 | A 保持 JSON 列 + 影子列索引（推荐）；B 直接类型化列（PocketBase 式，需重建表迁移） | 决定 P0-2 的长期形态与索引方案 |

**建议**：D1=A、D2=A、D3=A、D4=A、D5=A。即本期只补「平台数据面」与「安全/运维底线」，不把 PocketBase 的 BaaS 子系统整体搬进来。

---

## 8. 分阶段实施清单

### 阶段 0：契约冻结（0.5d）

- [x] 评审并冻结 P0-1 的查询参数与响应形状（含 `rows` 兼容期）。
- [x] 冻结 filter 语法子集与错误码表（`invalid_filter` / `unknown_field` / `filter_scan_limit_exceeded` / `field_type_mismatch` / `duplicate_value` / `schema_not_registered`）。
- [x] 冻结 `sys_collections` / `sys_collection_fields` / `sys_backups` 字段（§9）。
- 验证：文档评审通过；无代码改动。

### 阶段 A：HTTP 边界（P0-4，2–3d）

- [x] CORS / gzip / securityHeaders / requestID header 中间件与配置。
- [x] token bucket 限流 + 指标 + 429 协议。
- [x] trusted proxy 与 IP 解析接入审计。
- 验证：`go test ./internal/api/... ./internal/config/...`；新增中间件单测 + 路由顺序测试。

### 阶段 B：集合元数据 + 文档数据面（P0-2 → P0-1，8–10d）

- [x] v51/v52 迁移定义与 `internal/collection` 领域包（8 类型校验）。
- [x] `meta` 四端点 + 缓存 + `dry_run`。
- [x] `Dialect` 扩展 JSON 函数 + 两引擎实现。
- [x] 查询器（分页 keyset/offset、filter、sort、fields、count）与 5 个新文档端点。
- [x] SDK/CLI/Agent 跟进（`js-sdk`、`go-sdk`、`sbcli document list --filter`、gocloudagent 只读工具）。
- 验证：`go test ./internal/api ./internal/collection ./internal/database/... ./internal/sbcli`；`cd packages/js-sdk && npm test`。

### 阶段 C：备份与恢复（P0-3，5–7d）

- [x] `Snapshotter`（duckdb `COPY FROM DATABASE` / sqlite `VACUUM INTO`）+ 一致性测试。
- [x] 备份服务、保留策略、S3 双写、定时器。
- [x] 恢复状态机（关闭 → 校验 → 原子替换 → 重开 → 失败回滚）。
- [x] API + CLI + 审计。
- 验证：两引擎 × local/S3 各跑一遍备份恢复；kill -9 中断恢复测试。

### 阶段 D：P1 组（10–14d）

- [x] D-1 Scoped API Key（v54 + `policyGate` + 字段隐藏）。
- [x] D-2 `internal/realtime` + SSE 端点 + 写路径埋点 + 续传。
- [x] D-3 文件：MIME 嗅探、代理下载、私有直链、目录语义。
- [x] D-4 日志统计/导出 + 设置中心页（`/console/settings` 真实化）。

### 阶段 E：P2 组（按需，逐项独立评审）

- [x] 导入导出 → 视图集合 → 事件触发器 → 迁移 status → OpenAPI/API 预览 → 数据浏览器 UI。

---

## 9. 系统表变更清单（v51–v55）

在 `internal/systemdb/migrate.go` 的 `systemMigrations` 追加（**只定义，不在本计划中执行**；生效方式与 v50 一致：由用户以正常可写实例启动时应用，或用户手动执行 DDL）。

| 版本 | 名称 | 用途 | 阶段 |
|---|---|---|---|
| v51 | `sys_collections` | 集合元数据（name/schema_mode/status） | B |
| v52 | `sys_collection_fields` | 字段元数据（type/required/unique/options/hidden/readonly/order） | B |
| v53 | `sys_backups` | 备份索引（id/project/对象key/大小/sha256/engine/status/created_at） | C |
| v54 | `sys_api_key_policies` | Scoped Key 策略 | D |
| v55 | `sys_function_triggers` | 事件触发器 | E |

```sql
CREATE TABLE IF NOT EXISTS sys_backups (
    id VARCHAR NOT NULL,
    project_id VARCHAR NOT NULL,
    storage_key VARCHAR NOT NULL,
    size_bytes BIGINT NOT NULL,
    sha256 VARCHAR NOT NULL,
    engine VARCHAR NOT NULL,
    format_version BIGINT NOT NULL DEFAULT 0,
    status VARCHAR NOT NULL DEFAULT 'ready',   -- creating|ready|failed|restoring
    error VARCHAR NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP
);
```

约定：
- 不建 PK / UNIQUE（与现有 `sys_*` 一致），唯一性由单写实例 + 应用层保证。
- 索引能力受限，`sys_collections`/`sys_backup` 的查询都带 `project_id` 且数据量小，全扫可接受。
- 本计划**不执行** `init_db`、不执行任何 `ALTER`/`DDL`；以 `TIMER_NO_RUN=1`（纯 API 模式）启动时迁移不会自动生效，需用户手动运行。

---

## 10. 关键测试矩阵

| 维度 | 用例 |
|---|---|
| 查询 | 两引擎 × 8 类型 × 8 运算符；NULL 排序一致性；cursor 翻页不重不漏；`fields` 含嵌套与非法字段 |
| 注入 | 字段名/值/排序字段/filter 长度/like 元字符/分号/NUL；SQL 日志中无原始值 |
| 集合元数据 | 注册/覆盖/注销；`dry_run` 不改数据；旧集合宽松模式回归；缓存失效 |
| 写入校验 | required/type/pattern/select/max/unique；批量回滚定位；未知字段拒绝 |
| 备份 | 两引擎 × local/S3；并发写排队；SHA 破坏；kill -9 恢复中断；保留策略三参数 |
| 中间件 | CORS 预检与非法 origin；SSE 不被 gzip；限流 429 + Retry-After + 内存不涨；XFF 伪造 |
| Realtime | 订阅/取消/续传/`gap`/背压丢弃/连接上限/权限拒绝 |
| 文件 | MIME 不一致 415；代理下载 `Range`；私有 token 过期与篡改；目录分隔 |
| 权限 | 只读实例 503（新写路由全覆盖，`TestWriteGate_AllWriteRoutesBlockedOnReadOnly` 驱动）；Scoped Key 越权 403；系统库写拒绝 |
| UI | `yarn build`、`yarn test`（coverage 阈值）、i18n 两棵树 key 一致、reka `data-[state=…]` 状态类 |

最终验证命令：

```bash
go build ./internal/... ./cmd/...
go test ./...
(cd gofunction && go test ./... -short)
(cd gosandbox && go test ./...)
(cd gocloudagent && GOWORK=off go test ./...)
cd ui && yarn build && yarn test
cd packages/js-sdk && npm run typecheck && npm test
```

---

## 11. 验收标准

1. 集合可声明字段与校验规则；非法文档 100% 被拒，错误码与字段名可定位。
2. 列表 API 支持 `limit/cursor/offset/sort/filter/fields/count`，100 万文档下翻页正确、无全量返回。
3. filter/sort 只接受注册字段，注入尝试全部 400，且日志/审计无原始值。
4. 项目可备份、可下载、可恢复；恢复失败不影响原数据；保留策略生效。
5. 浏览器跨域、压缩、限流、安全头、真实 IP 五项在部署文档中有明确配置样例并通过验证。
6. 现有 API/SDK/Agent/CLI 行为零回归（`rows` 兼容期保留）。
7. 新增写路由全部挂 `writeGate` 并有测试覆盖；系统库仍不可写不可删。
8. 不新增第三方依赖（除 §7 D4 明确批准的例外）。
9. 未引入自动 DDL：迁移只定义，由用户按既有流程执行。
10. 文档站（`docs/`）与 `packages/*-sdk` 同步更新，`ui/tests/docs-order.test.ts` 通过。

---

## 12. 风险登记

| 风险 | 影响 | 缓解 |
|---|---|---|
| JSON 列 filter 全表扫描 | 单写实例被拖慢 | `max_filter_scan_rows` 上限 + 显式错误码；文档给出「收窄条件/物化列」建议 |
| 备份快照占用写租约 | 大库出现秒级写延迟 | 快照限时（`snapshot_timeout`）+ 定时窗口 + 文档说明；必要时先降级为「只读窗口」 |
| 恢复中断导致项目不可用 | 数据不可用 | `.restore-old` 双阶段 rename + 启动时自愈扫描 |
| 元数据与物理表漂移（用户手写 DDL） | 查询编译/校验失真 | `GET .../meta` 附带 `drift` 提示；`schema/show` 与注册表差异在 UI 标红 |
| 新增中间件改变现有行为 | 回归 | 全部默认关闭（CORS/gzip/限流），默认开启的只有安全头与 `X-Request-Id` |
| 迁移 v51+ 未生效（纯 API 模式） | 新功能 500 | 启动时检测缺失表 → 对应能力降级为「未启用」并给出提示，不 panic |
| 实时订阅内存增长 | OOM | 环形缓冲 + 订阅者容量 + 连接上限 + 指标告警 |

---

## 13. 工作量估算

| 阶段 | 内容 | 估算 |
|---|---|---|
| 0 | 契约冻结 | 0.5d |
| A | HTTP 边界 | 2–3d |
| B | 集合元数据 + 文档数据面 | 8–10d |
| C | 备份与恢复 | 5–7d |
| D | P1 组（Key 策略/realtime/文件/日志设置） | 10–14d |
| E | P2 组（6 项） | 30–36d（可按需裁剪） |
| — | 合计（P0+P1） | **26–35d** |

---

## 14. 附录：证据索引

### PocketBase（相对 `thirdparty/pocketbase/`）

| 主题 | 文件 |
|---|---|
| 路由与中间件装配 | `apis/base.go:19-57` |
| 集合模型 / 校验 / 表同步 | `core/collection_model.go`、`core/collection_validate.go`、`core/collection_record_table_sync.go` |
| 字段类型与修饰符机制 | `core/field.go:67-185`、`core/field_*.go` |
| filter/sort/分页 | `tools/search/filter.go:40`、`sort.go`、`provider.go:56,105-230` |
| 字段裁剪 | `tools/picker/pick.go:31`、`picker/modifiers.go` |
| 关联展开 | `core/record_query_expand.go:24-65` |
| 视图集合 | `core/view.go:82,138` |
| 集合导入导出 | `core/collection_import.go:19,37`、`apis/collection_import.go` |
| 记录 API | `apis/record_crud.go:29-33` |
| 认证 | `apis/record_auth.go:22-68`、`core/record_tokens.go`、`core/mfa_model.go`、`core/otp_model.go` |
| OAuth2 | `apis/record_auth_with_oauth2*.go`、`tools/auth/*` |
| 实时 | `apis/realtime.go:37-91,168-173,856`、`tools/subscriptions/*` |
| Batch | `apis/batch.go:28,101-143` |
| 文件 | `apis/file.go:44-45,115,165`、`core/field_file.go:104-138,327-400`、`tools/filesystem/filesystem.go:608-736` |
| 备份 | `apis/backup.go:19-24`、`core/backup_create.go:67`、`core/backup_restore.go` |
| 日志 | `apis/logs.go:19-22`、`core/log_model.go`、`ui/src/logs/logsChart.js` |
| Settings | `core/settings_model.go:122-185,265-275,356-558`、`apis/settings.go:14-21` |
| 限流 / CORS / gzip / 安全头 | `apis/middlewares_rate_limit.go`、`middlewares_cors.go`、`middlewares_gzip.go`、`middlewares.go:288` |
| TLS/ACME | `apis/serve.go:134-150` |
| 迁移 | `core/migrations_list.go:9-82`、`core/migrations_runner.go:14-110`、`plugins/migratecmd/*` |
| JS 扩展 | `plugins/jsvm/jsvm.go:61-109`、`binds.go` |
| 控制台 | `ui/src/router.js:129-174`、`ui/src/collections/*`、`ui/src/records/*`、`ui/src/apiPreview/*`、`ui/src/settings/**` |

### SimpleBase（相对仓库根）

| 主题 | 文件 |
|---|---|
| 路由与守卫 | `internal/api/router.go:212-290,308-331,400-470` |
| 文档 API 现状 | `internal/api/data_handler.go:102-134,303-306` |
| Schema / 行读取 | `internal/api/schema_handler.go:195-204`、`internal/api/schema_rows.go` |
| SQL 边界 | `internal/database/sqlguard/classifier-desens.go:22-66` |
| 方言接口 | `internal/database/dialect.go:12-36` |
| 认证与角色 | `internal/auth/principal.go`、`role.go:31-35`、`session.go`、`api_key_repository.go` |
| 系统库迁移 | `internal/systemdb/migrate.go`（v1–v50） |
| 单文件库（前置工作） | `plan/planv6.2/project-single-file-database-plan.md` |
| 快照手法先例 | `internal/database/ducklake/catalog_syncer.go:564-592` |
| 事件 hub 先例 | `internal/api/run_events.go` |
| 文件存储 | `internal/objectstore/filestore.go`、`localfilestore.go`、`internal/api/s3_handler.go:118-215` |
| 配置 | `config.example.yaml`、`internal/config/config.go` |
| 部署与备份现状 | `docs/ops/deployment.md`（L265-269 备份 API 已移除） |
| 前端路由与页面 | `ui/src/router/index.ts`、`ui/src/pages/*` |
