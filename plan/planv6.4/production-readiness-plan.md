# SimpleBase 生产就绪：让「demo 能跑」变成「敢上线」（planv6.4 下一迭代）

> **状态：** 规划（未实施）；2026-11-09 完成第一批细化（§8：入口层 / 身份层 / 运维层）
> **日期：** 2026-11-08（初稿）、2026-11-09（§1.1 / §1.2 / §1.6 按代码复核修订，新增 §8）
> **关联：** `plan/planv6.3/pocketbase-capability-gap-plan.md`（能力差距，D1–D5 决策）、`plan/planv6.4/examples-skill-verification-report.md`（BUG-0~5、权限层级、删库语义）、`docs/ops/deployment.md`（安全基线）、`docs/sdk/auth.md`（浏览器警告）

---

## 0. 问题判断

AI 编码让「做出 demo」的成本接近零：用 `skills/simplebase` 几分钟就能生成迁移、云函数、定时任务并 `dev apply`。但 demo 和线上服务之间隔着一整层**没人会主动写、也不会在 demo 里暴露**的基础设施：

| demo 阶段不会遇到 | 上线第一周就会遇到 |
| --- | --- |
| 自己一个人用，Key 写在 `.env` | 浏览器要直连，Key 被打包进前端 |
| 数据是种子数据 | 真实用户数据误删、要恢复到昨天 |
| 云函数调本机 | 函数被诱导请求内网 `169.254.169.254` |
| 请求量个位数 | 爬虫、爆破、单个用户打满单写实例 |
| 出错看终端日志 | 半夜挂了没人知道，第二天用户反馈 |
| 只有一个环境 | 开发改动直接打到生产库 |

**目标用户画像**：不懂运维的独立开发者 / 小团队，用 AI 生成业务代码，期望「一个二进制 + 一个域名」就能上线。他们不会读 `deployment.md` 的安全基线，也不会自己配 Ingress 限流。

**设计原则**：

1. **安全默认值，而不是安全文档**：生产模式下不安全的配置直接拒绝启动或在控制台标红，不依赖用户读文档。
2. **一个开关切到生产**：`instance.profile: production` 统一收紧默认值，开发模式保持现在的零配置体验。
3. **补「平台必须有、用户写不出来」的东西**：终端用户认证、行级访问、出网控制、备份演练、告警。业务功能不在本计划范围。
4. **沿用现有边界**：单写实例、每项目单库、系统库只读、配置只来自 YAML + 环境变量、不新增第三方依赖（需要例外的单独列为决策项）。

---

## 1. 现状盘点（2026-11-08，基于代码核实）

按「请求从外到内」的七层逐层检查。✅ 已具备 / ⚠️ 有但默认关闭或不完整 / ❌ 缺失。

### 1.1 入口层（网络与 HTTP）

| 能力 | 现状 | 证据 | 判定 |
| --- | --- | --- | --- |
| TLS | 不在进程内终止，依赖反代/Ingress | `docs/ops/deployment.md`；planv6.3 §6 明确不做 ACME | ⚠️ 对小白是空白 |
| 安全响应头 | 有 `nosniff`、`X-Frame-Options`、`Referrer-Policy`、`Permissions-Policy`；**无 HSTS、无 CSP** | `internal/api/middleware_http.go:securityHeadersMiddleware` | ⚠️ |
| CORS | 有，**默认关闭**，禁止无条件 `*` | `config.example.yaml http.cors` | ✅（设计正确） |
| 全局限流 | 有令牌桶，**默认关闭**，规则为空 | `limits.rate_limit.enabled: false` | ⚠️ |
| 登录防爆破 | 有，按 IP+用户名限速返回 429；**无账号锁定、无告警** | `auth_handler.go:123` | ⚠️ |
| 真实 IP | 有 `trusted_proxies`，为空时忽略 XFF | `http.trusted_proxies` | ✅ |
| `/metrics` | **无鉴权**，与业务端口同一监听 | `router.go:312` | ❌ 生产暴露面 |
| `/debug/pprof` | 仅 `PerfStageTiming` 开启时挂载，**无鉴权** | `router.go:280-287` | ⚠️ |
| 请求体上限 / 超时 | 有（1MiB 默认、查询 30s、并发 64） | `limits.*` | ✅ |

**2026-11-09 复核补充**（细化方案见 §8.1）：

| 发现 | 证据 | 影响 |
| --- | --- | --- |
| 登录限速器 `loginLimiter` 是 handler 私有的切片窗口，与全局令牌桶 `rateLimiter` 是两套实现；键只有 `IP/用户名`，换用户名即可绕过对单 IP 的总量限制 | `auth_handler.go:277-307`、`middleware_ratelimit.go` | 撞库（固定密码换用户名）不受限 |
| `trusted_proxies` 非空时用 `echo.ExtractIPFromXFFHeader(TrustIPRange...)`，echo 默认还**额外信任回环 / 链路本地 / 私网**（`newIPChecker` 默认三项为 true） | `middleware_http.go:73-85`、echo `ip.go:175` | 同 VPC 内任意主机可伪造 XFF 绕过按 IP 限流 |
| 全局限流 `key()` 把 `APIKeyID+UserID+RealIP` 拼在一起，规则只能「每身份每 IP」，无法表达「每 IP 总量」或「每 Key 总量」 | `middleware_ratelimit.go:92-104` | 生产内置规则无法表达（§3.1 的「`/go` 按 Key 600/min」） |
| 限流中间件挂在 auth 之前（`e.Use`），此时 `PrincipalFromContext` 恒为空，`id` 永远是 `""` | `router.go:294` 在 `mountV1Routes` 之前 | 现有「按身份」键实际只按 IP 生效 |
| 用户文件下载 `Content-Disposition: inline`，与控制台同源 | `s3_content_handler.go:213` | 上传的 SVG / HTML 若被嗅探放行可在控制台源执行脚本（嗅探已拦 `text/html`，SVG 需核实） |
| 控制台构建产物无内联脚本（`index.html` 只有一个 `type=module` 外链），CSP 可直接用 `script-src 'self'` | `internal/web/dist/index.html` | CSP 落地成本低 |

### 1.2 身份层（谁在调用）

| 能力 | 现状 | 判定 |
| --- | --- | --- |
| 平台用户（控制台） | argon2id、refresh 会话（轮转 + 重放检测）；「首次登录强制改密」**未生效**，见下方复核 | ⚠️ |
| 引导账号 | `simplebase2026/simplebase2026` 固定写在代码，且未置 `must_change_password` | ❌ 公网实例长期可用默认口令登录超管 |
| API Key | HMAC 存储、可吊销、Scoped（资源/动作/字段/过期）、权限层级 | ✅ |
| `api_key_hash_secret` 轮换 | **一换全部 Key 失效**，无版本化 | ❌ |
| **项目终端用户**（应用的用户注册/登录） | **没有**。只有平台用户和 API Key | ❌ **最大缺口** |
| MFA / SSO（控制台） | 无 | ❌ |
| 项目成员与角色 | 只有 owner（`sys_project_owners`） | ❌ |

**后果**：小白做一个「用户能登录、只能看自己数据」的应用，只有两条路——①前端带只读 Key（所有人看到所有数据），②自己写 BFF 再实现一套账号体系。AI 生成的 demo 几乎都会走①，`docs/sdk/auth.md` 的「浏览器警告」就是在描述这个坑。

**2026-11-09 复核补充**（细化方案见 §8.2；上表「引导账号 ⚠️」需改判为 ❌）：

| 发现 | 证据 | 影响 |
| --- | --- | --- |
| **引导账号实际上没有被强制改密**：`EnsureBootstrapUser` 建用户时未设置 `MustChangePassword`（零值 false），全仓也没有任何地方把它置 true；「首次登录强制改密」只是文档描述 | `auth/user.go:446-478`、`docs/getting-started/quickstart.md:22` | 公网实例永久可用 `simplebase2026/simplebase2026` 登录超管，**不是窗口期问题** |
| **强制改密流程是死路**：`Login` 遇 `must_change_password` 返回 423 且不发 token；`PUT /v1/auth/password` 挂在需认证的 `/v1` 组下；`IssueAfterPasswordChange` 无调用方；前端没有处理 423 | `session.go:209-230`、`router.go:391`、`ui/src/services/http-api.ts` | 管理员给用户重置密码（`Update` 会置 `MustChangePassword=true`）后，该用户再也登不进来 |
| 会话 JWT、委托令牌、envd 令牌都直接或派生自 `api_key_hash_secret` | `app.go:663-664`、`app.go:1128`、`e2b_auth.go:227` | 轮换该密钥 = 所有 Key + 所有会话 + 所有委托同时失效；密钥职责过载 |
| `sys_api_keys` 只有 `id/project_id/key_hash/permissions/created_at/revoked_at`：无名称、无前缀、无 `last_used_at`、无密钥版本 | `systemdb/schema.go:69-76` | 无法在列表中辨认是哪把 Key；无法做「90 天未使用」；无法版本化轮换 |
| 登录失败无审计、无指标；只有内存限速 | `auth_handler.go:113-141` | 爆破发生了也看不到 |
| 口令规则只有长度 ≥ 8 | `auth/user.go:177-182` | 允许 `12345678`、允许与用户名相同 |

### 1.3 数据访问层（能访问哪些行）

| 能力 | 现状 | 判定 |
| --- | --- | --- |
| 项目隔离 | 跨项目 403，物理文件隔离 | ✅ |
| 资源 / 字段级 | Scoped Key（`collection:orders`、`hidden_fields`、`readonly_fields`） | ✅ |
| **行级访问控制** | 无（文档明确「不是行级 RLS」） | ❌ |
| SQL 边界 | `sqlguard`：拒绝清单、多语句、只读检测、超时、行数 | ✅ |
| 系统库保护 | 只读、禁删 | ✅ |

### 1.4 代码执行层（云函数 / 沙盒 / 助手）

| 能力 | 现状 | 判定 |
| --- | --- | --- |
| 云函数并发 | 共享上限 16，满载 503 | ✅ |
| 云函数超时 | 有 `ErrTimeout → 504` | ✅ |
| **云函数出网** | `net/http` 真实发请求，**无出网白名单、无私网/元数据地址拦截** | ❌ SSRF |
| **函数密钥** | 无 secrets / 环境变量机制；第三方 Key 只能写进源码 | ❌（`dev validate` 只能拦工作区里的 `sb_live_`/`sk-` 等，控制台直接保存的源码拦不到） |
| 函数内存 / CPU 上限 | 未见每次调用的内存上限（待核实解释器侧） | ⚠️ |
| 沙盒网络 | `network: none` | ✅ |
| 云助手 | 破坏性命令确认、只读 fail-closed、委托令牌降权 | ✅ |

### 1.5 数据持久层（丢不丢）

| 能力 | 现状 | 判定 |
| --- | --- | --- |
| 在线持久 | S3 模式快照 + manifest，同步间隔 15s | ✅（RPO≈15s） |
| 备份 | 有完整 API 与定时，**默认关闭**，默认本地目录 | ⚠️ |
| 恢复演练 | 无自动校验「备份能不能恢复」 | ❌ |
| 系统库备份 | 不纳入（planv6.3 D3=A，靠 S3 版本控制） | ⚠️ 本地模式下系统库无保护 |
| 删库 | 已改为只清空用户数据，KV/备份保留（BUG-2） | ✅ |
| 迁移前快照 | 无；远端 `dev apply` 直接执行迁移 | ❌ |
| 软删除 / 回收站 | 无 | ❌ |

### 1.6 运维层（出事知不知道）

| 能力 | 现状 | 判定 |
| --- | --- | --- |
| 指标 | Prometheus | ✅（但无鉴权，见 1.1） |
| 运行日志 | 系统库 logs，检索 / 导出 / 保留期 | ✅ |
| 审计 | 函数、cron、助手、沙盒、备份、LLM 有记录；**API Key 签发/吊销、用户管理、项目设置变更未见审计** | ⚠️ |
| **告警 / 通知** | 无（无邮件、无 webhook 外发） | ❌ |
| 健康检查 | live / ready | ✅ |
| 触发器可靠性 | 内存队列，满即丢、重启丢 | ⚠️ 文档已说明，但小白不会读 |
| 高可用 | 单写实例，副本数 1 | ⚠️ 设计边界 |

**2026-11-09 复核补充**（细化方案见 §8.3）：

| 发现 | 证据 | 影响 |
| --- | --- | --- |
| 审计只有 6 处写入点：函数、cron、助手 run、沙盒、备份、LLM；`APIKeysHandler`、`UsersHandler`、`settingsHandler`、`settingsStorageHandler`、`logs retention`、`CollectionMetaHandler`、项目创建**都没有** `audit.Record` | `rg "audit.Record\|AuditEvent{"` | 签发 / 吊销 Key、加管理员、改存储都不留痕 |
| 审计表 `sys_operations` 没有 `detail` 列，`Record` 把 `Detail` 拼进 `status`（`"ok: xxx"`）；也没有 `actor_kind`、`ip`、`target` | `audit/service.go:82-85`、`schema.go:101-110` | 不能按「谁 / 对什么」检索；`status` 不再是枚举 |
| 审计 PrincipalID 口径不一：备份用 `principal.APIKeyID`（登录态为空 → `system`），沙盒 / 函数用 `actorID()`（UserID 优先） | `backup_handler.go:146`、`gofunction_handler.go:814` | 控制台操作在备份审计里显示成 `system` |
| `/health/ready` 只看系统库 Ping 与 S3 Check，不反映 catalog 同步滞后、租约、备份、日志缓冲丢弃 | `app.go:145-171` | 「活着但在丢数据」时探针仍是绿的 |
| 已有可直接复用的「坏事」信号，但都只进 Prometheus 或内存计数，无人消费：`CatalogSyncFailures`、`LeaseLost`、`CatalogSyncConflicts`、`APIRateLimited`、`trigger.Dispatcher.Dropped()`、`systemdb.droppedLog`、`backup.Scheduler.RunOnce` 的错误（被 `_ =` 丢弃） | `observability/metrics.go`、`trigger/dispatcher.go:127`、`systemdb/logs.go:90`、`backup/scheduler.go:64` | 告警的事件源已经有了，只差「汇总 → 判定 → 外发」 |
| 备份定时失败只是被吞掉，无日志、无指标 | `backup/scheduler.go:64` | 备份连续失败一个月也无人知道 |

### 1.7 交付层（怎么上线、怎么改）

| 能力 | 现状 | 判定 |
| --- | --- | --- |
| 本地 → 云端重放 | `simplebase dev` + skill，幂等、远端需 `--yes` | ✅ |
| 环境分离 | 靠不同实例 + 同 project ID | ✅ |
| 前端托管 | 无（只托管自带控制台） | ❌ |
| 自定义域名 / 证书 | 无（依赖外部反代） | ❌ |
| 上线前自检 | `sbdev.py doctor` 只查工作区与凭据，不查实例安全配置 | ❌ |
| 镜像 | 非 root（`USER ubuntu`） | ✅ |
| 依赖漏洞扫描 / SBOM | CI 未见 | ❌ |

### 1.8 合规与数据生命周期

| 能力 | 现状 | 判定 |
| --- | --- | --- |
| 项目删除 | **无 API**（只有建项目） | ❌ |
| 项目数据导出 | 集合可导出，无「整个项目打包导出」（备份下载可部分替代） | ⚠️ |
| 静态加密 | S3 依赖 SSE-KMS（部署方配置）；本地盘无 | ⚠️ |
| LLM 凭据存储 | 项目级 LLM API Key **原文**存于系统库 `sys_llm_settings.api_key`（`internal/systemdb/llm.go:68` 注释「APIKey 为原文」），对外脱敏；系统库备份 / S3 快照里同样是明文 | ❌ |

---

## 2. 差距排序

用「小白上线后**多快会出事** × **出事有多严重** × **用户能否自己补**」三个维度打分，得出：

| 优先级 | 缺口 | 为什么排这里 |
| --- | --- | --- |
| **P0** | 生产模式 + 上线自检（§3.1） | 成本最低、覆盖面最大：把已有但默认关闭的能力（限流、备份、CORS、安全头）一次收紧 |
| **P0** | 终端用户认证 + 行级规则（§3.2） | 不补，所有「有用户的应用」都只能把 Key 放前端；这是 AI demo 走向上线的第一道坎 |
| **P0** | 云函数出网控制 + 函数密钥（§3.3） | SSRF 可以直接拿到云厂商元数据凭据；密钥写进源码会进入版本记录和备份 |
| **P0** | 观测端点收口（§3.4） | `/metrics`、`/debug/pprof` 公网暴露是低成本信息泄露 |
| **P1** | 数据安全网：备份默认开 + 恢复演练 + 迁移前快照 + 回收站（§3.5） | 第一次误操作通常发生在上线一个月内 |
| **P1** | 告警与通知（§3.6） | 没有告警，前面所有防护出问题都没人知道 |
| **P1** | 审计补全 + 密钥轮换（§3.7） | 出事后要能回答「谁、什么时候、改了什么」 |
| **P2** | 前端托管 + 自定义域名（§3.8） | 让「一个二进制上线」真正闭环；外部反代可以替代 |
| **P2** | 项目删除 / 导出 / 成员角色（§3.9） | 合规与团队协作，规模起来后才迫切 |
| **P2** | 触发器持久队列（§3.10） | 已有文档说明边界，影响面相对小 |

---

## 3. 方案设计

### 3.1 P0-A 生产模式与上线自检

**目标**：一个配置项把实例切成「安全默认值」，不满足的项启动失败或在控制台标红。

**配置**：

```yaml
instance:
  profile: development   # development | production。SIMPLEBASE_INSTANCE_PROFILE
```

**`production` 下的规则**（分「拒绝启动」与「警告」两级）：

| 检查项 | 级别 | 规则 |
| --- | --- | --- |
| 引导账号 | 拒绝 | 存在未改密的 `simplebase2026` 时拒绝对外监听；改为要求 `SIMPLEBASE_BOOTSTRAP_PASSWORD` 首次注入，或只在 `127.0.0.1` 上完成首登 |
| `api_key_hash_secret` | 拒绝 | 长度 < 32 字节或等于示例值 |
| 限流 | 默认开 | `rate_limit.enabled=true`，内置规则：登录 10/min、注册 5/min（§3.2）、`/go` 按 Key 600/min |
| 安全头 | 默认开 | 追加 `Strict-Transport-Security`（`trusted_proxies` 非空或 TLS 时）、控制台路由 CSP |
| `/metrics`、pprof | 默认收口 | 见 §3.4 |
| 备份 | 警告 | `backup.enabled=false` 或 `dir` 不在持久卷（无法判断时提示） |
| CORS | 警告 | 开启且 `allowed_origins` 含通配 |
| 云函数出网 | 默认收紧 | `functions.egress.mode=allowlist`（§3.3） |
| 存储模式 | 警告 | `storage.database=local` 且未开备份 |
| 日志 | 警告 | `agent.trace_log=true` |

**自检入口**（三处同一份规则，规则定义只写一次，放 `internal/config` 或新包 `internal/readiness`）：

- 启动时：拒绝项直接退出，警告项打一条结构化日志。
- `GET /v1/system/readiness`（超管）：返回每项 `{id, level, ok, message, doc}`。
- 控制台首页「上线检查」卡片；`simplebase doctor --instance` CLI；skill 的 `sbdev.py doctor` 在远端目标上调用该接口，把结果给用户看。

**验收**：production 下用默认 `config.example.yaml` 启动必须失败并列出原因；补齐后启动成功、readiness 全绿。

### 3.2 P0-B 项目终端用户与访问规则

这是 planv6.3 D1/D2 当时选了「不做」的部分。**本计划建议改选 D1=B（最小集）+ D2=B（受限规则）**，理由：当时的定位是「开发者平台」，而 AI 生成应用的主要形态恰恰是「带用户的应用」；不补这块，前两层所有安全设计都会被「Key 进前端」绕过。

**范围（最小集，不做 OAuth / MFA / OTP）**：

| 功能 | 接口（项目级，免 API Key） |
| --- | --- |
| 注册 | `POST /v1/projects/:p/end-users/signup`（项目可关闭公开注册） |
| 登录 / 刷新 / 登出 | `POST .../end-users/login`、`/refresh`、`/logout` |
| 当前用户 | `GET .../end-users/me` |
| 改密 | `PUT .../end-users/password` |
| 管理 | 控制台列表、禁用、重置密码（`project:admin`） |

- **存储**：系统库新表 `sys_end_users(project_id, id, email, password_hash, disabled, created_at, ...)`，复用 `internal/auth/password.go` 的 argon2id；会话复用 `sys_user_sessions` 的结构但独立表，避免与平台用户混用。
- **令牌**：项目级 JWT，claims 含 `project_id`、`end_user_id`、`role=end_user`；签名密钥按项目派生，不复用平台会话密钥。
- **邮件验证与找回密码**：依赖 SMTP（§3.6 一起做）；SMTP 未配置时这两项不可用，并在 readiness 里提示。

**访问规则（受限 RLS）**：不做通用表达式语言，只支持**按字段归属**这一种最常见的模式，覆盖「只能看/改自己的数据」：

```json
{
  "collection": "todos",
  "owner_field": "owner_id",
  "rules": { "list": "owner", "get": "owner", "create": "auth", "update": "owner", "delete": "owner" }
}
```

- 取值只有 `public`（匿名可）、`auth`（任一登录终端用户）、`owner`（`owner_field == 当前 end_user_id`）、`deny`。
- `create` 时服务端强制写入 `owner_field`，客户端传的值被覆盖。
- `list` 的 owner 规则在 SQL 层拼 `WHERE json_extract(data, '$.owner_id') = ?`，参数化，不经字符串拼接；复用集合 filter 的方言函数。
- 规则存放在集合元数据（`sys_collections` 增列或新表），控制台在集合页编辑；无规则的集合**对终端用户默认 deny**（fail-closed），API Key 访问不受影响。
- 先只作用于集合文档 API 与对象存储（按前缀 `users/{end_user_id}/`）；SQL 表不开放给终端用户（SQL 面太大，行级规则做不严）。

**SDK**：JS SDK 增加 `auth.signUp / signIn / signOut / onAuthStateChange`，令牌自动续期；文档把「浏览器警告」改写为「前端用终端用户令牌，后端用 API Key」。

**不做**：OAuth2 provider、MFA、OTP、通用规则表达式、SQL 表行级规则。这些放到后续，按用户反馈再排。

**验收**：`examples/` 新增一个「每个用户只能看自己的待办」站点，前端只持有 publishable 配置（项目 ID + URL），没有任何 API Key；用两个终端用户互相越权访问全部 403。

### 3.3 P0-C 云函数出网控制与函数密钥

**出网控制**（解释器层，不新增依赖）：

```yaml
functions:
  egress:
    mode: allowlist        # off | denylist | allowlist；production 默认 allowlist
    allow_hosts: []        # 项目级可在控制台追加；支持 *.example.com
    block_private: true    # 永远拦截回环、私网、链路本地、云元数据地址
    timeout: 10s
    max_response_bytes: 10485760
```

- 在 `gofunction` 注册的 `net/http` 包里替换默认 Transport：`DialContext` 解析后检查最终 IP（防 DNS rebinding：检查解析结果而不是 host 字符串），拒绝 `127.0.0.0/8`、`10/8`、`172.16/12`、`192.168/16`、`169.254/16`、`::1`、`fc00::/7`、`fe80::/10`；重定向的每一跳都检查。
- 拦截返回明确错误 `egress_denied`，写入调用日志，方便用户在控制台看到「函数想访问 X 被拦」后一键加白。
- 项目级白名单存系统库，`dev` 清单增加 `functions_egress.allow_hosts`，可随工作区重放。

**函数密钥**：

- 新表 `sys_project_secrets(project_id, name, value_enc, updated_at)`，值用实例主密钥加密（AES-GCM，标准库）；主密钥来自 `SIMPLEBASE_SECRETS_KEY`，production 下必填。
- 函数内读取：注册包 `simplebase/secrets` 提供 `secrets.Get("STRIPE_KEY")`；不经 `os.Getenv`，避免把宿主环境变量暴露给函数。
- API：`PUT/DELETE/GET(仅列名) /v1/projects/:p/secrets`，`project:admin`；值永不回显，日志与审计只记名称。
- `dev` 清单增加 `secrets: ["STRIPE_KEY"]`（只声明名称），`dev plan` 检查目标环境是否已设置，缺失时警告而不是写入；值由用户在各环境控制台填写。
- 控制台保存函数源码时复用 validate 的密钥扫描（`sb_live_`、`sk-`、`AKIA` 等），命中则拒绝保存并提示改用 secrets。
- **同一加密层迁移已有明文密钥**：`sys_llm_settings.api_key`、`sys_llm_provider_creds.credentials_json` 改为加密存储；启动时检测到明文行自动加密回写（一次性迁移，幂等），production 下未配置 `SIMPLEBASE_SECRETS_KEY` 拒绝启动。

**验收**：函数请求 `http://169.254.169.254/` 与解析到私网的域名均返回 `egress_denied`；`secrets.Get` 能读到值，`GET /secrets` 只返回名称；含 `sk-` 的源码保存失败。

### 3.4 P0-D 观测端点收口

- `/metrics`：新增 `observability.metrics_address`（如 `127.0.0.1:9090`）单独监听；或 `observability.metrics_token`（Bearer）。production 下二者至少有一个，否则拒绝启动。
- `/debug/pprof`：只允许挂在 metrics 监听上，不再挂在业务端口。
- 验收：production 下业务端口访问 `/metrics`、`/debug/pprof/` 返回 404。

### 3.5 P1-A 数据安全网

| 项 | 方案 |
| --- | --- |
| 备份默认开 | production 下 `backup.enabled` 默认 true、`schedule` 默认每日；S3 模式默认 `upload_to_s3=true` |
| 恢复演练 | 新增定时任务：每周随机挑一份备份恢复到临时文件，校验 SHA-256 与引擎可打开、表数量一致，结果写 readiness 与告警（§3.6）；不覆盖线上数据 |
| 迁移前快照 | 远端 `dev apply` 遇到待执行迁移时，先调 `backup create` 再执行；失败则中止。`--no-backup` 显式跳过。本地目标不做 |
| 回收站 | 文档删除、对象删除、函数删除进入 7 天软删除（系统库记录 + 对象延迟物理删除）；删库提供「删库前自动备份」而非软删除（单文件模型下软删除成本高） |
| 系统库保护 | 本地存储模式下把系统库纳入定时备份（重新评估 planv6.3 D3，只做本地模式） |

**验收**：迁移执行前自动产生一份备份并在 plan 输出里展示；恢复演练失败时 readiness 变红并触发告警。

### 3.6 P1-B 告警与通知

- **通道**：先做 Webhook（标准库 HTTP，签名头 `X-SimpleBase-Signature: HMAC-SHA256`）和 SMTP（标准库 `net/smtp`）。飞书 / Slack / 企业微信用 Webhook 模板覆盖。
- **事件**（内置，不做规则引擎）：实例 readiness 变红、备份失败、恢复演练失败、S3 同步滞后超过阈值、写租约丢失、云函数错误率 5 分钟内超过阈值、触发器队列丢弃、配额达到 80% / 100%、登录爆破触发限流。
- **去重与限频**：同类事件 30 分钟内只发一次，恢复时发一条「已恢复」。
- 配置放项目设置（项目级事件）与实例配置（实例级事件）；Webhook URL 视为密钥存 secrets 表。

### 3.7 P1-C 审计补全与密钥轮换

- 补审计：API Key 签发 / 吊销 / scope 变更、平台用户增删改与角色变化、终端用户禁用、项目设置与 secrets 变更（只记名称）、备份恢复、CORS / 出网白名单变更。统一走 `audit.Record`，补一张「必须审计的路由」清单测试，新增写路由不在清单里就失败。
- `api_key_hash_secret` 版本化：支持 `SIMPLEBASE_AUTH_APIKEY_SECRETS=v2:xxx,v1:yyy`，新 Key 用最新版本签名、校验时按 Key 记录的版本选密钥；旧版本可在所有 Key 轮换后下线。
- API Key 增加「最后使用时间」与「90 天未使用」提示，控制台可一键吊销。

### 3.8 P2-A 前端托管与自定义域名

- 静态站点托管：项目级 `sites`，上传构建产物（zip）到对象存储，按 `Host` 头路由到项目；SPA 回退 `index.html`；带版本、可回滚。`dev` 清单增加 `site: { dir: "web/dist" }`，随 `dev apply` 发布。
- 自定义域名：只做「绑定 + 校验 TXT 记录 + 按 Host 路由」，**证书仍由反代负责**（保持 planv6.3 §6 不做 ACME 的决定）；文档给出 Caddy 一键配置样例，作为小白的推荐部署方式。

### 3.9 P2-B 项目生命周期与团队

- `DELETE /v1/projects/:p`：二次确认（项目 ID）、先自动做最终备份、进入 7 天待删除状态、到期物理删除；超管可撤销。
- 项目整包导出：数据库快照 + 对象文件 + 函数 / cron / 集合元数据（不含 secrets 值），格式与 `dev` 工作区兼容，可直接 `dev apply` 到另一实例。
- 项目成员：`owner / developer / viewer` 三角色，映射到现有权限位；邀请走 SMTP。

### 3.10 P2-C 触发器持久队列

- 触发事件先写系统库 `sys_trigger_outbox`，分发器从 outbox 拉取，成功后标记；重启后继续处理未完成事件；失败按指数退避重试 N 次后进入死信，控制台可重放。
- 保持「至少一次」语义，文档要求函数幂等。

---

## 4. 迭代计划

| 里程碑 | 内容 | 预估 | 依赖 |
| --- | --- | --- | --- |
| **M1 安全默认值** | §8.0 profile、§8.1 入口层（安全头 / 限流 / 观测收口 / loginGuard）、§8.2.2 引导口令与强制改密闭环、§8.3.6 readiness + `simplebase doctor` | 6–8d | — |
| **M1.5 身份层底座** | §8.2.3 密钥域拆分、§8.2.4 Keyring 与 Key 元数据、§8.2.5 审计补全、§8.3.1 日志落库闭环、§8.3.3 审计表与查询 | 5–6d | M1 |
| **M2 代码执行安全** | §3.3 出网控制 + secrets + 源码密钥扫描 | 5–6d | M1（production 默认 allowlist） |
| **M3 终端用户** | §3.2 注册登录、§8.2.6 身份层接入（独立密钥域 / 路由组 / owner 规则门禁）、JS SDK、新示例站点 | 10–12d | M1.5 |
| **M4 运维闭环** | §8.3.2 备份调度与恢复演练、§8.3.4 水位预警、§8.3.5 告警总线、§3.6 通知 | 7–9d | M1.5 |
| **M5 上线闭环** | §3.8 + §3.9 + §3.10 | 10–12d | M3、M4 |

M1、M1.5 可部分并行；M3 仍是最大块。建议本迭代交付 **M1–M3 + M4 的备份部分（§8.3.2）**，其余进入下一迭代前半段。

**每个里程碑的验收方式**：

- 后端单测 + handler 测试（fake service）+ 系统库拒绝用例（沿用 AGENTS.md 要求）。
- `examples/capabilities` 新增对应用例：`16-production-profile`、`17-backup-restore`、`18-secrets`（M2）、`19-end-users`、`20-alerts`，接入 `run_all.sh`；其中 `16` 与 `17` 的验收断言见 §8.1.10 / §8.3.8。
- `scripts/examples-verify/verify.sh` 增加一轮 `--profile production`，四个站点在生产模式下仍须 0 FAIL。
- skill（`skills/simplebase`）同步：`doctor` 调 readiness、清单新增字段两端（Go / Python）同时实现并共用 fixture。

---

## 5. 需要拍板的决策

| 编号 | 决策 | 建议 | 影响 |
| --- | --- | --- | --- |
| R1 | 是否推翻 planv6.3 D1/D2，做项目终端用户与受限行级规则 | **做（D1=B、D2=B）** | 决定 M3 是否立项；定位从「开发者平台」扩展到「带用户的应用后端」 |
| R2 | production 下引导账号的处理 | 改为首次启动从 `SIMPLEBASE_BOOTSTRAP_PASSWORD` 注入；未注入时仅监听回环地址完成初始化 | 影响 Docker 首次启动文档 |
| R3 | 云函数出网默认策略 | production 默认 allowlist，development 默认 denylist（只拦私网） | 已有函数在生产模式下可能需要补白名单 |
| R4 | 实例主密钥 `SIMPLEBASE_SECRETS_KEY` 丢失的处理 | 不提供找回；文档要求与 `api_key_hash_secret` 一起放进密钥管理 | 丢失即 secrets 全部不可读 |
| R5 | 是否引入 SMTP 之外的通知 SDK | 不引入，Webhook 模板覆盖 IM | 保持零新增依赖 |
| R6 | 软删除范围 | 文档 / 对象 / 函数做；删库只做「删前自动备份」 | 单文件模型下删库软删除成本过高 |
| R7 | production 引导口令缺失时的行为 | 拒绝启动（§8.2.2），而非只警告 | Docker 首次启动必须传 env；examples / verify.sh 用 dev profile 不受影响 |
| R8 | `session_secret` 未显式配置时是否拒绝启动（production） | 本迭代只警告 + readiness 标记；下个迭代再评估升级 | 轮换 Key 盐会登出全部用户是接受的行为；给用户一个版本周期的适应期 |

---

## 6. 明确不做（本迭代）

- 进程内 TLS / ACME：仍由反代负责，提供 Caddy 样例（维持 planv6.3 §6）。
- 多写实例高可用：单写实例是架构边界；通过 RPO≈15s + 备份 + 告警兜底。
- OAuth2 / MFA / OTP / 通用规则表达式：等 M3 上线后按反馈排。
- WAF、DDoS 防护：交给云厂商或 CDN，文档说明。
- 合规认证（SOC2 / 等保）：本计划提供的审计、加密、删除能力是前置条件，但不以认证为目标。

---

## 7. 风险

| 风险 | 应对 |
| --- | --- |
| 生产模式收紧后老用户升级即启动失败 | 默认 `profile=development` 不变；升级文档列出切换步骤，readiness 先以「警告」形式运行一个版本 |
| 行级规则做不严导致越权 | 只支持 owner 一种模式，fail-closed；越权测试矩阵（匿名 / 他人 / 自己 × 五种操作 × 集合与对象）作为合并门槛 |
| 出网检查被 DNS rebinding 绕过 | 检查连接时的实际 IP，而不是 URL 主机名；每次重定向重新检查 |
| secrets 主密钥泄露 | 主密钥只来自环境变量，不落 YAML / 日志；支持版本化轮换（同 §3.7） |
| 终端用户表放系统库，量大后影响系统库性能 | 先按项目分区索引，记录量与查询延迟进指标；超过阈值再评估迁到项目单文件 |
| `/v1/auth/password/initial` 成为新的爆破面 | 端点强制要求旧口令正确 + 仅对 `must_change_password` 用户生效 + 复用 loginGuard 同款限流；409 与 401 语义区分，不泄露用户是否存在 |
| 密钥域拆分后委托令牌 / envd 令牌验签错密钥导致全量 401 | §8.2.3 要求跨模块 round-trip 测试；`applyProfileDefaults` 之外不允许任何地方直接读 `APIKeyHashSecret` 做 JWT 签名（加 lint 式测试 grep 调用点） |
| 备份调度循环卡死或重复触发 | 调度器单飞（`sync.Mutex` + in-flight 标记）；`sys_backup_state.last_attempt_at` 单调递增，catchup 判定以此为准，不依赖内存状态 |


---

## 8. 第一批细化：入口层 / 身份层 / 运维层（2026-11-09）

> 本节把 §1.1、§1.2、§1.6 的缺口（含 2026-11-09 复核补充）落到「怎么做、改哪些文件、怎么验收」。与 §3 的关系：§3.1（生产模式）、§3.4（观测收口）、§3.6（告警）、§3.7（审计与轮换）在这里展开为可直接开工的设计；§3.2（终端用户）只定义身份层的接入点（§8.2.6），完整设计仍以 §3.2 为准。
>
> 全部改动遵守硬性约束：不新增第三方依赖、配置只来自 YAML + `SIMPLEBASE_*`、密钥不落 YAML、系统库表结构直接改 `schema.go`（本节新增的都是**新表**，不改已有表，避免已有实例必须 reset）。
>
> 结构总览：§8.0 profile 开关 → §8.1 入口层（对应 §1.1）→ §8.2 身份层（对应 §1.2）→ §8.3 运维层（对应 §1.6）。各节内小节编号规则：x.y.1 目标与验收 → x.y.2+ 设计 → 末尾「代码改动清单」与「测试」。

### 8.0 公共前置：`instance.profile`

§8.1–§8.3 的大部分「生产默认值」都挂在这个开关上，先单独实现。

```yaml
instance:
  profile: development   # development | production。SIMPLEBASE_INSTANCE_PROFILE
```

**实现**：

| 文件 | 改动 |
| --- | --- |
| `internal/config/config.go` | `InstanceConfig` 增 `Profile string`；增 `func (c Config) IsProduction() bool`；`Validate` 校验取值 |
| `internal/config/yaml.go` / `env.go` | 解析 `instance.profile` / `SIMPLEBASE_INSTANCE_PROFILE` |
| `internal/config/profile.go`（新） | `applyProfileDefaults(cfg *Config, explicit explicitFields)`：只在**用户没有显式设置**时填生产默认值。`explicit` 由 YAML 指针字段非 nil + `os.LookupEnv` 命中两路汇总，避免「用户显式写 `rate_limit.enabled: false` 被生产模式悄悄改成 true」 |
| `internal/config/profile_test.go`（新） | 表驱动：dev 不变 / prod 填默认 / prod 下显式值优先 |

`Load()` 顺序变为：代码默认值 → YAML → env → **`applyProfileDefaults`** → `Validate`（含 `validateProduction`，拒绝项在这里报错）。

### 8.1 入口层（网络与 HTTP）

#### 8.1.1 目标与验收

1. production 下业务端口：`/metrics`、`/debug/pprof/*` 返回 404；所有响应带正确的安全头；控制台带 CSP；HTTPS 部署带 HSTS。
2. 同 VPC 主机伪造 `X-Forwarded-For` 不能绕过按 IP 限流。
3. 同一 IP 换用户名撞库被限速；同一用户名被多 IP 爆破触发软锁定并产生告警事件（§8.3）。
4. 内置限流规则可按「IP」「身份」「项目」分别计数，且按身份的规则**真的拿得到身份**。
5. 用户上传的文件不能在控制台源下执行脚本。

#### 8.1.2 架构：中间件链调整

```text
现状（router.go NewRouter）
  requestID → [CORS] → securityHeaders → [perfStage + pprof 路由] → Recover → accessLog
  → [gzip] → rateLimiter(此时尚未认证，principal 恒空) → bodyLimit
  → /health/* → /metrics(无鉴权) → /v1/auth/login|refresh → /v1 组(authMW → projectContext → handler)

目标
  requestID → [CORS] → securityHeaders(按路由类别) → Recover → accessLog → [gzip]
  → rateLimit(stage=pre_auth，只跑 key=ip 的规则) → bodyLimit
  → /health/*
  → [/metrics、/debug/pprof：仅当未配置独立监听时挂在业务端口，且需 Bearer token]
  → /v1/auth/login|refresh|password/initial(登录防护 loginGuard)
  → /v1 组：authMW → rateLimit(stage=post_auth，跑 key=principal|project 的规则) → projectContext → handler
  → /go 组：同上

独立观测监听（observability.metrics_address 非空时）
  127.0.0.1:9090 → /metrics、/debug/pprof/*（pprof 需 observability.pprof=true）
```

#### 8.1.3 安全响应头（替换 `securityHeadersMiddleware`）

按路由类别发不同的头，类别由路径判定（`routeClass(path)`，纯函数，单测覆盖）：

| 类别 | 判定 | 额外响应头 |
| --- | --- | --- |
| `api` | `/v1/`、`/go/`、`/health/`、metrics 路径 | `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'`、`Cache-Control: no-store`（仅 `/v1/auth/*`） |
| `file` | `/files/`、`/v1/projects/*/s3/objects/content` | `Content-Security-Policy: sandbox; default-src 'none'`、`Cross-Origin-Resource-Policy: same-site` |
| `console` | 其余 GET（SPA 与静态资源） | 见下方 CSP；`Cross-Origin-Opener-Policy: same-origin` |

控制台 CSP（构建产物已核实无内联脚本，只有 `/assets/*.js` 外链）：

```text
default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline';
img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self';
object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'
```

- `style-src 'unsafe-inline'` 是因为 Vue `:style` 绑定与 reka-ui 定位会写内联 style 属性；脚本不放开。
- 浏览器终端走 WebSocket 同源，`connect-src 'self'` 已覆盖 `ws(s)://同源`（CSP3 语义），上线前在 Safari 上回归一次。
- 四个公共头（`nosniff`、`X-Frame-Options`、`Referrer-Policy`、`Permissions-Policy`）保持所有类别都发。

**HSTS**：只在「确认是 HTTPS」时发——请求本身是 TLS，或直连对端在 `trusted_proxies` 内且 `X-Forwarded-Proto: https`。不用 echo 的 `c.Scheme()`（它无条件信任 `X-Forwarded-Proto`）。

```yaml
http:
  public_url: ""                 # 对外访问地址，如 https://api.example.com。SIMPLEBASE_HTTP_PUBLIC_URL
                                 # 用于：HSTS 判定兜底、文件外链、告警消息里的控制台链接、readiness 检查
  security_headers:
    hsts_max_age: 0              # 秒；0=不发。production 且 public_url 为 https 时默认 31536000
    hsts_include_subdomains: false
    csp: enforce                 # enforce | report-only | off；development 默认 report-only
```

#### 8.1.4 用户文件下载

- `s3_content_handler.go:213` 的 `inline` 改为白名单：`image/png|jpeg|gif|webp`、`application/pdf`、`text/plain` 才 `inline`，其余（含 `image/svg+xml`）一律 `attachment`。
- `file` 类别统一加 `CSP: sandbox`，即使 inline 渲染也没有脚本执行能力、没有同源权限。
- 单测：SVG / HTML 声明 + 内容组合 → 断言 `attachment` 与 CSP 头。

#### 8.1.5 真实 IP 收紧

`trustedIPExtractor` 改为**只信任显式配置的 CIDR**：

```go
options := []echo.TrustOption{echo.TrustLoopback(false), echo.TrustLinkLocal(false), echo.TrustPrivateNet(false)}
for _, cidr := range cidrs { options = append(options, echo.TrustIPRange(network)) }
```

- 行为变化：原来 `trusted_proxies: ["10.0.0.5/32"]` 时，其余私网地址也被信任；改后只信任 `10.0.0.5`。本机 Nginx/Caddy 反代需显式写 `127.0.0.1/32`。写进升级说明（§8.4 R7）。
- 新增 `isTrustedPeer(c) bool`（直连对端在 `trusted_proxies` 内），供 HSTS / `X-Forwarded-Proto` 判定复用。
- `validateProduction`：`trusted_proxies` 含 `0.0.0.0/0` 或 `::/0` 时拒绝启动（等于信任全网 XFF）。

#### 8.1.6 限流重构（统一实现、分阶段、可选键）

**规则模型扩展**（向后兼容：不写 `key` 时取 `ip+principal`，与现状一致）：

```yaml
limits:
  rate_limit:
    enabled: false           # production 默认 true
    burst: 2
    exempt_sse: true
    max_buckets: 100000      # 内存上限；超过后先清理过期桶，仍超则对新键放行并计数（fail-open + 指标）
    rules:
      - { match: "/v1/auth/refresh", key: ip,        max: 30,   window: 1m }
      - { match: "/files/**",        key: ip,        max: 300,  window: 1m }
      - { match: "/go/**",           key: principal, max: 600,  window: 1m }
      - { match: "/v1/projects/*/v1/**", key: principal, max: 60, window: 1m }   # LLM 兼容出口
      - { match: "/v1/**",           key: principal, max: 1200, window: 1m }
```

- `match` 增加 `/**` 后缀表示前缀匹配（`path.Match` 的 `*` 不跨 `/`，现有规则写不出「`/go` 下所有路径」）。
- `key` 取值：`ip` | `principal` | `project` | `ip+principal`。`principal` 取 `APIKeyID`/`UserID`/`EndUserID`（§8.2.6），未认证时退化为 `ip`。
- **分阶段**：`rateLimiter.middleware(stage)`；`pre_auth` 只执行 `key=ip` 的规则（全局 `e.Use`），`post_auth` 执行其余规则（挂在 `/v1` 与 `/go` 组的 `authMW` 之后）。同一请求两阶段各命中第一条规则。
- production 下用户 `rules` 为空时使用上表内置规则；用户写了规则则**完全以用户为准**（不合并，避免两套规则叠加难以推理）。
- 内置规则再补一条 `{ match: "/v1/projects/*/end-users/signup", key: ip, max: 5, window: 1m }`（对应 §3.1 的「注册 5/min」）。**§3.1 表中的内置限流规则以本节为准**，避免两份规则漂移；`applyProfileDefaults` 是唯一注入点。

**登录防护 `loginGuard`**（替换 `auth_handler.go` 私有的 `loginLimiter`，复用同一令牌桶实现）：

| 维度 | 默认 | 作用 |
| --- | --- | --- |
| `ip + username` | 10 次 / 分钟 | 现有行为 |
| `ip` | 30 次 / 分钟 | **新增**，挡「固定密码换用户名」撞库 |
| `username` 失败计数 | 15 分钟内失败 10 次 → 该用户名软锁定 15 分钟（返回 429 `login_locked`） | **新增**，挡分布式爆破 |

- 软锁定豁免「该用户最近 5 个登录成功的 IP」（内存记录），避免攻击者用已知用户名把管理员锁死在外面。
- 成功登录清零失败计数。触发软锁定时：写审计 `auth.login.locked`、`simplebase_auth_login_total{outcome="locked"}` +1、发告警信号 `auth.bruteforce`（§8.3）。
- `/v1/auth/password/initial`（§8.2.1）共用同一个 `loginGuard`。

#### 8.1.7 观测端点收口（§3.4 展开）

```yaml
observability:
  metrics_path: /metrics
  metrics_address: ""        # 非空时 /metrics 与 pprof 改到独立监听，如 127.0.0.1:9090。SIMPLEBASE_METRICS_ADDRESS
  pprof: false               # 是否挂 /debug/pprof；只会挂在独立监听或带 token 的路由上。SIMPLEBASE_PPROF
  # SIMPLEBASE_METRICS_TOKEN：Bearer token，只能来自环境变量
```

| 组合 | 业务端口 `/metrics` | 业务端口 pprof | 独立监听 |
| --- | --- | --- | --- |
| dev，都没配 | 开放（现状） | `pprof=true` 时开放 | 无 |
| 配了 `metrics_address` | 404 | 404 | `/metrics` + （`pprof=true` 时）pprof，无鉴权（靠监听地址隔离） |
| 只配了 token | 需 `Authorization: Bearer <token>`（常量时间比较），否则 404 | 同左 | 无 |
| production，都没配 | **拒绝启动** | — | — |

- pprof 与 `perf_stage_timing` 解耦：`perf_stage_timing` 只负责分段计时与 mutex/block 采样率，不再决定是否挂 pprof 路由。
- `metrics_token` 加入 `yamlSecretPresence`（写进 YAML 直接拒绝启动）。

#### 8.1.8 TLS（仍不在进程内终止）

维持 §6「不做进程内 TLS / ACME」，但把「对小白是空白」补成可执行的默认路径：

- `docs/ops/deployment.md` 增加「一键 Caddy」小节：`Caddyfile` 样例（自动证书 + `reverse_proxy 127.0.0.1:8080` + `header_up X-Forwarded-Proto`），以及对应的 SimpleBase 配置（`http.address: 127.0.0.1:8080`、`trusted_proxies: [127.0.0.1/32]`、`public_url: https://...`）。
- readiness（§3.1）增加两项警告：`public_url` 为空或非 https；`http.address` 监听 `0.0.0.0` 且 `trusted_proxies` 为空（说明大概率直接暴露明文 HTTP）。

#### 8.1.9 代码改动清单

| 文件 | 改动 |
| --- | --- |
| `internal/api/middleware_http.go` | `securityHeadersMiddleware(cfg)` 按 `routeClass` 发头；`trustedIPExtractor` 收紧；新增 `isTrustedPeer`、`isHTTPS` |
| `internal/api/middleware_ratelimit.go` | 规则 `key` / `/**` 前缀匹配 / 分阶段 / `max_buckets`；导出给 `loginGuard` 复用的 `allow` |
| `internal/api/login_guard.go`（新） | 三维度登录防护 + 软锁定 + 成功 IP 豁免；替换 `auth_handler.go` 的 `loginLimiter` |
| `internal/api/observability_routes.go`（新） | 从 `NewRouter` 抽出 `/metrics`、pprof 挂载；`NewObservabilityHandler(deps) http.Handler` 供独立监听使用 |
| `internal/api/router.go` | 中间件顺序按 §8.1.2 调整；`/v1`、`/go` 组挂 `post_auth` 限流 |
| `internal/api/s3_content_handler.go` | `Content-Disposition` 白名单 |
| `internal/app/app.go` | `metricsServer *http.Server`：`Start` 中并行监听、`Shutdown` 中关闭；监听失败视为启动失败 |
| `internal/config/{config,yaml,env,http_boundary}.go` | 新字段解析与校验；`metrics_token` 入密钥拒绝清单 |
| `config.example.yaml` | 新字段与注释 |
| `docs/ops/deployment.md` | Caddy 样例、`trusted_proxies` 行为变化说明 |

#### 8.1.10 测试

- `middleware_http_test.go`：三类路由 × dev/prod 的响应头表；HSTS 只在可信 HTTPS 时出现；伪造 XFF（非可信对端）`RealIP` 不变。
- `middleware_ratelimit_test.go`：`/**` 匹配、`key` 四种取值、两阶段各自生效、`post_auth` 能取到 principal、`max_buckets` 溢出 fail-open 并计数。
- `login_guard_test.go`：换用户名撞库被 `ip` 维度拦截；多 IP 爆破触发软锁定；成功 IP 豁免；成功登录清零。
- `observability_routes_test.go`：§8.1.7 四种组合。
- `config/profile_test.go`：production 缺 metrics 收口拒绝启动、`trusted_proxies` 全网拒绝启动。
- `examples/capabilities/16-production-profile`：起 production 实例，断言业务端口 `/metrics` 404、控制台带 CSP。


### 8.2 身份层（谁在调用）

#### 8.2.1 目标与验收

1. 公网实例**任何时刻**都不存在可用默认口令登录的超管。
2. 管理员重置某用户口令后，该用户能自行完成首次改密并登录（当前是死路）。
3. 轮换 `api_key_hash_secret` 时，已签发 Key 不失效，且会话可按独立密钥保持有效。
4. 控制台能看到「哪把 Key、什么时候最后用过」，可一键吊销；签发 / 吊销 / 改权限全部留审计。
5. 终端用户令牌**无法**访问管理面（SQL、备份、设置、日志、用户管理、函数管理）的任何一个路由。
6. 无 API Key 的前端站点可以注册登录，且只能读写自己的数据（完整设计仍见 §3.2）。

#### 8.2.2 P0 修补：引导账号与强制改密（阻塞上线）

这是 §1.2 复核里最严重的一条，独立于后续所有工作，必须最先做。

**引导口令注入**：

```text
SIMPLEBASE_BOOTSTRAP_PASSWORD   # 只在首次种子时读取，不落 YAML（加入 yamlSecretPresence 拒绝清单）
```

| profile | 行为 |
| --- | --- |
| `development` | 未注入时用 `simplebase2026`（保持现有 examples / `verify.sh` 零配置），日志打一条 warn |
| `production` | **未注入则拒绝启动**；注入后校验长度 ≥ 16 且不等于 `simplebase2026` |

- `EnsureBootstrapUser(ctx, password string)` 改签名，去掉 `BootstrapPassword` 常量（保留 `BootstrapUsername`）；`systemdb.SeedInput` 增 `BootstrapPassword`。
- 已有实例（库里已有 `simplebase2026` 行）：启动时读 `must_change_password`，若为 0 且口令 hash 仍是 `simplebase2026` 的 hash，production 下**拒绝监听对外地址**并给出可执行提示（「设置 `SIMPLEBASE_BOOTSTRAP_PASSWORD` 并 `simplebase reset` 重建，或先登录改密」）。不自动改口令（没有带外通道通知用户新口令）。
- `examples/lib/init/run.mjs`、`scripts/examples-verify/bootstrap.mjs` 已支持 `SIMPLEBASE_ADMIN_PASSWORD`，无需改脚本；`docs/getting-started/quickstart.md` 补 production 段落。

**修复强制改密闭环**（新增端点，补齐 `IssueAfterPasswordChange` 的调用方）：

```text
POST /v1/auth/password/initial     # 免认证
body { username, old_password, new_password }
→ 200 { token_type, access_token, refresh_token, expires_in, user }
```

- 流程：验旧密 → 要求 `u.MustChangePassword=true`（否则 409 `password_change_not_required`，避免变成免登录改密后门）→ 校验新口令 → 更新并清 flag → 吊销该用户全部会话 → 直接签发正式 TokenPair 返回。
- 挂在 `e.POST` 的免认证区（`/v1/auth/login` 旁边），**不经** `/v1` 组的 `authMW`；复用 `loginGuard`（§8.1.6）的 `ip`/`ip+username` 两维度。
- `AuthHandler.Login` 的 423 响应体补 `user` 字段（含 `must_change_password=true`），前端 `ui/src/services/http-api.ts` 的 `auth.login` 识别 423 后不再抛错，改为 `auth.markMustChangePassword(username)`；`LoginModal` 的 `password` 模式调新端点，成功后 `applyTokens` 直接进控制台（不再 `markUnauthorized` 重新登录）。
- `UsersHandler.Patch` 重置口令仍置 `MustChangePassword=true`（现有行为正确）；`UsersHandler.Create` 也置 true（当前新建用户可直接用管理员给的口令登录，无强制改密）。
- 口令强度：`ValidatePassword` 增三条（长度 ≥ 12 for production / 不得等于用户名 / 不得命中常见弱口令表 `12345678`、`password`、`simplebase2026` 等 20 条），development 保持 ≥ 8 以免打断现有测试。

#### 8.2.3 密钥职责分离

现状：`api_key_hash_secret` 同时是 API Key HMAC 盐、会话 / 委托 JWT 密钥、envd 令牌派生源。轮换一次 = 全站登出 + 所有 Key 失效 + 所有沙盒令牌失效。

```text
SIMPLEBASE_AUTH_APIKEY_SECRET     # 现有名，保留
SIMPLEBASE_AUTH_SESSION_SECRET    # 新，可选；未设置时 = HMAC(apikey_secret, "simplebase-session-v1") 派生
```

| 用途 | 密钥来源 |
| --- | --- |
| API Key hash | 版本化密钥（§8.2.4），当前版本默认取 `SIMPLEBASE_AUTH_APIKEY_SECRET` |
| 登录态 / 委托 JWT | `SIMPLEBASE_AUTH_SESSION_SECRET`（显式）或派生值 |
| envd 令牌 | 沿用 `newEnvdTokenSigner` 的域分离派生，但**入参改为 session secret**，与 Key 轮换彻底解耦 |
| 终端用户 JWT | 按项目派生：`HMAC(session_secret, "end-user-v1:"+projectID)`（§8.2.6） |

- `auth.Service` 保持单密钥入参（API Key 通道），`SessionService` 构造处改传独立密钥；`app.go:663-667`、`app.go:1128`（委托签发）、`api/sandbox_handler.go:113`（envd）三处调用点同步。
- 委托令牌 `DelegationIssuer` 与 `AuthMiddlewareWithDelegation` 的验签密钥必须同源于 session secret，否则现有助手 skill 全部 401；加一条跨模块 round-trip 测试锁定。
- production readiness 增警告项：`auth.session_secret_inherited=true`（未显式配置时，轮换 Key 盐仍会登出所有人）。是否升级为「拒绝启动」见 §8.4 R8。

#### 8.2.4 API Key 版本化轮换与可辨识元数据

**密钥配置**（兼容现有单值）：

```text
SIMPLEBASE_AUTH_APIKEY_SECRETS=v2:<new>,v1:<old>   # 第一个 = 当前签发版本；逗号分隔，最多 8 个
SIMPLEBASE_AUTH_APIKEY_SECRET=<old>                 # 单值等价于 v1:<old>，保持全部旧 Key 有效
```

- 新增 `internal/auth/keyring.go`：`type Keyring struct{ versions []KeyVersion }`，`SigningVersion()` / `HashAll(rawKey) []keyHashCandidate`；解析错误由 `config.Validate` 拒绝启动（版本号格式、重复版本、空值、超过 8 个）。
- **校验不依赖明文格式**：`CachedAuthenticate` 对每个已配置版本各算一次 hash，依次查 `FindByHash`，命中即用；最多 8 次索引点查，且命中结果进 30s 缓存，热路径无额外成本。这样旧 Key（无版本段）与委托 / 脚本里的 Key 全部无需改动。
- 缓存键改成 `version + "|" + hash`，避免不同版本 hash 碰撞时串身份。
- 新签发 Key 明文带版本：`sb_live_v2_<32 hex>`（`generateAPIKeySecret`），便于在日志 / 源码扫描里辨认；同时保留前缀扫描规则 `sb_live_` 不变。
- 校验时顺带读 Key 记录的 `secret_version`，与命中版本不一致则按安全失败处理（防止有人手改库把 Key 挪到别的版本下）。

**元数据旁表**（不改 `sys_api_keys`，避免已有实例必须 reset，符合 §8.0 约束）：

```sql
CREATE TABLE IF NOT EXISTS sys_api_key_meta (
  key_id VARCHAR NOT NULL,
  project_id VARCHAR NOT NULL,
  name VARCHAR NOT NULL DEFAULT '',
  prefix VARCHAR NOT NULL DEFAULT '',           -- 明文前 12 字符，仅供列表展示
  secret_version VARCHAR NOT NULL DEFAULT 'v1',
  last_used_at TIMESTAMP
)
```

- `Create` 写 meta（`name` 取自请求体新增可选字段，`prefix` 取 `secret[:12]`，`secret_version` 取当前版本）。
- `List` LEFT JOIN meta，返回 `name / prefix / secret_version / last_used_at`；旧行没有 meta 时 `secret_version='v1'`、`last_used_at=NULL`（显示为「从未记录」而不是 1970 年）。
- `last_used_at` 在 `CachedAuthenticate` **缓存未命中**时异步更新（批量：内存聚合 5s 窗口，一次 `UPDATE ... WHERE key_id IN (...)`），不进请求关键路径、不让读请求产生写放大。
- 控制台「设置 → 连接」列展示 name/prefix/最后使用时间，90 天未使用标黄并提供一键吊销；`GET /v1/projects/:p/api-keys` 响应新增字段，`ui/src/services/types.ts` 与 `http-api.ts` 同步加 interface（不用 `type`、不写 `any`）。
- 审计：`Create` / `Delete` / 策略变更分别 `audit.Record(Kind: "api_key.issue"|"api_key.revoke"|"api_key.scope_change", Detail: key_id + ":" + prefix)`，**不记明文 / hash**。

#### 8.2.5 身份层审计补全（§3.7 的前半）

统一 `PrincipalID` 口径：新增 `api.auditActor(c) string`，规则为 `UserID → APIKeyID → EndUserID → "system"`，替换 `backup_handler.go:146` 的 `principal.APIKeyID` 与 `catalog/service.go:509` 的同类写法。

| 写路由 | 补 `audit.Record` 的 Kind |
| --- | --- |
| `POST/DELETE /v1/projects/:p/api-keys` | `api_key.issue` / `api_key.revoke` |
| `POST/PATCH/DELETE /v1/users` | `user.create` / `user.update` / `user.disable`（Detail 只记 user id + 变更字段名列表） |
| `PUT /v1/settings`、`PUT /v1/projects/:p/settings` | `settings.global_change` / `settings.project_change`（只记变更 key 列表，不记值——值里可能有密钥） |
| `PUT /v1/system/storage` | `settings.storage_change` |
| `PUT /v1/projects/:p/logs/retention` | `settings.logs_retention_change` |
| `POST /v1/projects` | `project.create` |
| `PUT/DELETE /v1/projects/:p/databases/:id/data/collections/:c/meta` | `collection.meta_change` |
| `POST /v1/auth/login` 失败 / 锁定、`POST /v1/auth/password/initial` | `auth.login_failed` / `auth.login_locked` / `auth.password_initial` |

- 加一张「必须审计的写路由」清单测试：遍历 `e.Routes()`，对 `POST/PUT/PATCH/DELETE` 且路径以 `/v1/` 开头的路由，断言其在 `auditedWriteRoutes` 表内；新增写路由漏登记就失败。做法参照 `delegation_route_table_test.go` 的真实 router 遍历（不维护手写字符串白名单与路由漂移）。
- 例外登记表（如 `/v1/auth/logout`、`/v1/projects/:p/realtime/subscriptions`）必须逐条写明理由，不允许通配。

#### 8.2.6 终端用户（§3.2 的接入点与边界）

> 完整产品范围、owner 规则 JSON 与 SDK API 仍以 §3.2 为准。本节只补「怎么接进现有认证链而不把管理面暴露出去」这一层，这是 §3.2 没写、但合并时最容易出事的地方。

**核心决策：用独立密钥域 + 独立路由，而不是在现有 `Require` 上开洞。**

现状 `AuthMiddlewareWithDelegation`（`auth/middleware.go:117`）把 JWT 一律当平台会话 / 委托令牌，用 session secret 验签；`Require` 再按 `HasPermission` 判权限。若让终端用户 JWT 走这条链，就必须给终端用户塞权限位，再靠「路由白名单」挡管理面——白名单一漂移就是越权。改为：

```text
平台会话 / 委托 / API Key → 现有 /v1、/go 组（authMW → projectContext → Require）
终端用户令牌             → 新的 /v1/projects/:p/end-users/** 组（endUserAuthMW → endUserProjectContext → 规则门禁）
```

两条链的签名密钥不同：

```text
endUserSecret(projectID) = HMAC-SHA256(sessionSecret, "simplebase-end-user-v1|" + projectID)
```

- 终端用户令牌发到 `/v1/**`：`LooksLikeJWT` 为真，但用 session secret 验签必然失败 → 回落到 `sessions.VerifyAccess` 也失败 → 401，不会误入 API Key 通道。
- 平台会话 / API Key 发到 `/end-users/data/**`：`endUserAuthMW` 用项目派生密钥验签必然失败 → 401。
- **即使项目 A 的终端用户令牌被拿到项目 B**：claims 里的 `project_id` 与路径 `:projectID` 不等 → 403；且签名密钥按项目派生，B 的密钥也验不过。三道独立防线。
- 会话密钥轮换会同时失效所有终端用户令牌（与平台会话一致），文档写明；后续要独立轮换再加 `SIMPLEBASE_ENDUSER_SECRET`。

**认证链实现**（保持 `auth` 不 import `api`）：

| 文件 | 改动 |
| --- | --- |
| `internal/auth/principal.go` | 增 `EndUserID string`；`func (p Principal) IsEndUser() bool { return p.EndUserID != "" }`。**不新增权限位**，`Permissions` 保持空 map |
| `internal/auth/jwt.go` | `JWTClaims.Type` 已存在，复用：新增常量 `EndUserType = "end_user"`、`EndUserAudience = "simplebase-end-user"`；`ProjectID`/`Subject`/`SessionID`/`JWTID` 沿用现有字段，不改 JWT 结构 |
| `internal/auth/end_user.go`（新） | `EndUserService`：注册 / 登录 / 刷新（轮转 + 重放检测，逻辑镜像 `SessionService`）/ 改密 / 禁用；argon2id 复用 `password.go`；密钥派生函数 `DeriveEndUserSecret` |
| `internal/auth/end_user_repository.go`（新） | 读写 `sys_end_users` / `sys_end_user_sessions` / `sys_end_user_rules`；写后调 `store.NotifyWrite`（SYNC-01 同款） |
| `internal/auth/middleware.go` | 新增 `AuthMiddlewareWithEndUsers(...)`：在 `AuthMiddlewareWithDelegation` 前多一段；`EndUserAuthenticator` 接口（`VerifyEndUser(ctx, token) (Principal, error)`）由 `api` 注入适配器。旧函数保留为包装（传 nil），现有测试不破 |
| `internal/api/end_user_handler.go`（新） | `EndUserHandler`（signup / login / refresh / logout / me / password）+ `EndUserAdminHandler`（列表、禁用、重置口令、规则 CRUD、项目开关） |
| `internal/api/end_user_context.go`（新） | `endUserAuthMW`（可选认证：无 Authorization 头 → 匿名 Principal；有头 → 必须验过）、`endUserProjectContext`：解析项目 → 拒绝系统项目 → 查 `sys_project_auth_settings.enabled` → 注入 `ProjectContext` → 解析项目默认库并 `c.SetParamNames/SetParamValues` 补 `databaseID` |
| `internal/api/router.go` | 在 `web.Register` 之前挂 §8.2.6 的公开组与认证组；管理端路由仍挂 `/v1` 组内 `require(auth.ProjectAdmin)` + `writeGate` |

**为什么数据面走新路径**：新增 `/v1/projects/:p/end-users/data/collections/...` 与 `.../end-users/objects/...`，内部复用 `DataHandler` / `S3Handler` 的方法（注入好 `Principal` 与 `ProjectContext` 后直接调用），而不是给 `/v1` 组加例外。好处是「管理面」一行代码都不用改，越权面随新路由显式可见；代价是 SDK 需要按 end-user 模式切一组路径。

| 路由组 | 认证 | 说明 |
| --- | --- | --- |
| `POST /v1/projects/:p/end-users/signup\|login\|refresh` | 免认证 | 挂 `loginGuard`；`signup` 受 `signup_enabled` 与 5/min 限流 |
| `POST .../end-users/logout`、`GET .../me`、`PUT .../password` | `endUserAuthMW`（必须） | 平台 `/v1/auth/password/initial` 的同类语义 |
| `GET\|POST\|PATCH\|PUT\|DELETE .../end-users/data/collections/:c[/documents/:id]`、`.../documents/batch` | `endUserAuthMW`（可选） | 匿名只在规则为 `public` 时放行；`auth`/`owner` 要求 `EndUserID != ""` |
| `GET\|POST\|DELETE .../end-users/objects/...` | 同上 | 强制 key 前缀 `users/{EndUserID}/`，`public` 规则只允许前缀 `public/` |
| `GET\|PUT /v1/projects/:p/end-users/settings`、`GET\|PUT .../rules/:c`、`GET .../users`、`PATCH .../users/:id` | 平台 `authMW` + `require(auth.ProjectAdmin)` | 管理面，`writeGate` 同现有写路由 |

**owner 规则落地**（§3.2 的「放在元数据」选**新表**，理由见 §8.0）：

```sql
CREATE TABLE IF NOT EXISTS sys_end_user_rules (
  project_id VARCHAR NOT NULL,
  collection VARCHAR NOT NULL,
  owner_field VARCHAR NOT NULL DEFAULT 'owner_id',
  list_rule VARCHAR NOT NULL DEFAULT 'deny',
  get_rule VARCHAR NOT NULL DEFAULT 'deny',
  create_rule VARCHAR NOT NULL DEFAULT 'deny',
  update_rule VARCHAR NOT NULL DEFAULT 'deny',
  delete_rule VARCHAR NOT NULL DEFAULT 'deny',
  updated_at TIMESTAMP NOT NULL
)
```

- 取值只有 `public|auth|owner|deny`；无行 = 全 `deny`（fail-closed）。
- 在 `DataHandler` 的 7 个文档方法开头加一次 `endUserGuard(op, collection, docID)`：`p.IsEndUser()` 或匿名时才生效，平台会话 / API Key 通道完全不变（保证 §3.2「API Key 访问不受影响」）。
- `list` 的 owner 条件走 `compileFilter` 的方言函数拼 `WHERE json_extract(data,'$.owner_id') = ?`，参数化；**不把用户输入的 filter 与 owner 条件做字符串拼接**，而是在 `ListDocuments` 已有 SQL 上 `AND` 一段预编译片段（读 `collection_filter.go:57` 的 `dialect` 分支）。
- `create` 服务端强制覆盖 `owner_field`；`update/patch` 先 `loadDocumentForWrite` 再比对 owner，防止改 `owner_id` 把数据挪给别人；`protectReadonlyFields` 现有逻辑保留。
- 对象存储：新增 `endUserObjectGuard` 薄适配层，在调用 `S3Handler` 前把 `key`/`prefix` 规范化到 `users/{EndUserID}/`，越界返回 403；不改 `S3Handler` 内部逻辑。
- **不挂载**给终端用户的：`/databases/*/query|execute|batch`、`/schema/*`、`/kv`、`/backups`、`/settings`、`/logs`、`/audit`、`/gofunctions`、`/cron-jobs`、`/sandboxes`、`/agent`、`/v1/chat/completions`、`/go`。加一张路由表测试遍历 `e.Routes()`，断言任何 `end-users/data|objects` 之外的终端用户路由都不含这些片段。

**新增系统表**（全部 `CREATE TABLE IF NOT EXISTS` 追加到 `systemdb/schema.go` 的 `systemTables`，不修改已有表）：

```text
sys_end_users(project_id, id, email, password_hash, disabled, created_at, updated_at, last_login_at)
sys_end_user_sessions(id, project_id, end_user_id, refresh_token_hash, access_jti,
                      user_agent, ip, expires_at, created_at, revoked_at)
sys_end_user_rules(...)                     # 见上
sys_project_auth_settings(project_id, enabled, signup_enabled,
                          require_email_verification, updated_at)
sys_api_key_meta(...)                       # §8.2.4
sys_audit_events(...)                       # §8.3.3
sys_alert_state(...)                        # §8.3.5
```

- email 唯一性在应用层按 `(project_id, lower(email))` 校验（DuckLake 无 PK/唯一约束）；大小写归一。
- 邮件验证 / 找回密码依赖 SMTP（§3.6 / §8.3），未配置时 `require_email_verification` 强制为 false，readiness 给警告；本批只做「未验证用户可登录但 `me.email_verified=false`」，不发信。
- `SystemSchemaFormatVersion` 保持 1（`schema_status.go` 只数表数量，新增表会让 `Expected` 自动变大，旧实例重启即补齐；`EnsureSystemSchema` 幂等，不需要 reset）。

**SDK 与文档**：

- `packages/js-sdk/src/` 新增 `auth.ts`：`signUp / signIn / signOut / onAuthStateChange / currentUser`；`client.ts` 增 `endUser: true` 模式，令牌存内存 + `localStorage`（可配），401 时自动 refresh 一次；`collections.ts` / `storage.ts` 按模式切到 `end-users/data|objects` 前缀。
- `docs/sdk/auth.md` 的「浏览器警告」改写为「前端用终端用户令牌，后端用 API Key」；`docs/getting-started/quickstart.md` 增加「带用户的应用」最短路径。
- `examples/capabilities/19-end-users`：两个终端用户互相越权访问全部 403（§3.2 验收），并额外断言终端用户令牌打 `/v1/projects/:p/settings`、`/databases/:id/query` 都是 401/403。

#### 8.2.7 代码改动清单（汇总）

| 文件 | 改动 |
| --- | --- |
| `internal/auth/{user.go,session.go,password.go}` | 引导口令注入签名、`MustChangePassword` 闭环、口令强度 |
| `internal/auth/{jwt.go,principal.go,middleware.go,keyring.go,end_user.go,end_user_repository.go}`（后 4 个新增） | 密钥域、Keyring、终端用户认证 |
| `internal/api/{auth_handler.go,api_keys_handler.go,users_handler.go,backup_handler.go,adapter.go}` | 新端点、Key meta、审计、统一 `auditActor` |
| `internal/api/{end_user_handler.go,end_user_context.go,login_guard.go}`（新） | 终端用户路由、登录防护 |
| `internal/api/{data_handler.go,collection_filter.go,s3_content_handler.go}` | `endUserGuard` / 参数化 owner 条件 / 下载头（后者见 §8.1.4） |
| `internal/api/router.go` | 免认证新端点、`/v1` 组审计中间件、终端用户组挂载 |
| `internal/app/app.go` | session secret 装配、Keyring 注入、`bootstrapPassword` 传入 `SeedInput` |
| `internal/systemdb/{schema.go,seed.go}` | 新增表、引导口令 |
| `internal/config/{config.go,yaml.go,env.go}` | `bootstrap_password` / `session_secret` / `auth.apikey_secrets` 只从 env 读 |
| `internal/agentbridge/delegation.go`、`internal/api/sandbox_handler.go` | 委托签发、envd signer 改用 session secret |
| `ui/src/{services/http-api.ts,services/types.ts,stores/auth.ts,components/modal/LoginModal.vue,locales/*}` | 423 处理、`/password/initial`、Key 列表新字段、i18n 两棵树同构 |
| `packages/js-sdk/src/*`、`docs/sdk/auth.md` | 终端用户 auth 模式 |
| `config.example.yaml`、`docs/getting-started/quickstart.md` | 新环境变量与最短路径 |

#### 8.2.8 测试

- `auth/bootstrap_user_test.go`：production 未注入拒启动、注入弱口令拒启动、已有默认口令行拒监听、development 零配置仍可种子。
- `auth/initial_password_test.go` + `api/auth_handler_test.go`：423 → `/password/initial` → 拿到 TokenPair → 旧 refresh 重放被拒；非 `must_change_password` 用户调新端点返回 409。
- `auth/keyring_test.go`：v2 签发 / v1 旧 Key 仍可用 / 未知版本 / 缓存键不串 / 8 版本上限。
- `api/api_keys_handler_test.go`：List 返回 name/prefix/last_used_at；旧行（无 meta）显示 `v1` + 空 last_used；签发明文不进审计。
- `api/audit_route_table_test.go`（新）：所有 `/v1` 写路由都在审计登记表内；`auditActor` 登录态不再显示 `system`。
- `auth/end_user_test.go` + `api/end_user_handler_test.go`：跨项目令牌、平台令牌打终端用户路由、终端用户令牌打管理面，三组全部拒绝；`public/auth/owner/deny` × 五种操作 × 集合与对象的越权矩阵（§7 风险条目要求的合并门槛）；`create` 覆盖 `owner_id`、`patch` 改 `owner_id` 被拒；匿名只允许 `public`。
- `api/delegation_route_table_test.go` 补一条：session secret 与 API Key 盐不同时，委托令牌签发 → 验签 → 路由访问仍 200（防止 §8.2.3 改坏助手 skill）。
- `ui/tests/http-api-auth.test.ts`、`ui/tests/i18n.test.ts`：423 分支与新增文案两棵树一致。
- `packages/js-sdk`：token 自动续期、切 end-user 模式后路径正确。

### 8.3 运维层（出事知不知道）

#### 8.3.0 目标与验收

1. 公网实例被爆破 / 越权 / 打爆配额时，**不用翻日志也能知道**：控制台首页有告警，`/metrics` 有计数，审计表有记录。
2. 备份**真的在跑**：默认自动定时备份，错过窗口会补跑；备份失败产生告警而不是静默。
3. 磁盘水位 / 数据库文件膨胀 / 配额耗尽有前置预警，而不是写失败时才发现。
4. 恢复演练有文档、有脚本、有端到端例子验证「备份能恢复」。
5. production 启动时做一次系统性自检（readiness），把「该配没配」的东西列成清单，小白第一次部署就能看到全貌。
6. 现有「日志只进缓存不落库」的坑修掉：`StartPeriodicFlush` 由可选变成必选。

#### 8.3.1 日志落库闭环（修当前最实际的一个坑）

现状：`systemdb.Store.RecordLog`（§7.2 P3.1）只入缓冲；只有调用方启动了 `StartPeriodicFlush` 后台才批量落 `sys_log_events`。检查 `app.go` 的装配路径：**没有统一启动 flusher**，意味着运行日志在进程正常退出时丢失、重启后不可见——「出事知不知道」的第一层就断了。

改动（最小化，不动 `RecordLog` 签名）：

| 文件 | 改动 |
| --- | --- |
| `internal/systemdb/store.go` | `StartPeriodicFlush(ctx)` 改为幂等；新增 `FlushOnShutdown`：`app.Shutdown` 里先 `FlushLogs` 再关库。`flushQueueCap` 满时丢日志计数 `droppedLog` 导出为 `simplebase_log_dropped_total` |
| `internal/app/app.go` | `Start` 中 systemdb 初始化完成后立即 `StartPeriodicFlush`（带 2s / 512 条双触发，与 §7.2 P3.2 参数一致）；`Shutdown` 中先刷日志再关 store |
| `internal/api/system_handlers.go` | `logsHTTPHandler.List` 增加调试字段（响应头 `X-SimpleBase-Log-Dropped`）只在 dev 返回 |

验收：`go test ./internal/app -run TestLogFlushOnShutdown`——进程模拟重启前后 `QueryLogs` 能查到退出前的日志；`droppedLog` 有指标。

#### 8.3.2 备份默认开启 + 补跑（§1.6 复核 R2 的落地）

现状问题（对照 `backup/service.go`、`backup/scheduler.go` 逐条核实）：

1. `backup.enabled` 默认 false，小白永远拿不到第一份备份。
2. 已有 `Scheduler`（cron 解析用仓内 `internal/utils/crontab`，不缺依赖）：`Start` 起单 goroutine 定时 `RunOnce`，逐项目调 `Service.Create`。但有三处缺口：**错误被 `_ =` 吞掉**（`scheduler.go:64`）；只按「下一次 cron 时间」推进，进程停机的窗口**错过不补**（`NextAfter(time.Now())` 从当前时刻算，停机跨过 04:00 就直接跳到明天）；`RunOnce` 全项目串行，无单飞标记，`Start` 与手动触发可能重叠。
3. `Prune` 的保留逻辑（`service.go:203`）语义缺陷：`i >= s.cfg.KeepLast && expired` 因 Go 运算符优先级实际是 `i >= KeepLast && expired`（与 `i >= MaxPerProject` 平级），排序按 `created_at DESC`，`i` 从新到旧——超过 `max_per_project` 的**立即删**（哪怕很新，硬上限优先），`KeepLast` 分支则是「序号超了**且**过期才删」。行为可以接受但**没有任何测试**覆盖三约束交叉，改一行就漂。
4. `Schedule` 为空时调度器不启动（`NewScheduler` 只在表达式非空时被调用），没有告警。

设计（复用现有 `Scheduler` / `crontab`，不重写）：

```yaml
backup:
  enabled: true                 # production 默认开（见下表）；SIMPLEBASE_BACKUP_ENABLED 可覆盖
  dir: ""                       # 空则用 data_dir/backups
  schedule: "0 4 * * *"         # 每日 04:00 本地时间
  keep_last: 7
  max_per_project: 30
  keep_days: 14
  missed_policy: catchup        # miss | catchup（默认 catchup：错过立即补跑一次）
```

| profile | `backup.enabled` 默认 | 行为 |
| --- | --- | --- |
| `development` | false | 保持现状，不额外写盘 |
| `production` | **true** | `dir` 为空时落到 `data_dir/backups`；启动时 readiness 检查可写性 |

- **改造现有 `Scheduler`**（不新建文件）：`RunOnce` 错误不再吞——逐项目失败写 `sys_audit_events(kind="backup.failed", detail=err)` 并 Emit 告警信号（§8.3.5），聚合错误照旧 `errors.Join` 返回给调用方记日志。
- **补跑（catchup）**：新增表 `sys_backup_state(project_id, last_success_at, last_attempt_at, last_error, next_due_at)`；`Scheduler.Start` 时先查 `now > next_due_at && last_attempt_at < next_due_at` 的项目立即补跑一轮再进入定时循环；每轮 `RunOnce` 后更新 `next_due_at = spec.NextAfter(now)`。连续失败按 1h → 2h → 4h 退避推迟 `next_due_at`。
- **单飞**：`Scheduler` 加 `sync.Mutex` 的 in-flight 标记，`RunOnce` 重入直接返回 nil；手动触发（`POST /v1/projects/:p/backups`）不受调度器影响（走 `Service.Create`，本来就独立）。
- cron 解析继续用 `internal/utils/crontab`（支持标准五段表达式）；`Schedule` 为空且 production 下 readiness 给 warn（「备份未调度」）。
- `Prune` 语义显式化：先按 `MaxPerProject` 截断（硬上限），再在剩余里按 `KeepDays` 过期删除，`KeepLast` 保护最近 N 份不过期删除；补表驱动测试覆盖三种约束交叉。
- 恢复脚本 `scripts/restore.sh`：`simplebase restore --project <id> --backup <id> [--target-dir]`，流程 = 停写（挂维护标志）→ 覆盖数据库文件 → 校验 SHA256 → 重启。**本批只做脚本与文档**，恢复编排不做进程内自动 failover。仓库已有 `backup/restore.go`，脚本薄封装其入口。
- `docs/ops/backup-restore.md`：怎么改窗口、怎么看下次运行时间、怎么恢复、备份包含什么（数据库文件 + manifest，**不含**对象存储数据——写明这个边界与手动导出对象的方法）。
- `examples/capabilities/17-backup-restore`：建项目 → 写数据 → `POST /v1/projects/:p/backups` → 删数据库文件 → restore → 断言数据回来。放进 CI。

#### 8.3.3 审计信号（把 §8.2.5 的记录变成「能看见的事」）

`sys_audit_events` 表（§8.2.5 已列入新表清单）：

```sql
CREATE TABLE IF NOT EXISTS sys_audit_events (
  id VARCHAR PRIMARY KEY,           -- DuckLake 下退化为普通列，唯一性由生成端保证
  occurred_at TIMESTAMP NOT NULL,
  actor VARCHAR NOT NULL,           -- user:<id> | key:<id> | enduser:<id> | system
  project_id VARCHAR NOT NULL DEFAULT '',
  kind VARCHAR NOT NULL,           -- auth.login_failed / backup.failed / api_key.issue ...
  detail VARCHAR NOT NULL DEFAULT ''
)
```

- 写入走 `systemdb.Store.RecordAudit`（与 `RecordLog` 同款缓冲 + 后台批量刷，复用 §8.3.1 的 flusher），失败不阻塞业务请求。
- 控制台「设置 → 审计」页（新 `ui/src/views/AuditView.vue`）：按时间倒序分页，筛选 actor / kind / project；superadmin 可看全系统，普通角色只看本项目。
- `GET /v1/projects/:p/audit` 挂 `require(auth.DatabaseRead)`（读权限即可，不设单独 admin 位，避免权限位膨胀）。
- **脱敏规则**：`detail` 只允许结构化短字符串（kind 相关的字段名列表、ID、错误摘要），写入端点统一走 `audit.Record` 的白名单序列化，杜绝把请求体直接塞进去的可能。

#### 8.3.4 资源水位预警（磁盘 / 存储 / 配额）

三条前置预警，全部先只做「计数 + 控制台提示」，不做自动处置：

| 信号 | 阈值 | 触发动作 |
| --- | --- | --- |
| 磁盘可用空间 | `data_dir` 所在卷 < 20%（警告）/ < 10%（严重） | 启动时与每 5min 检查一次；严重时告警信号 + `/metrics` gauge `simplebase_disk_free_ratio` |
| 数据库文件膨胀 | 单项目库文件 > 配额 80% 且 `usage.SumDatabaseStorage` 周环比增长 > 50% | 日志 + 审计 `storage.growth`，控制台项目页横幅 |
| 配额耗尽 | `usage.CheckQuota` 返回 `ErrQuotaExceeded` | 计数器 `simplebase_quota_exceeded_total{kind}`；控制台首页告警条 |

- 新增 `internal/health/disk.go`：`DiskFreeRatio(path string) (float64, error)`（`golang.org/x/sys` 若已有则用 syscall，否则 `df` 语义用 `os.Statfs`——查证 go.mod：无 x/sys 依赖，直接用 `syscall.Statfs`，标准库自带，macOS/Linux 均可）。
- `metricsHandler` 增 `/v1/system/health` 聚合端点（现有 readiness §3.1 的扩展）：返回 `{disk_free_ratio, backup_last_success_at, quota_exceeded_recent, log_dropped_total, pending_flush}`，控制台首页轮询 30s。
- 控制台：`ui/src/views/HomeView.vue` 顶部告警条组件 `AlertBanner.vue`，数据来自 health 端点 + WebSocket 推送（复用现有 realtime 通道，topic `system.alerts`）；**没有配置 WebSocket 时降级为轮询**（现状 realtime 已如此，不新增依赖面）。

#### 8.3.5 告警信号总线（进程内，不做外部依赖）

设计约束：不引入 Prometheus Alertmanager / PagerDuty 等外部组件（§6 已排除），全部进程内闭环。

```go
// internal/alerts/bus.go（新）
type Signal struct {
    Kind     string    // "auth.bruteforce" | "backup.failed" | "disk.critical" | "quota.exceeded" | "ratelimit.flood"
    Project  string
    Detail   string
    Severity string    // warning | critical
    At       time.Time
}

type Bus interface {
    Emit(sig Signal)                       // 非阻塞；内部 ring buffer（256）
    Subscribe(fn func(Signal))             // 控制台 WS、metrics、audit 三个订阅方
}
```

| 订阅方 | 行为 |
| --- | --- |
| audit 记录 | `sys_audit_events(kind="alert."+sig.Kind)`（复用 §8.3.3 刷写通道） |
| metrics | `simplebase_alerts_total{kind,severity}` 计数器 |
| 控制台 WS | 推 `system.alerts` topic；前端收到后 AlertBanner 更新并保留最近 50 条 |
| 持久化状态 | `sys_alert_state(kind, project, first_at, last_at, count, severity, detail)`——同 kind+project 60s 窗口内聚合去重，避免爆破时刷屏 |

- 触发点接线：`loginGuard`（§8.1.6）锁定时、备份失败（§8.3.2）、磁盘严重（§8.3.4）、配额拒绝（`usage.CheckQuota` 返回处）、限流 `post_auth` 维度被撞满时。
- **告警不等于通知**：进程内只保证「可见」。外部通知（邮件 / webhook）挂 SMTP 配置（§3.6），本批只在 readiness 里检查「critical 告警出现过但 SMTP 未配置」并给警告，不做强制。

#### 8.3.6 readiness 自检扩展（§3.1 的收口）

现有 readiness 只查依赖连通性。扩展为四组，全部带 `severity: ok|warn|fail`：

| 组 | 检查项 | fail 时的行为 |
| --- | --- | --- |
| 依赖 | 系统库、对象存储、（可选）SMTP —— 现有 | 拒绝启动（现状） |
| 安全 | 引导口令已改（§8.2.2）、metrics 收口（§8.1.7）、`trusted_proxies` 非空且非全网、`public_url` 为 https、`session_secret` 显式配置 | production 下 fail = 拒绝启动（详见 §8.4 决策）；dev 下只 warn |
| 备份 | `enabled=true`、`dir` 可写、上次成功时间 < 48h、`restore` 演练标记（手动确认字段） | warn + 告警信号 |
| 资源 | 磁盘 > 20%、无 `quota_exceeded` 最近 1h、`log_dropped_total` 无增长 | warn |

- `GET /v1/system/readiness` 响应结构 `{status, checks: [{name, severity, detail}]}`；控制台「设置 → 就绪」页渲染成清单，每项给一句话解释 + 跳转文档锚点。
- **小白路径**：`simplebase doctor`（新子命令）= 进程外跑同样的检查逻辑（复用 `config.Load` + 直接探测），输出人类可读清单与「下一步做什么」。这是 §1.6「出事知不知道」的入口：先让用户知道「哪里没配好」。

#### 8.3.7 代码改动清单

| 文件 | 改动 |
| --- | --- |
| `internal/systemdb/{store.go,audit.go,schema.go}` | flusher 幂等与关闭刷写、`RecordAudit`、新表（含 `sys_backup_state`） |
| `internal/backup/{service.go,scheduler.go}` | 改造现有调度器：错误上报、catchup 补跑、单飞标记；Prune 语义显式化与测试 |
| `internal/usage/service.go` | `CheckQuota` 触发告警信号（依赖注入 Bus，避免环） |
| `internal/health/disk.go`（新） | `syscall.Statfs` 封装，macOS/Linux 双平台构建标签不需要（同一 API） |
| `internal/alerts/{bus.go,store.go}`（新） | 信号总线 + `sys_alert_state` |
| `internal/api/system_handlers.go` | health / readiness 聚合端点、audit 查询端点 |
| `internal/app/app.go` | 装配：flusher、scheduler、bus 注入 metrics/audit/WS |
| `cmd/simplebase/{doctor.go,restore.go}`（新子命令） | 进程外自检、恢复脚本入口 |
| `internal/config/{config.go,backup.go,profile.go}` | 备份默认值按 profile、`missed_policy`、readiness 安全组阈值 |
| `ui/src/{views/AuditView.vue,views/HomeView.vue,components/AlertBanner.vue,services/http-api.ts}` | 审计页、告警条、health 轮询 |
| `scripts/restore.sh`、`docs/ops/{backup-restore.md,alerts.md}` | 运维文档 |
| `examples/capabilities/17-backup-restore` | 端到端备份恢复验证 |

#### 8.3.8 测试

- `systemdb/store_test.go`：flusher 幂等、shutdown 刷写、drop 计数。
- `backup/scheduler_test.go`（已有文件，补用例）：`RunOnce` 错误逐项目上报、catchup 补跑、失败退避、单飞标记、`sys_backup_state` 更新幂等。
- `backup/service_test.go` 补：Prune 三约束交叉表驱动测试（KeepLast/KeepDays/MaxPerProject）。
- `alerts/bus_test.go`：ring buffer 满不阻塞、60s 聚合去重、三个订阅方各收到一次。
- `usage/service_test.go` 补：配额拒绝时 Emit 被调用（mock Bus）。
- `health/disk_test.go`：临时目录可写、`DiskFreeRatio` 返回 (0,1]。
- `api/system_handlers_test.go` 补：readiness 四组结构、health 聚合字段。
- `cmd/simplebase/doctor_test.go`：对一份故意缺配置的 YAML 输出 warn 清单（快照断言）。
- `examples/capabilities/17-backup-restore`：见 §8.3.2。
