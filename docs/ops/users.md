---
title: 用户与角色
order: 5
---

# 用户与角色

管理界面使用登录态控制人员访问；API Key 用于机器到机器调用，二者不能混用为同一密钥。超级管理员通过本地初始化创建，普通用户创建接口不能再创建超级管理员。

## 三种角色

| 角色 | 主要用途 |
|---|---|
| `superadminl1` | 初始平台维护账号，可管理所有项目与用户 |
| `admin` | 管理员查看权限；控制台内以只读为主 |
| `user` | 所属项目的日常读写，不能绕过项目边界 |

权限还受用户所属项目与接口的权限要求共同约束；登录态能看到某项目不代表具备该项目所有敏感操作能力。系统项目 `sb-admin` 与普通项目分开保护。人员管理使用 `/v1/users`，列表、创建、详情、修改、删除在服务端进行角色校验；登录入口为 `POST /v1/auth/login`，当前账号信息为 `GET /v1/auth/me`。

## API Key

`GET` / `POST /v1/projects/:projectID/api-keys` 查看或签发项目 Key；`DELETE /api-keys/:keyID` 撤销。权限包括 `database:read`、`database:write`、`database:admin`、`project:admin` 等；创建数据库需 `database:admin`，执行写入需 `database:write`，修改项目日志保留期需 `project:admin`。Key 明文通常只在创建时返回一次，应保存在可信的服务器环境，不写进网页或日志。

## 本地引导与生产要求

`dev_mode: true` 仅用于本地联调；示例 Key 与引导用户密码不能进入生产。部署后先更换引导密码，再按最小权限创建业务 Key。Web 前端不能在公开资源中嵌入读写权限密钥；细节见 [SDK 鉴权与安全](/docs/sdk/auth) 与 [核心概念](/docs/getting-started/concepts)。
