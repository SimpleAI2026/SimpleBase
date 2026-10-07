---
title: DuckLake 使用须知
order: 4
---

# DuckLake 使用须知

本页描述 **SimpleBase 中**的 DuckLake 用法；[DuckLake 上游参考](/docs/database/ducklake)是离线镜像，不等同于 SimpleBase 已实现或允许使用的所有能力。

## 唯一性与写入

DuckLake 当前不提供可靠的 `PRIMARY KEY`、`UNIQUE`、索引或序列约束；即使 SQL 声明唯一性，也不能依赖它防止重复行。业务主键可用 UUID；幂等写入考虑 `MERGE INTO ... ON <业务键>`，或借助项目 [Key-Value `SET NX`](/docs/database/kv) 控制重复请求。单进程内「先查再写」仍须处于正确的事务和单写部署约束内，并非跨实例唯一保证。

## SQL 安全边界

通过 HTTP `/query` 的 SQL 必须是只读查询；`/execute`、`/batch` 要求写权限。应用层 `sqlguard` 限定语句意图并禁止访问系统库与敏感路径；用户库中的表、集合也受项目和数据库 ID 的归属校验。拒绝的 SQL 不会因为在浏览器 SQL 工作台粘贴而绕过服务端校验。

## 单写与远端同步

一个逻辑数据库同时只能有一个写实例；不能通过多开副本绕过限制。写入先本地提交，远端对象存储同步有延迟，其他读实例看到的数据可能滞后。遇到延迟或故障时，不要将远端对象立即可见当作写入已持久确认的唯一依据；运维细节见[部署指南](/docs/ops/deployment)。

## 适用场景

复杂查询使用[SQL 与集合](/docs/database/sql)；简单键值状态用[项目级 KV](/docs/database/kv)。上游镜像仅供 DuckLake 通用语法参考，SimpleBase 的权限、限制和 API 行为以本指南和服务端错误为准。
