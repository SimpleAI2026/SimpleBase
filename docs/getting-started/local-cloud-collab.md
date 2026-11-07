---
title: 本地开发、云端重放
order: 5
description: 在本地用工作区开发迁移、云函数和定时任务，再用同一套命令幂等重放到云端 SimpleBase。
---

# 本地开发、云端重放

业务在本地开发和验证，成果放在一个可以进 git 的工作区里，再用同一套命令重放到云端。业务代码只换 `SIMPLEBASE_URL` 和 `SIMPLEBASE_API_KEY`，不改代码路径。

## 工作区

```text
my-app/
  simplebase.json        # 清单（非密钥）
  migrations/001_init.sql
  functions/hello.go
  seed/dev.sql           # 可选，仅 --seed 时执行
  .env                   # 不提交：本地 URL / Key / 项目 ID
  .env.cloud             # 不提交：云端同名变量
  .simplebase/lock.json  # 各环境的库 ID、迁移 checksum、函数版本（无密钥，可提交）
  .simplebase/env.local  # apply 生成：URL、项目 ID、库 ID（无密钥）
```

清单字段、忽略项和校验规则见 `skills/simplebase/references/manifest.md`。`examples/*/simplebase.json` 是同一格式。

## 前提

- 本地和云端用**同一个项目 ID**（8 位字母、数字或 `-`）。项目和 Key 需要在各自环境的控制台创建，重放器不建项目。
- 发布用的 Key 需要 `database:admin`（建库，并隐含函数、定时任务所需的 `database:write` 与读取所需的 `database:read`）。业务运行时的 Key 只给 `database:read` / `database:write`，不要复用发布 Key。
- 云端和本地的数据库引擎（DuckDB 或 SQLite）要一致，迁移只按一种方言写。

## 本地开发循环

```bash
export SIMPLEBASE_URL=http://127.0.0.1:8080
export SIMPLEBASE_API_KEY=...        # 控制台「设置 → 连接」签发
export SIMPLEBASE_PROJECT_ID=myapp001

simplebase dev validate --dir .      # 离线校验（不需要凭据）
simplebase dev plan  --dir .         # 看会做什么
simplebase dev apply --dir .         # 本地直接执行
simplebase function invoke --name hello --export Hello --input '{"name":"sb"}'
```

重放固定按「库 → 迁移 →（种子）→ 集合 → 函数 → 定时任务」进行，每一步先查后写，输出 `create` / `update` / `apply` / `skip`。中断后原样重跑即可接着做；连续执行两次，第二次全部是 `skip`。

迁移只追加：已应用的文件改了内容会报 `migration_modified`，要改表结构就新增 `002_xxx.sql`。迁移里不写 `PRIMARY KEY`、`UNIQUE`、`CREATE INDEX`、`CREATE SEQUENCE`（DuckLake 不支持），每个文件最多 99 条语句，不使用 `ATTACH`、`COPY`、`SET`、`PRAGMA`、`CALL` 等被 SQL 守卫拒绝的语句。语句按 `;` 拆分时会跳过引号（`'...'`、`"..."`、`$$...$$`）与 `--` / `/* */` 注释里的分号，所以字面量里可以写 `;`。

云函数按源码 sha256 比较：相同则跳过，不会每次都新建版本（每个函数最多 50 个版本）。源码变化时先建未生效的新版本，用清单里的 `test` 试跑，通过后才生效，所以试跑失败的版本不会上线。

## 发布到云端

```bash
set -a; . ./.env.cloud; set +a        # 云端的 URL / Key / 项目 ID

simplebase dev plan  --dir .          # 把差异给团队确认
simplebase dev apply --dir .          # 远端不带 --yes：只输出计划，退出码 3，不写入
simplebase dev apply --dir . --yes    # 确认后执行
```

重放**不会**删除云端多余的资源，不会触发定时任务，也不搬数据。种子只在显式 `--seed` 时执行，开发夹具不要对生产使用。

`apply` 结束后 `.simplebase/env.<label>` 里有目标环境的 URL、项目 ID 和库 ID（例如 `SIMPLEBASE_DATABASE_ID_MAIN`）。库 ID 在本地和云端各不相同，业务启动时可以按库名解析，或读这个文件。

## 切换环境

业务服务端只需要换两个变量：

```bash
SIMPLEBASE_URL=https://cloud.example.com
SIMPLEBASE_API_KEY=...               # 云端控制台签发的运行时 Key
```

前端不直连：Key 不进浏览器，沿用 [`examples`](https://github.com/linkxzhou/SimpleBase/tree/main/examples) 中 BFF 的做法。

## CLI 与脚本

| 入口 | 说明 |
|---|---|
| `simplebase dev validate\|plan\|apply\|status` | 二进制自带，适合 CI。连接信息来自 flag、环境变量或 `config.json`，不读 `.env`；`validate` 与 `status` 不需要连接配置 |
| `skills/simplebase/scripts/sbdev.py` | Claude Code skill 使用的 Python 版，行为一致，另有 `init`（生成骨架）、`doctor`（自检）并自动读取工作区 `.env` |

两者用同一份清单和 lock 格式，并共用 `internal/sbcli/testdata/dev` 里的 fixture 对账。

## 退出码

| 码 | 含义 |
|---|---|
| 0 | 成功 |
| 1 | 校验或用法错误，包括 `migration_modified` |
| 2 | 远端错误，输出里有 `error.code` |
| 3 | 远端 `apply` 需要 `--yes` |
| 4 | 缺少连接配置（`validate` / `status` 不需要） |

## 常见问题

| 现象 | 处理 |
|---|---|
| `workspace_invalid` | 按错误列表修清单、迁移或函数；疑似密钥会直接失败，把密钥换成环境变量 |
| `migration_modified` | 恢复原文件，把改动写进新的迁移 |
| `data_model_mismatch` | 远端同名库的类型与清单不一致，改库名或修正 `data_model` |
| `forbidden` | 发布 Key 缺少 `database:admin`（或运行时 Key 缺少所需的 read/write） |
| `writer_unavailable` | 目标实例只读，换可写实例 |
| 退出码 3 | 远端 apply 缺 `--yes`，确认计划后加上 |
| `project_not_found` | 目标实例上还没有这个项目，在控制台用同一个项目 ID 建项目并签发 Key |
| 提示两个环境引擎不一致 | 迁移只按一种方言写，把两个项目的数据库引擎调成一致 |

## 参考

- [CLI](/docs/cli)：`dev` 子命令参数与输出。
- [云函数](/docs/automation/gofunction)、[定时任务](/docs/automation/cronjob)：函数约束与调度规则。
- [数据库引擎与约束](/docs/data/ducklake-notes)：迁移里不能用的语法。
