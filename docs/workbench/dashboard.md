---
title: 监控大盘
order: 10
group: 监控大盘
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

## 怎么读这些指标

| 指标 | 含义 | 注意 |
|---|---|---|
| 请求 | 近 24 小时内项目接口的请求总数 | 来自 `metrics/summary`；趋势图另按天聚合，默认 7 天，可用 `days` 调整 |
| 错误率 | 近 24 小时错误请求数 ÷ 请求总数，页面以百分比展示，保留 4 位小数 | `metrics/summary` 返回的 `error_rate` 是 0~1 的比值，不是百分数，例如 `0.015` 在页面显示为 `1.5000%`；请求总数为 0 时为 `0` |
| 延迟 | 近 24 小时平均延迟，以及 P50 / P90 / P99 | 分位数按固定桶估算，超出量程会显示为 `≥30s` |
| 趋势图 | 请求与错误的按天变化 | 默认折线图，可切换为柱状图；两者数据相同 |

「暂无近 24 小时接口样本」表示该时间窗内没有可统计延迟分布的请求，并不代表服务异常。

## 使用场景

### 场景一：错误率突然升高

1. 在大盘看趋势图，确认是单日尖峰还是持续上升。
2. 打开[日志管理](/docs/ops/logs)，把级别设为 `error`，时间范围对齐尖峰日期，查看高频消息。
3. 若来自某个云函数，查看该函数的调用耗时与错误记录（指标 `gofunction_invoke_errors`），回到[云函数](/docs/automation/gofunction)修复后重新保存。

### 场景二：用脚本做简易告警

下面的脚本读取摘要，错误率超过 5% 时输出告警。注意 `error_rate` 是比值，所以阈值写 `0.05`：

```bash
RATE=$(curl -sS "$BASE/v1/projects/$PROJECT/metrics/summary" \
  -H "Authorization: Bearer $SB_KEY" | jq -r '.error_rate')
awk -v r="$RATE" 'BEGIN { if (r > 0.05) { print "ALERT: error_rate=" r; exit 1 } }'
```

可放进定时任务或外部调度器。`database:read` 权限的 Key 足够，不需要写权限。

### 场景三：核对容量是否健康

大盘首行卡片展示数据库总数、就绪数和异常数。出现「异常数据库」时，先看数据库状态是否为 `degraded`，再按[部署指南](/docs/ops/deployment)排查存储与单写实例问题。

## 常见问题

| 现象 | 可能原因 | 处理 |
|---|---|---|
| 趋势图为空 | 项目刚创建，或趋势接口请求失败 | 点击「刷新数据」；失败时页面会提示「趋势加载失败」 |
| 摘要行没有错误率 | 近 24 小时请求总数为 0 | 产生请求后再刷新 |
| 数字与日志条数不一致 | 指标是聚合与采样值 | 以日志与审计为准，参见[日志管理](/docs/ops/logs) |

## 运维只读接口

以下接口控制台不直接展示或只用于状态卡，供运维脚本与外部系统查询，均要求当前项目的 `database:read` 权限：

| 方法 | 路径 | 响应 | 说明 |
|---|---|---|---|
| GET | `/v1/projects/:projectID/quota` | `{ "llm_allowed": bool, "database_allowed": bool }` | 配额是否可用；服务端按项目缓存 10 秒 |
| GET | `/v1/projects/:projectID/llm/providers` | `{ "providers": […] }` | 当前项目可用的 LLM 供应商名称；未启用 LLM 时不挂载 |

项目级与全局键值设置接口 `GET/PUT /v1/projects/:projectID/settings`、`GET/PUT /v1/settings`（`{"key","value_json"}`）为**保留接口**：控制台不使用，后续版本可能收紧或移除，新集成请改用对应业务接口（如 `/llm/settings`）。
