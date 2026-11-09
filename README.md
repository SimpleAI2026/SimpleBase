# SimpleBase

一个二进制起一套后端：数据库（SQL / 集合 / KV）、对象存储、Go 云函数、定时任务、事件触发器、实时订阅、备份恢复、LLM 网关和云助手，自带 Vue 控制台与 CLI。本地用本地盘跑，生产切到 S3 做在线持久层，业务代码只换 URL 和 Key。

## 亮点

| 亮点 | 说明 |
| --- | --- |
| **单二进制、零外部依赖起步** | `./build.sh dev` 一条命令拉起后端与控制台；本地默认本地盘，不需要 Postgres / Redis / S3。前端经 `go:embed` 打进同一个二进制 |
| **每项目一个单文件库** | 项目数据落在一个 DuckDB 或 SQLite 文件里，SQL 表、集合文档、KV 共用；S3 模式以「不可变快照 + manifest」同步，节点丢盘可只凭 S3 恢复 |
| **SQL + 文档 + KV 三种模型同库** | 参数化 SQL / 批量事务；JSON 集合支持 filter、投影、patch、导入导出、字段 schema；KV 支持 String / Hash / List / Set / ZSet / TTL |
| **进程内 Go 云函数** | 自研 `gofunction` 解释器，单文件 `package main` 即可部署；版本、试跑、激活分离，试跑不过不上线；`/go/:project/:fn/:export` 直接 HTTP 调用 |
| **自动化全家桶** | cron / interval / once 定时任务、记录与对象事件触发器、SSE 实时订阅（支持 `Last-Event-ID` 回放）、项目级备份与恢复 |
| **本地开发、云端幂等重放** | `simplebase.json` 声明库、迁移、集合、函数、定时任务；`simplebase dev plan/apply` 先查后写，重复执行全部 skip，迁移只追加、改动已应用迁移会被拒绝；远端写入必须 `--yes` |
| **LLM 网关 + 云助手** | OpenAI 兼容 `chat/completions`、`responses`、`models`；云助手经 CLI 技能操作项目资源，破坏性命令需发起人确认，只读助手 fail-closed |
| **安全默认值** | API Key HMAC 存储、权限位 + Scoped Key（资源 / 动作 / 字段级）、跨项目 403、系统库只读、SQL 守卫（拒绝清单 / 多语句 / 行数与超时）、密钥不得进 YAML、审计与日志脱敏 |
| **可验证的样例** | `examples/` 4 个业务站点 + 15 个能力用例，`scripts/examples-verify/verify.sh` 双实例模拟「本地 → 云端」逐站验证 |

## 架构概览

```text
 Console (Vue, go:embed) / SDK (JS·Go) / CLI (simplebase <resource> <action>)
          │
          ▼
┌──────────────────────────────────────────────────────────────┐
│ SimpleBase Server（单写实例）                                 │
│                                                              │
│ API：Auth · Project · API Key/Scope · Quota · Audit · Logs    │
│      SQL · Collections · KV · Storage · Realtime(SSE)         │
│      Go Functions · Cron · Triggers · Backups · Sandbox(E2B)  │
│      LLM Gateway (OpenAI 兼容) · Cloud Agent                  │
│                                                              │
│ Runtime                                                      │
│ ├─ godatabase：项目单文件库（DuckDB/SQLite）· Registry · KV · 租约 │
│ ├─ 系统库：DuckLake（用户、Key、函数、任务、审计、日志）         │
│ ├─ gofunction：Go 解释器（云函数 / cron / 触发器共享并发上限）   │
│ └─ gocloudagent：云助手运行时（技能、确认、委托令牌）            │
└──────────────┬──────────────────────────────┬────────────────┘
               ▼                              ▼
     本地盘 或 S3 兼容存储                 LLM Providers
     快照 + manifest / 对象文件            OpenAI / Anthropic / ...
```

### 核心约束

- **单写实例**：副本数固定为 1；多实例需显式开启写租约（`instance.lease.enabled`），条件写探针失败时拒绝启动。
- **每项目一个用户数据库**：建第二个库返回 `database_already_exists`。删库只清空该库的表、视图与集合，同项目的 KV、对象文件与备份保留。
- **系统库保护**：只允许 SELECT，禁止写入与删除。
- **权限层级**：`database:admin` 隐含 `database:write` 与 `database:read`；`write` 不隐含 `read`。
- **配置来源唯一**：代码默认值 → `config.yaml` → `SIMPLEBASE_*` 环境变量；YAML 中出现非空密钥直接拒绝启动。

## 快速开始

```bash
./build.sh dev --no-open      # 后端 :8080 + 控制台 :5173；缺 config.yaml 时自动从示例生成
```

首次用引导账号 `simplebase2026` / `simplebase2026` 登录（首次登录必须改密），右上角创建项目，在「设置 → 连接」签发 API Key。完整步骤见 [5 分钟上手](docs/getting-started/quickstart.md)。

```bash
./build.sh                    # 前端构建 + go build -o simplebase
./simplebase                  # 默认 :8080；/health/live、/health/ready
docker build -t simplebase . && docker run -p 8080:8080 --env-file .env simplebase
```

### 本地开发、云端重放

```bash
simplebase project create --id myapp001 --name "My App"   # 管理员在本地与云端各建一次同 ID 项目
simplebase dev validate --dir .                           # 离线校验，不需要凭据
simplebase dev plan     --dir .                           # 只读列出 create/update/apply/skip
simplebase dev apply    --dir . --seed                    # 本地直接执行
SIMPLEBASE_URL=https://cloud.example.com simplebase dev apply --dir . --yes   # 远端必须 --yes
```

教程见 [本地开发、云端重放](docs/getting-started/local-cloud-collab.md)；AI 编码代理使用 [`skills/simplebase`](skills/simplebase/SKILL.md)。

### 样例与验证

```bash
./examples/init.sh                                         # 初始化 4 个业务站点
./examples/run_all.sh --start --skip-agent --api-port 18087 # 能力用例 01–15
scripts/examples-verify/verify.sh --ports 18090,18091      # 双实例逐站验证（本地 → 模拟云端）
```

## API 速览

业务路由位于 `/v1/projects/:projectID/...`，`Authorization: Bearer <api-key>`，按 project 上下文鉴权。

| 能力 | 主要路径 | 权限 |
| --- | --- | --- |
| 数据库 | `POST/GET/DELETE /databases` | 建删 `database:admin`，读 `database:read` |
| SQL | `/databases/:id/query`、`/execute`、`/batch` | 读 / 写 |
| 集合 | `/databases/:id/data/collections/:c/documents`（含 batch / export / import） | 读 / 写 |
| KV | `POST /kv` | 读 / 写 |
| 对象存储 | `/s3/objects`、`/s3/presign`、`/s3/link` | 读 / 写 |
| 云函数 | `/gofunctions`、`POST /go/:projectID/:name/:export` | 写 / 读 |
| 定时任务 / 触发器 | `/cron-jobs`、`/function-triggers` | 写 / `project:admin` |
| 实时订阅 | `GET /realtime`（SSE） | 读 |
| 备份 | `/backups`（需 `backup.enabled`） | `project:admin` |
| LLM | `/v1/chat/completions`、`/v1/responses`、`/v1/models` | `llm:invoke` |
| 云助手 | `/agents`、`/agent-threads/:id/runs` | 读 / 写 |
| 配额与审计 | `/quota`、`/audit`、`/logs` | 读 |

完整说明见 [`docs/`](docs/README.md) 与 [`docs/cli.md`](docs/cli.md)。

## 部署

详见 [docs/ops/deployment.md](docs/ops/deployment.md)。要点：副本数为 1（滚动升级先停旧再起新）；S3 bucket 私有并开启 TLS、SSE-KMS、版本控制；`auth.api_key_hash_secret` 只从环境变量注入，更换会使全部 Key 失效；浏览器直连需显式开启 CORS，且前端生产包不得包含写权限 Key。

## 目录结构

```text
cmd/simplebase/         服务与资源 CLI 统一入口（含 simplebase dev）
internal/api/           HTTP 路由、请求校验、错误协议、流式响应
internal/app/           依赖装配与生命周期（唯一装配点）
internal/auth/          用户、会话、API Key、权限层级与 Scope
internal/catalog/       tenant / project / database 元数据
internal/sbcli/         资源 CLI 与 dev 重放
internal/agentbridge/   云助手宿主适配
godatabase/             数据库运行时（独立模块）：单文件库 / DuckLake / Registry / KV / 租约 / SQL 守卫
gofunction/             Go 解释器（独立模块）
gocloudagent/           云助手运行时（独立模块）
gosandbox/              沙盒执行层（独立模块）
ui/                     Vue 控制台与文档站
packages/               Go 与 JavaScript SDK
skills/simplebase/      本地开发与云端重放 skill（sbdev.py）
examples/               业务站点与能力用例
scripts/                测试、冒烟与 examples-verify
docs/  plan/            用户文档 / 设计计划
```

## 技术栈

Go 1.25+（CGO）· DuckDB / SQLite / DuckLake · Echo v4 · AWS SDK v2（S3 兼容）· litellm + CloudWeGo Eino · Zap · Prometheus · Vue 3 + Vite + Tailwind
