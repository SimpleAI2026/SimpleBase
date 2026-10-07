---
title: 监控大盘
order: 3
---

# 监控大盘

控制台「监控大盘」展示所选项目的运行概况，包括请求、错误、延迟等指标；它不是明细日志列表。需要逐条排错请进入[日志与审计](/docs/ops/logs)。

## 查询项目指标

| 方法 | 路径 | 用途 |
|---|---|---|
| GET | `/v1/projects/:projectID/metrics/summary` | 项目概览指标 |
| GET | `/v1/projects/:projectID/metrics/trend?days=7` | 近 7 天趋势，响应 `{ "points": […] }` |

接口要求当前项目的 `database:read` 权限：

```bash
curl -sS "$BASE/v1/projects/$PROJECT/metrics/summary" \
  -H "Authorization: Bearer $SB_KEY"
```

设置项目和 Key 的方法见[5 分钟上手](/docs/getting-started/quickstart)。指标可能存在聚合、缓存和采样延迟，不用于替代审计记录或远端存储一致性检查；容量和异常处理参见[部署指南](/docs/ops/deployment)。
