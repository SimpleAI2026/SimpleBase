# SimpleBase

基于 Turso/libSQL、S3 在线持久层与 Go 构建的云端数据库服务，并集成统一 LLM 转发能力。

## 架构概览

```text
 Clients / SDK / CLI
          │
          ▼
┌─────────────────────────────────────────────────────┐
│ SimpleBase Server（首期唯一可写实例）              │
│                                                     │
│ API Gateway                                         │
│ ├─ Auth / Project / Quota / Audit                   │
│ ├─ Database Management API                          │
│ ├─ SQL Query API                                    │
│ └─ LLM Gateway API                                  │
│                                                     │
│ Database Runtime                 LLM Runtime         │
│ ├─ DB Registry（每库唯一 writer）├─ litellm Router  │
│ ├─ libsql / database/sql         ├─ Provider Keys   │
│ ├─ Local Cache Manager           ├─ Streaming       │
│ └─ Recovery Manager              └─ Usage Metering  │
└───────────────┬──────────────────────────┬──────────┘
                │                          │
                ▼                          ▼
       S3 Compatible Storage        LLM Providers
       在线持久数据与元数据          OpenAI/Anthropic/...
```

### 核心设计

- **S3 是在线持久层**：本地磁盘仅作缓存与工作集；节点丢失本地数据后可仅凭 S3 恢复。
- **单写实例**：首期固定副本数为 1，所有写请求经同一进程的 Registry 路由，保证单库唯一 writer。
- **Turso/libSQL 驱动**：通过标准 `database/sql` 使用，业务模型不直接依赖驱动包。
- **项目隔离**：每个 tenant/project 拥有独立 logical database 与 S3 对象前缀，认证、配额、审计按 project 划分。
- **LLM Gateway**：复用 `/litellm` 多供应商客户端，服务端封装鉴权、配额、流式转发与用量计量。

## 快速开始

### 构建

```bash
go build -o simplebased ./cmd/simplebased
```

### 配置

复制 `config.example.yaml` 为 `config.yaml` 并按环境调整。所有字段可由 `SIMPLEBASE_` 前缀环境变量覆盖。生产凭据（S3 密钥、catalog token、server_secret）必须从环境变量或 IAM 角色注入，禁止写入配置文件。

### 运行

```bash
./simplebased
```

默认监听 `:8080`。健康检查：

- `GET /health/live` — 进程存活
- `GET /health/ready` — Catalog 可用 + S3 可达

### Docker

```bash
docker build -t simplebased .
docker run -p 8080:8080 --env-file .env simplebased
```

## API

所有业务路由位于 `/v1/projects/:projectID/...`，需 `Authorization: Bearer <api-key>` 认证并按 project 上下文校验权限。

### 数据库管理

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| POST | `/v1/projects/:p/databases` | `database:admin` | 创建 logical database |
| GET | `/v1/projects/:p/databases` | `database:read` | 列出 project 下数据库 |
| GET | `/v1/projects/:p/databases/:id` | `database:read` | 查询状态与容量 |
| POST | `/v1/projects/:p/databases/:id/open` | `database:admin` | 预热数据库 |
| POST | `/v1/projects/:p/databases/:id/close` | `database:admin` | 释放本地资源 |
| POST | `/v1/projects/:p/databases/:id/backups` | `database:admin` | 创建独立恢复点 |
| POST | `/v1/projects/:p/databases/:id/restore` | `database:admin` | 恢复为新库后受控切换 |
| DELETE | `/v1/projects/:p/databases/:id` | `database:admin` | 软删除，异步清理 S3 |

### SQL 执行

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| POST | `/v1/projects/:p/databases/:id/query` | `database:read` | 参数化查询 |
| POST | `/v1/projects/:p/databases/:id/execute` | `database:write` | 单条写语句 |
| POST | `/v1/projects/:p/databases/:id/batch` | `database:write` | 批量事务 |

约束：参数化 SQL、强制 context deadline、最大返回行数、请求体大小与并发数限制。DDL/恢复/删除单独授权并写入审计日志。

### LLM Gateway

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| POST | `/v1/projects/:p/llm/chat` | `database:read` | 非流式 Chat |
| POST | `/v1/projects/:p/llm/stream` | `database:read` | SSE 流式响应 |
| GET | `/v1/projects/:p/llm/providers` | `database:read` | 列出允许的 provider/model |

### 配额与审计

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/v1/projects/:p/quota` | `database:read` | 当前配额使用 |
| GET | `/v1/projects/:p/audit` | `database:read` | 操作审计记录 |

## 部署

详见 [docs/deployment.md](docs/deployment.md)。关键约束：

- **副本数固定为 1**：滚动升级必须先停旧实例、再启新实例。
- **S3 bucket 私有**：启用 TLS、SSE-KMS、最小权限 IAM、版本控制。
- **Catalog 独立库**：与用户库使用不同前缀和权限。
- **LLM 密钥**：来自 KMS 或加密配置，日志仅输出 provider/key ID 摘要。

## 迁移

从旧版本（LessDB/ha-sqlite）迁移到新链路的工作流详见 [docs/migration-guide.md](docs/migration-guide.md)。

## 文档

- [内部模块说明](internal/README.md)
- [部署文档与 runbook](docs/deployment.md)
- [迁移指南](docs/migration-guide.md)
- [重构计划](plan/plan.md)

## 目录结构

```text
cmd/simplebased/        服务入口
internal/api/           HTTP 路由、请求校验、错误协议、流式响应
internal/app/           依赖装配与生命周期
internal/auth/          API Key、项目身份、权限与上下文
internal/audit/         不可变审计事件与脱敏
internal/catalog/       元数据：tenant/project/database/LLM 配置/usage
internal/config/        配置加载
internal/database/      libsql 连接、事务、查询、错误映射
  ├─ registry/          进程内每库唯一 writer、引用计数、空闲卸载
  ├─ cache/             本地缓存目录、配额、LRU 淘汰
  ├─ turso/             DSN 构建与连接工厂
  ├─ sqlguard/          SQL 执行边界（超时、行数、批量）
  └─ serialize.go       行序列化
internal/deploy/        Preflight 检查（缓存可写、S3 可达、flock）
internal/jobs/          持久化异步任务（删除/备份/恢复/校验）
internal/llmgateway/    litellm 服务端封装、流式转发、用量计量
internal/objectstore/   S3 client、KeyBuilder、descriptor、health
internal/observability/ 日志、指标、健康检查
internal/usage/         配额检查与用量聚合
litellm/                多供应商 LLM 客户端（独立保留）
```

## 技术栈

- Go 1.25+
- Turso/libSQL（驱动名 `libsql`）
- S3 兼容存储（AWS SDK v2）
- Echo v4 HTTP 框架
- litellm v1.5.8（LLM 客户端）
- Zap 结构化日志
- Prometheus 指标
