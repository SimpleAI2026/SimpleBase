---
title: 核心概念
order: 3
---

# 核心概念

## 项目与数据库

项目是请求、权限与资源的隔离边界。HTTP 路径使用 `/v1/projects/:projectID`；项目内可建立多个用户数据库。SQL 和文档接口还需要数据库 ID，而 [Key-Value](/docs/database/kv)、[对象存储](/docs/storage)、[云函数](/docs/gofunction/overview)和[云沙盒](/docs/sandbox)只需项目 ID。请求不能越权访问其他项目的数据。

## 系统项目

`sb-admin` 是系统项目，承载平台内部元数据，不是普通业务项目；创建应用数据时请选普通项目。控制台管理员可查看部分系统信息，但系统项目受额外写入保护。

## 用户与 API Key

控制台用户通过登录建立会话；脚本与服务端应用使用 `Authorization: Bearer <API_KEY>`。API Key 归属于项目并由权限位限定操作范围；常见权限有 `database:read`、`database:write`、`database:admin`、`project:admin`。写操作需要相应写权限；创建数据库等管理操作需要 `database:admin`。生产 Key 只能保存在可信后端或密钥服务中，不能写进公开网页。

## 持久化与执行

用户数据在 DuckLake 中；对象存储经 S3 协议访问；云函数负责短时 Go 代码调用；云沙盒提供独立 Linux 环境用于脚本执行，命令与输出不写入系统数据库。部署时遵循[单写实例约束](/docs/ops/deployment)。
