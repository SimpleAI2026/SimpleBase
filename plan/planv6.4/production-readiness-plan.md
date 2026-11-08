# SimpleBase 生产就绪：让「demo 能跑」变成「敢上线」（planv6.4 下一迭代）

> **状态：** 规划（未实施）
> **日期：** 2026-11-08
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

### 1.2 身份层（谁在调用）

| 能力 | 现状 | 判定 |
| --- | --- | --- |
| 平台用户（控制台） | argon2id、refresh 会话、首次登录强制改密 | ✅ |
| 引导账号 | `simplebase2026/simplebase2026` 固定写在代码，首登改密 | ⚠️ 公网实例首登前存在窗口 |
| API Key | HMAC 存储、可吊销、Scoped（资源/动作/字段/过期）、权限层级 | ✅ |
| `api_key_hash_secret` 轮换 | **一换全部 Key 失效**，无版本化 | ❌ |
| **项目终端用户**（应用的用户注册/登录） | **没有**。只有平台用户和 API Key | ❌ **最大缺口** |
| MFA / SSO（控制台） | 无 | ❌ |
| 项目成员与角色 | 只有 owner（`sys_project_owners`） | ❌ |

**后果**：小白做一个「用户能登录、只能看自己数据」的应用，只有两条路——①前端带只读 Key（所有人看到所有数据），②自己写 BFF 再实现一套账号体系。AI 生成的 demo 几乎都会走①，`docs/sdk/auth.md` 的「浏览器警告」就是在描述这个坑。

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
| **M1 安全默认值** | §3.1 生产模式与 readiness、§3.4 观测收口、引导账号加固 | 4–5d | — |
| **M2 代码执行安全** | §3.3 出网控制 + secrets + 源码密钥扫描 | 5–6d | M1（production 默认 allowlist） |
| **M3 终端用户** | §3.2 注册登录、项目 JWT、集合 owner 规则、对象前缀规则、JS SDK、新示例站点 | 10–12d | M1 |
| **M4 数据安全网与告警** | §3.5 + §3.6 + §3.7 | 8–10d | M2（secrets 存 Webhook） |
| **M5 上线闭环** | §3.8 + §3.9 + §3.10 | 10–12d | M3、M4 |

M1、M2 可并行开发，M3 是最大块。建议本迭代交付 **M1–M3**，M4 进入下一迭代前半段。

**每个里程碑的验收方式**：

- 后端单测 + handler 测试（fake service）+ 系统库拒绝用例（沿用 AGENTS.md 要求）。
- `examples/capabilities` 新增对应用例：`16-production-profile`、`17-egress`、`18-secrets`、`19-end-users`、`20-alerts`，接入 `run_all.sh`。
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
