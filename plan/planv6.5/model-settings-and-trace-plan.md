# 模型页迁移工作台 + 系统 Trace 表（Model Settings & LLM Trace Plan）

> **状态：** 第一期（§1–§7）✅ 已交付（2026-11-12，全部验证通过）；**第二期（§8 起）✅ 已交付（2026-11-12，T1–T6 全部完成：存储扩列 + ensureTraceColumns 兜底、llmgateway 内容埋点（含流式聚合）、agent/cron ctx 注入与工具 span、分组/详情 API、模型页双 tab 合并 + 分组列表 + 详情 Sheet；`go test ./internal/...` 全绿，`yarn build` 通过，`yarn test` 866/867——唯一失败为已知常驻文档断链项 `docs-content.test.ts`）**
> **日期：** 2026-11-12（一期）/ 2026-11-12（二期立项同日交付）
> **需求来源：**
> 一期：用户 2026-11-12 ——「将控制台→设置→模型单独迁移出来，放到侧边栏的工作台下；增加系统 trace 表，能看到每次模型调用的 trace 日志（参考 LangSmith 截图）」
> 二期：用户 2026-11-12 16:50 ——「① Trace 和模型应该统一在模型侧边栏打开；② 通过 tab 展示模型和 Trace 日志；③ Trace 日志没有 tools 调用、树形结构等内容 —— 点每一行要能看到输入输出、Agent 调用的 tools、系统提示词等」

---

# 第一期（已交付，存档）

**Goal：** ①「设置→模型」迁出全局弹窗，成为 `/console/models` 独立页面；② 新建 `sys_llm_traces` 表 + `/llm/traces` 查询接口，llmgateway 层埋点记录每次调用（含流式），前端 `/console/traces` 表格呈现。

**交付记录：**
- 后端：`systemdb/traces.go`（缓冲写 + RFC3339Nano 时间规避 sqlite 比较错位）、`llmgateway` Chat/Stream 埋点（`TraceRecorder` 接口）、`api/llm_traces.go`（`GET /llm/traces`）、app 装配
- 前端：`/console/models`（ModelSettings.vue 薄壳）、`/console/traces`（ModelTraces.vue 过滤+分页表格）、SettingsModal 收窄三页签、`?settings=models` deep-link 落路由
- 验证：`go test ./...` 全绿；`yarn build` 通过；`yarn test` 866/867（唯一失败 `docs-content.test.ts` 为已知常驻断链项）

一期明确的出界项（§3.4）：span 树、输入输出内容、source 区分 —— **第二期全部拾起**。

---

# 第二期：模型页 Tab 合并 + Trace 内容观测（LangSmith 式）

## 8. 需求与现状缺口

### 8.1 用户反馈 → 目标

| # | 反馈 | 目标 |
| --- | --- | --- |
| R1 | Trace 和模型统一在模型侧边栏打开 | 侧边栏删「调用追踪」独立入口，只留「模型」 |
| R2 | 通过 tab 展示模型和 Trace 日志 | `/console/models` 页内双 tab：模型设置 / 调用追踪（`?tab=` 同步，兼容 deep-link） |
| R3 | Trace 缺 tools 调用、树形结构；点行要看输入输出、Agent 的 tools、系统提示词 | trace 从「元数据行」升级为「内容观测」：span 模型 + 详情视图 |

### 8.2 现状与缺口（证据）

| # | 位置 | 现状 | 缺口 |
| --- | --- | --- | --- |
| G1 | `systemdb/traces.go` `LLMTrace` | 仅元数据：status/stream/duration/tokens/error | 无输入消息、输出内容、工具声明、工具执行结果 |
| G2 | `NavMenu.vue:79` | 工作台组 `['agents','models','traces','dashboard']` 两个入口 | 需合并为一个 |
| G3 | `llmgateway/service.go` Chat/Stream | 埋点只记元数据；但该层**拿得到全部内容**：`req.Messages`（含 system prompt、历史）、`req.Tools`、`resp.Content/ToolCalls` | 内容未记录 |
| G4 | `llmgateway/streaming.go` usageRecordingReader | 已遍历所有 chunk 聚合 usage | 未聚合 content 文本与 tool_call delta（流式输出内容拿不到） |
| G5 | `gocloudagent/runtime.go:276-380` | 运行循环已构建完整工具卡片：`{call_id, name, arguments, content, duration_ms, is_error, truncated}` → `RunResult.ToolCallsJSON` | 只落 `sys_agent_messages.tool_calls_json`，未进 trace；无 run↔trace 关联 |
| G6 | `sys_agent_runs` | 有 status/duration/tokens/tool_calls 计数 | 与 `sys_llm_traces` 无关联键（无 request_id/trace_id） |
| G7 | `api/llm_traces.go` | 扁平行列表 + total | 无分组聚合（一次 Agent run = 多行分散）、无详情端点 |
| G8 | agent 多轮 LLM 调用 | 每轮经 eino → agentcore.Model → ChatClient 适配 → `llmgateway.Chat/Stream`，ctx 由 `runtime.StartRun(runCtx)` 派生 | llmgateway 无法区分调用来源（api/agent/cron）与所属 run —— 需 ctx 元数据透传（含 eino 透传验证，见 §10.4） |

### 8.3 目标形态（用户视角）

```
侧边栏·工作台：云助手 / 模型 / 监控大盘        ← traces 入口并入「模型」
/console/models
  ├─ Tab 1：模型设置（现 ModelDefaultsPanel 原样）
  └─ Tab 2：调用追踪
        列表（每行 = 一次顶层 trace，按 trace 分组聚合）：
        | 时间 | 来源(API/云助手/定时) | 模型 | 状态 | 轮次(LLM/工具数) | 耗时 | tokens |
        点行 → 右侧 Sheet 详情：
        Trace 头：状态 / 总耗时 / 总 tokens / request_id（复制）
        Span 时间线（树形缩进，按时间排序）：
        ▸ ● LLM  deepseek-ai/DeepSeek-V4.1-Flash   1.2s  in 800 → out 50
        │    展开：输入消息列表（system 系统提示词高亮折叠 / user / 历史）
        │          输出（正文 或 tool_calls 请求）
        │          本次声明的工具列表（name + description）
        ▸ ◆ Tool sql_query                          300ms  ✓
        │    展开：输入参数 JSON / 输出结果（截断标记）/ 耗时 / 是否出错
        ▸ ● LLM  deepseek-ai/...（第二轮，输入含工具结果）
        ▸ ◆ Tool ...
        ▸ ● LLM  最终回答
```

## 9. 数据模型：trace_id + span

### 9.1 `sys_llm_traces` 扩列（改造既有表）

```sql
-- 一期列保持不变，追加：
trace_id    VARCHAR NOT NULL DEFAULT '',   -- 顶层 trace：agent 场景 = run_id；API 直调 = request_id；空则回填自身 id
caller      VARCHAR NOT NULL DEFAULT '',   -- 'api' | 'agent' | 'cron'（展示徽章；一期 source 列废弃，迁移期读 caller 回退 source）
span_kind   VARCHAR NOT NULL DEFAULT 'llm',-- 'llm' | 'tool'
name        VARCHAR NOT NULL DEFAULT '',   -- llm: 模型名；tool: 工具名
input_json  VARCHAR NOT NULL DEFAULT '',   -- llm: 请求消息数组快照；tool: arguments JSON
output_json VARCHAR NOT NULL DEFAULT '',   -- llm: {content, tool_calls[]}；tool: 结果文本
tools_json  VARCHAR NOT NULL DEFAULT '',   -- 仅 llm span：本次声明的工具 [{name, description}]（不含 parameters 全文）
```

- **迁移**：该表为本轮开发新增、未随任何版本发布。已存在的 dev 库直接 `DROP TABLE sys_llm_traces`（或 `./build.sh reset -y`）后由 `EnsureSystemSchema` 重建。同时在 `schema.go` 旁加轻量升级兜底 `ensureTraceColumns`（PRAGMA/duckdb_columns 检查缺列则 `ALTER TABLE ADD COLUMN`），防止他人 dev 库漏 reset。
- **写入仍走既有缓冲**（traceBuf / async flusher），工具 span 复用同一通道。
- **截断常量**（`traces.go`）：单条消息 4000 字符；单 span input 总量 64KB / output 32KB；工具结果 8KB（与 runtime 工具卡片 4000 字符口径对齐，取小者）；超限置 `truncated` 标记进 JSON（`{"truncated":true,...}` 包装或字段）。
- **隐私**：单机自托管，数据本属用户项目，第一期默认记录、不做脱敏管线；`api_key` 等凭据天然不在消息/参数里。内容开关留作后续（§13 出界）。

### 9.2 span 语义

| 场景 | trace_id | span 序列 | 写入点 |
| --- | --- | --- | --- |
| API 直调（非流式/流式） | request_id | 1 × llm | llmgateway.Chat/Stream（既有埋点扩内容） |
| 云助手交互 run | run_id | N × llm + M × tool 交替 | llm span 同上；tool span 在 run 收尾由 `RunResult.ToolCallsJSON` 批量补写 |
| 定时 agent run | run_id | 同上 | 同上（cron 收尾路径） |

- 工具卡片已含 `arguments + content + duration_ms + is_error`，一张卡片 = 一条完整 tool span（`status: is_error ? "error" : "ok"`）。
- 树形结构第一期用**时间线呈现**（span 按 `created_at` 排序线性排列，LLM/Tool 用图标区分）；不引入 parent_id 嵌套 —— Agent 循环天然是交替序列，线性即可还原；LangSmith 式真正嵌套树留后续（加 `parent_span_id` 列即可演进，本期预留不做）。

### 9.3 查询

```go
// traces.go 新增
QueryLLMTraceGroups(ctx, TraceGroupQuery) ([]LLMTraceGroup, error)   // 列表：GROUP BY trace_id
QueryTraceSpans(ctx, projectID, traceID string) ([]LLMTrace, error)  // 详情：该 trace 全部 span 按时间
RecordAgentToolSpans(projectID, traceID, toolCardsJSON string)       // 解析卡片批量入队
```

分组聚合 SQL（列名即语义）：

```sql
SELECT trace_id,
       MIN(created_at)          AS started_at,
       MAX(created_at)          AS finished_at,
       MAX(caller)              AS caller,
       MAX(name)                AS model,        -- llm span 的 name；同 trace 模型通常一致
       MAX(provider)            AS provider,
       MAX(CASE WHEN status='error' THEN 1 ELSE 0 END) AS has_error,
       COUNT(*)                 AS span_count,
       SUM(CASE WHEN span_kind='tool' THEN 1 ELSE 0 END) AS tool_count,
       SUM(input_tokens)        AS input_tokens,
       SUM(output_tokens)       AS output_tokens,
       MAX(error_code)          AS error_code,
       MAX(stream)              AS stream        -- 任一 span 流式即视为流式（0/1）
FROM sys_llm_traces
WHERE project_id = ? [AND caller = ?] [AND status = ?] [AND name LIKE ?] [AND created_at >= ? ...]
GROUP BY trace_id
ORDER BY MAX(created_at) DESC
LIMIT ? OFFSET ?
-- COUNT 子查询同条件 COUNT(DISTINCT trace_id) 作 total
```

- 总耗时 = `finished_at - started_at`（跨 span 墙钟差；单 span 时与 duration_ms 一致），由前端计算，不落库。
- 既有 `QueryLLMTraces`/`CountLLMTraces`（扁平行）保留给兼容测试，业务面切到分组查询。

## 10. 埋点设计

### 10.1 llmgateway：LLM span 内容化

```go
// service.go — LLMTrace 扩展（一期字段不动）
type LLMTrace struct {
    // ...一期字段...
    TraceID   string
    Caller    string
    SpanKind  string        // "llm"
    Name      string        // 解析后的模型名
    InputJSON string        // 序列化后的消息快照（截断后）
    OutputJSON string
    ToolsJSON string
}

// 新增 ctx 元数据（llmgateway 定义，避免 api 反向依赖反转）
func WithTraceMeta(ctx context.Context, traceID, caller string) context.Context
func TraceMetaFrom(ctx context.Context) (traceID, caller string)
```

- **Chat**：成功路径把 `lreq.Messages`（含 system 提示词与历史）、`resp.Content`、`resp.ToolCalls`、`lreq.Tools`（仅 name+description）序列化截断后随既有 trace 一起记；错误路径 input 照记、output 记 `{error: ...}`。
- **Stream**：`usageRecordingReader` 扩展聚合 —— `chunk.Type=="content"` 累积正文、`"tool_call_delta"` 按 Index 聚合 id/name/args 分片（复用 `oaiStreamWriter` 的聚合思路，但注意这里聚合的是**网关形状** chunk）；`maybeRecord()` 时一并写入。
- **流式中途客户端断开**：已消费部分照记（一期语义不变）。

### 10.2 trace_id / caller 的解析规则

`api/llm_trace_recorder.go`（既有 recorder 扩展）：

```
traceID, caller := llmgateway.TraceMetaFrom(ctx)
if traceID == "" { traceID = RequestIDFromContext(ctx) }   // API 直调
if caller == "" { caller = "api" }                          // 未标记默认 api
```

### 10.3 Agent / Cron 注入

- `api/agent_handler.go`：`CreateRun`（同步与 streamRun 两条路径）在调 `runtime.StartRun` 前 `ctx = llmgateway.WithTraceMeta(ctx, run.ID, "agent")`。
- `internal/agentbridge/cron_agent.go`：同构注入 `caller="cron"`。
- **工具 span**：`finishAgentRun`（agent_handler）与 cron 收尾处，把 `res.ToolCallsJSON` 传 `RecordAgentToolSpans(projectID, run.ID, toolJSON)`；解析失败仅 Warn 不影响主流程。工具 span 的 `created_at` 用 run 收尾时刻统一落（卡片内已有 duration_ms，前端时间线用 `span 顺序 + duration` 呈现，不强求工具真实起始时刻 —— 第一期口径，注明）。

### 10.4 ctx 透传验证（关键前置）

链路：`agent_handler(ctx) → Runtime.StartRun → eino adk.Runner.Run(runCtx) → compose → agentcore.Model.Chat(ctx) → ChatClient → llmServiceAdapter → llmgateway.Chat(ctx)`。
Go 惯例 `WithTimeout/WithValue` 均保留父 ctx 值，eino compose 预期透传。**T2 第一步写穿透验证测试**（fake ChatClient 断言收到的 ctx 带 meta）。

**退化方案**（若 eino 内部换 ctx）：`sys_agent_runs` 加 `request_id` 列，agent_handler 落 run 时写入 HTTP request_id，前端用 request_id 关联 llm trace 与 run；caller 标记则退化为前端按「trace_id 是否命中 run_id 前缀」不可行 —— 改为 detail 接口 join runs 表补 caller。工作量可控，不阻塞。

## 11. API 面

```
GET /v1/projects/:id/llm/traces              → 列表（改造为分组聚合）
    ?caller=api|agent|cron&status=&model=&from=&to=&limit=&offset=
    → { traces: [LLMTraceGroup], total }
GET /v1/projects/:id/llm/traces/:traceID     → 详情（新）
    → { trace_id, caller, model, provider, status, stream, started_at, finished_at,
        input_tokens, output_tokens, error_code, spans: [LLMTrace（含内容字段）] }
```

- 权限对齐一期：`delegable(auth.DatabaseRead)`。
- 详情端点校验 trace 归属 project；traceID 为空或不存在返回 404。
- 既有扁平响应字段（`traces[].request_id` 等）在分组形状中保留为聚合首值，前端 `types.ts` 同步。

## 12. 前端设计

### 12.1 页面合并（R1+R2）

| 项 | 变更 |
| --- | --- |
| `router/index.ts` | `models` 路由不变；新增 `traces` 路由改 **redirect** → `{ name:'models', query:{ tab:'traces' } }`（保旧链/测试兼容）；首页 `?settings=models` 逻辑不变 |
| `pages/ModelSettings.vue` | 改造为双 tab 壳：`SbTabsPanel :items="[{value:'settings'},{value:'traces'}]"` + `?tab=` query 双向同步（默认 settings）；tab 文案 `pages.models.tabs.*` |
| `components/settings/ModelDefaultsPanel.vue` | 不动（原样进 settings tab） |
| `components/NavMenu.vue` | 工作台组 `['agents','models','dashboard']`；删 traces 菜单项 |
| `nav-icons.ts` | 删 `traces: ActivityIcon`（确认无他用） |
| i18n | 删 `nav.traces`；加 `pages.models.tabs.settings/traces`；`pages.traces.*` 扩展 key |

### 12.2 Trace 列表 + 详情（R3）

- **拆分**：`pages/ModelTraces.vue` → `components/traces/TraceList.vue`（列表）+ `components/traces/TraceDetail.vue`（Sheet 详情）；原页面文件删除（redirect 已兜底）。
- **TraceList**：
  - 过滤栏：来源 Select（全部/API/云助手/定时）、状态、模型关键词、刷新（沿用既有控件与三态）
  - 列：时间（首 span）｜来源 Badge（api=中性 / agent=蓝 / cron=紫灰）｜模型｜状态（聚合，任一 span error 即红）｜轮次 `LLM×n · 工具×m`｜耗时（finished-started）｜tokens 合计
  - 行点击 → 打开 TraceDetail（`GET /llm/traces/:traceID`），选中态高亮
  - `TablePager` 服务端分页沿用
- **TraceDetail**（`ui/sheet` 右侧滑出，宽 `sm:max-w-2xl`）：
  - 头部：caller 徽章、模型、状态、总耗时、总 tokens、request_id/run_id 复制按钮
  - **Span 时间线**：垂直时间线（左侧圆点连线），行 = `kind 图标（●LLM / ◆Tool）+ name + duration + tokens（llm）+ 状态点`；按 `created_at` 排序
  - **Span 展开**（点击行内切换，默认折叠）：
    - LLM：输入消息逐条渲染（`role` 标签，system 系统提示词默认折叠 + 「展开全文」，user/assistant/tool 直接文本）；输出区（正文或 tool_calls 列表：`name + arguments JSON`）；声明工具列表（name + description）；truncated 标记
    - Tool：参数 JSON（`<pre>` 滚动）、结果文本（截断标记）、耗时、is_error 红标
  - 内容均为只读展示，JSON 用等宽字体（`sb-mono`），长文本区域 `max-h` + 滚动
- **类型与 client**：`types.ts` 加 `TraceGroup` / `TraceSpan`；`http-api.ts` `llmTraces.list`（分组响应）+ `llmTraces.detail(projectId, traceId)`；`toTraceGroup`/`toTraceSpan` 映射对齐既有 `toLLMTrace` 风格。

## 13. 明确出界（第二期不做）

- 真·嵌套树（parent_span_id 层级）：列结构已可平滑演进，本期时间线足够还原 Agent 循环
- 工具 span 的真实起始时刻：统一用 run 收尾时刻 + duration 呈现
- 内容脱敏管线 / 记录开关（`sys_llm_settings.trace_content`）：默认全记
- trace TTL / retention：沿用 24h 默认查询窗口，清理机制挂后续
- 首 token 延迟（TTFT）：流式 duration 仍为总时长

## 14. 文件清单（预期 diff）

### 后端
| 文件 | 动作 |
| --- | --- |
| `internal/systemdb/schema.go` | `sys_llm_traces` DDL 加 7 列（trace_id/caller/span_kind/name/input_json/output_json/tools_json）；`ensureTraceColumns` 升级兜底 |
| `internal/systemdb/traces.go` | `LLMTrace` 扩字段；截断函数；`RecordAgentToolSpans`；`QueryLLMTraceGroups`；`QueryTraceSpans`；分组/计数 SQL |
| `internal/systemdb/traces_test.go` | 新列往返、分组聚合（多 span/多 trace）、trace_id 详情、工具卡片解析入队、截断 |
| `internal/llmgateway/service.go` | `LLMTrace` 扩内容字段；`WithTraceMeta/TraceMetaFrom`；Chat 埋点序列化输入输出工具 |
| `internal/llmgateway/streaming.go` | reader 聚合 content/tool_call_delta → OutputJSON |
| `internal/llmgateway/trace_test.go` | 内容断言（fake tracer 收集 input/output/tools）、流式聚合、ctx meta 透传穿透验证（fake ChatClient） |
| `internal/api/llm_trace_recorder.go` | 解 TraceMeta → trace_id/caller 回退规则 |
| `internal/api/agent_handler.go` | CreateRun/streamRun ctx 注入；finishAgentRun 调 `RecordAgentToolSpans` |
| `internal/agentbridge/cron_agent.go` | ctx 注入 caller=cron；收尾写工具 span |
| `internal/api/llm_traces.go` | List 改分组；+ `Get` 详情 handler |
| `internal/api/router.go` | + `p.GET("/llm/traces/:traceID", th.Get, delegable(auth.DatabaseRead))`（注意挂在 `/llm/traces` 之前不冲突：echo 静态段优先） |
| `internal/api/llm_traces_test.go` | 分组响应、详情 404/越权、caller 过滤 |

### 前端
| 文件 | 动作 |
| --- | --- |
| `ui/src/router/index.ts` | traces 路由改 redirect `?tab=traces` |
| `ui/src/pages/ModelSettings.vue` | 双 tab 壳 + query 同步 |
| `ui/src/components/traces/TraceList.vue` | 新建（自 ModelTraces.vue 重构） |
| `ui/src/components/traces/TraceDetail.vue` | 新建（Sheet + span 时间线 + 内容展开） |
| `ui/src/pages/ModelTraces.vue` | 删除 |
| `ui/src/components/NavMenu.vue` | 删 traces 入口 |
| `ui/src/components/nav-icons.ts` | 删 traces 图标 |
| `ui/src/services/types.ts` | + `TraceGroup`/`TraceSpan`；`LLMTrace` 扩内容字段 |
| `ui/src/services/http-api.ts` | `llmTraces.list` 改分组；+ `detail` |
| `ui/src/locales/{zh-CN,en-US}.ts` | tabs/来源/时间线/展开文案，两树同构 |
| `ui/tests/router.test.ts` | `/console/traces` → `/console/models?tab=traces` 断言 |
| `ui/tests/ModelSettings.test.ts` | tab 切换与 query 同步断言 |
| `ui/tests/ModelTraces.test.ts` → `TraceList.test.ts` | 改造：分组渲染、来源过滤、行点击打开详情（mock detail） |
| `ui/tests/TraceDetail.test.ts` | 新建：span 时间线渲染、system 折叠、tool 参数/结果展示 |
| `ui/AGENTS.md` | 页签约定 / 路由 redirect 清单 / components 清单同步 |

## 15. 实施顺序（每步可独立验证）

1. **T1 后端-存储**：DDL 扩列 + ensureTraceColumns + traces.go 查询扩展 + 单测 → `go test ./internal/systemdb/...`；dev 库 DROP/reset 验证重建
2. **T2 后端-埋点**：ctx 透传穿透验证测试先行（§10.4）→ llmgateway 内容埋点（Chat/Stream）→ fake tracer 断言 → `go test ./internal/llmgateway/...`
3. **T3 后端-API**：recorder 回退规则 + agent/cron ctx 注入 + 工具 span 写入 + 分组/详情 API + 单测 → `go test ./internal/api/... ./internal/agentbridge/...`；`go build ./...`
4. **T4 前端-合并**：路由 redirect + ModelSettings tab 壳 + NavMenu/图标/i18n + 既有测试更新 → `yarn build` + `yarn test`
5. **T5 前端-详情**：types/client + TraceList 重构 + TraceDetail + 新测试 → `yarn build` + `yarn test`
6. **T6 全量验证**：`./build.sh dev` 手测三类调用（API 直调 / 云助手对话触发工具 / 定时任务），检查 trace 分组、span 时间线、系统提示词与工具内容可见；`ui/AGENTS.md` 同步

## 16. 验收标准

- [x] 侧边栏工作台只有「模型」一个相关入口；页内「模型设置 / 调用追踪」双 tab 可切换且 `?tab=` 可直达；`/console/traces` 旧地址自动跳转到 traces tab
- [ ] API 直调一次：列表新增一行（来源=API），点开可见输入消息（含系统提示词）、输出正文、声明工具；流式调用输出聚合完整 —— *代码路径已由单测覆盖（llmgateway trace_test），待 `./build.sh dev` 手测确认*
- [ ] 云助手发起一次带工具调用的对话：列表出现一行（来源=云助手），详情时间线呈 LLM→Tool→…→LLM 交替；每个 LLM span 可看该轮输入（系统提示词/历史/工具结果）与输出；每个 Tool span 可看参数与结果；错误工具红标 —— *代码路径已由单测覆盖（systemdb/agentbridge/api），待手测确认*
- [ ] 定时 agent 跑一轮：同上，来源=定时 —— *代码路径已由单测覆盖，待手测确认*
- [ ] 错误调用（坏 Key）：详情可见错误 span 与错误信息 —— *llm 错误路径单测已覆盖，待手测确认*
- [ ] `go test ./...` 全绿；`yarn build` 通过；`yarn test` 除常败项 `docs-content.test.ts` 外全绿；i18n key 同构
- [ ] 覆盖率不低于既有阈值（≥95%）

## 17. 风险与备注

| 风险 | 应对 |
| --- | --- |
| eino compose 不透传 ctx 值 | T2 第一步穿透测试；退化方案：`sys_agent_runs` 加 request_id 列 join 关联（§10.4），不阻塞 |
| input_json 含全量历史，Agent 多轮重复存储 | 截断常量硬上限（64KB/span）；单机盘存储可接受；TTL 后续 |
| dev 库旧表结构不匹配 | 表未发布：优先 reset；`ensureTraceColumns` ALTER 兜底 |
| `/llm/traces/:traceID` 与列表路由冲突 | echo 静态/参数段分层注册验证；命名不重叠（列表无子路径） |
| 工具 span created_at 用收尾时刻，时间线顺序失真 | 前端按 span 顺序 + duration 呈现，不依赖绝对时刻；口径已注明 |
| 内容记录的隐私顾虑 | 单机自托管默认记录（数据本属用户）；开关列留后续 |
