---
title: 备份与恢复
order: 3
group: 部署与故障处理
description: 配置用户项目单文件数据库快照，并通过控制台或 HTTP API 下载、恢复和管理备份。
---

# 备份与恢复

SimpleBase 备份是用户项目 DuckDB/SQLite 单文件数据库的一致性快照，包含创建时间、引擎、大小、SHA-256 和格式版本等元数据。系统库不包含在备份中，对象存储里的独立文件也不会随数据库快照打包。

## 启用与配置

备份功能默认关闭。在 `config.yaml` 中启用并重启服务：

```yaml
backup:
  enabled: true
  dir: "./backups"
  upload_to_s3: false
  schedule: "17 3 * * *"
  keep_last: 7
  keep_days: 30
  max_per_project: 14
  snapshot_timeout: 10m
```

| 配置 | 作用 |
|---|---|
| `enabled` | 开放备份 API 与定时备份；关闭时接口返回 503 `backup_disabled` |
| `dir` | 本地快照与 manifest 目录，也是 S3 下载回本地的位置 |
| `upload_to_s3` | 同时上传到 `{s3.prefix}/_backups/<projectID>/`；需要可用的 S3 存储 |
| `schedule` | 五段 cron，按 UTC 执行；空字符串关闭自动备份 |
| `keep_last` | 每个项目至少保留最近 N 份，即使它们已超过 `keep_days` |
| `keep_days` | 清理超出最近 `keep_last` 份且超过保留天的快照；`0` 不按天数清理 |
| `max_per_project` | 每项目硬上限，必须不小于 `keep_last` |
| `snapshot_timeout` | 单次快照超时，应按数据库大小和存储延迟调整 |

对应配置也可由 `SIMPLEBASE_BACKUP_*` 环境变量覆盖。生产环境应把 `dir` 放在持久卷上；只使用本地目录时，节点和磁盘丢失会同时丢失备份。

## 创建与管理备份

控制台「设置 → 备份」可创建、下载、恢复和删除当前普通项目的备份。HTTP 接口均需要 `project:admin`，前缀为 `/v1/projects/:projectID/backups`：

| 操作 | 方法与路径 | 成功响应 |
|---|---|---|
| 列表 | `GET /` | 200，返回 `items` |
| 创建 | `POST /` | 201，返回快照元数据 |
| 下载 | `GET /:id/download` | 200，二进制文件流 |
| 删除 | `DELETE /:id` | 204 |
| 恢复 | `POST /:id/restore` | 204 |

创建快照：

```bash
curl -sS -X POST "$BASE/v1/projects/$PROJECT/backups" \
  -H "Authorization: Bearer $SB_KEY"
```

从响应保存 `id`，然后下载：

```bash
curl -sS -L -OJ -H "Authorization: Bearer $SB_KEY" \
  "$BASE/v1/projects/$PROJECT/backups/$BACKUP_ID/download"
```

恢复会**覆盖当前项目数据库**，执行前应先创建一份新的安全快照并确认业务已停止写入：

```bash
curl -sS -X POST "$BASE/v1/projects/$PROJECT/backups/$BACKUP_ID/restore" \
  -H "Authorization: Bearer $SB_KEY"
```

恢复成功返回 204。备份不存在返回 404 `backup_not_found`；项目句柄暂时无法关闭返回 409 `project_busy`；引擎、格式、大小或 SHA-256 不匹配返回 409 `snapshot_mismatch`。

## 一致性与恢复过程

创建快照时持有项目写租约，因此大库快照会让写请求排队，而不是绕过一致性直接复制文件。应为备份安排低峰窗口，并根据库大小设置 `snapshot_timeout`。

恢复时服务会：

1. 校验备份状态、格式版本、引擎、文件大小和 SHA-256，并验证快照可打开。
2. 关闭项目数据库句柄，先保留当前文件，再以临时文件原子替换。
3. 在 S3 模式下为恢复后的文件发布新 manifest；任一步失败会尝试回滚到原文件。

如果进程在替换阶段中断，下次打开项目文件时会利用 `.restore-old` 自愈。恢复完成后仍应执行业务查询和关键记录校验，不能只以 204 作为全部验收。

## 运维建议

- 同时启用 `upload_to_s3` 与本地持久目录，避免单副本身份；S3 bucket 应保持私有并启用版本控制。
- 定期在隔离环境实际恢复并核对数据，备份「创建成功」不等于「恢复可用」。
- 自动备份与保留清理由同一单写实例执行，调度使用 UTC，部署多实例并不受支持。
- 系统项目的元数据与 DuckLake 目录需要独立的实例级备份方案，不能依赖项目备份 API。

部署拓扑、S3 故障和滚动升级见[部署与故障处理](/docs/ops/deployment)。
