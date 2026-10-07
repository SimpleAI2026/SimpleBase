---
title: 5 分钟上手
order: 2
---

# 5 分钟上手

本示例只面向本地 `dev_mode: true`。生产环境**不要**使用内置开发 Key 或示例密码。

## 1. 启动本地服务

在仓库根目录执行：

```bash
./build.sh dev
```

开发模式启动后，打开控制台（以终端显示的地址为准）。本地初始化会建立普通项目 `dev-shop`；控制台可使用开发环境中的引导账号 `simplebase2026` 和同名初始密码登录，首次登录后按提示修改密码。CLI 示例使用本地种子 API Key：

```bash
export BASE=http://127.0.0.1:8080
export PROJECT=dev-shop
export SB_KEY=sb_live_dev_key_12345
```

此 Key 只用于本地开发；生产环境请在项目管理中签发具备所需权限的 Key 并从可信环境变量注入。浏览器生产包中不得嵌入写权限 Key。

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

预期 `columns` 含 `answer`，`rows` 含 `2`。写入 SQL 用同一路径下的 `/execute`（需要 `database:write`），批量语句用 `/batch`。详情见 [SQL 与集合](/docs/database/sql)。

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

第二条请求返回 `world`。详见 [Key-Value](/docs/database/kv)。

## 下一步

[JavaScript SDK](/docs/sdk/quickstart) 和 [Go SDK](/docs/sdk/go-quickstart) 提供带类型的调用；线上部署先看 [部署与故障处理](/docs/ops/deployment)。上面的数据库创建和查询步骤需要服务已正确配置数据库后端；若本地缺少后端服务，先按部署文档启动所需组件。
