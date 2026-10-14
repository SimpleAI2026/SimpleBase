---
title: 5 分钟上手
order: 2
---

# 5 分钟上手

本示例面向本地开发（`storage.database/files: local`，默认值）。生产环境**不要**使用示例密码，凭据一律从环境变量注入。

## 1. 启动本地服务

在仓库根目录执行：

```bash
./build.sh dev
```

启动后打开控制台（以终端显示的地址为准）。首次启动请先用引导账号 `simplebase2026` / 同名初始密码登录（首次登录后按提示修改密码），再在右上角创建项目。API Key 请在控制台「设置 → 连接」中点「重置」为项目签发：

```bash
export BASE=http://127.0.0.1:8080
export PROJECT=<你的项目 ID>
export SB_KEY=<控制台签发的 API Key>
```

浏览器生产包中不得嵌入写权限 Key。

## 2. 创建数据库

```bash
curl -sS -X POST "$BASE/v1/projects/$PROJECT/databases" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{"name":"quickstart"}'
```

响应中读取 `id`（数据库 ID）；创建数据库需要 `database:admin`。不同环境的 `id` 不同，请替换：

```bash
export DATABASE='上一步返回的数据库 id'
```

## 3. 运行第一条 SQL

```bash
curl -sS -X POST "$BASE/v1/projects/$PROJECT/databases/$DATABASE/query" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{"sql":"SELECT 1 + 1 AS answer","max_rows":10}'
```

预期 `columns` 含 `answer`，`rows` 含 `2`。写入 SQL 用同一路径下的 `/execute`（需要 `database:write`），批量语句用 `/batch`。详情见 [SQL 与集合](/docs/data/sql)。

## 4. 写入项目级 Key-Value

项目 KV 无需数据库 ID：

```bash
curl -sS -X POST "$BASE/v1/projects/$PROJECT/kv" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{"type":"cmd","argvs":["SET","quickstart:hello","world"]}'
curl -sS -X POST "$BASE/v1/projects/$PROJECT/kv" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{"type":"cmd","argvs":["GET","quickstart:hello"]}'
```

第二条请求返回 `world`。详见 [Key-Value](/docs/data/kv)。

## 下一步

[JavaScript SDK](/docs/sdk/quickstart) 和 [Go SDK](/docs/sdk/go-quickstart) 提供带类型的调用；线上部署先看 [部署与故障处理](/docs/ops/deployment)。上面的数据库创建和查询步骤需要服务已正确配置数据库后端；若本地缺少后端服务，先按部署文档启动所需组件。
