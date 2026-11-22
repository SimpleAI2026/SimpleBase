# 云助手协作群 v2 实施说明（交付与运维）

对应计划：[agent-console-buzz-inspired-v2-plan.md](./agent-console-buzz-inspired-v2-plan.md)

本文只记录**已落地的事实**：新增表、接口、事件、开关、建表步骤与回退方式。
实现细节以代码为准，此处给出定位与运维口径。

---

## 1. 交付范围

| 能力 | 状态 |
| --- | --- |
| 群 = 协作会话（`dispatch_mode=collaborative`） | 已落地 |
| 一轮多助手按依赖串行执行（全部执行，不替换、不遗漏） | 已落地 |
| 群级路由提示词 / 兜底助手 / 人工策略 | 已落地 |
| `@当前人` 人工待办（回复并继续 / 无法处理） | 已落地 |
| 所有群可展开多会话 + 群行 `+`（加已有 / 新建并加入 / 新建会话） | 已落地 |
| 群定时任务（`reuse_thread` / `new_thread_per_fire` + 触发去重） | 已落地 |
| 告警 / 统计输入入口（`POST /agent-tags/:tagID/events`） | 已落地 |
| 旧 `auto` 会话读时统一为 `collaborative` | 已落地 |
| 固定助手老会话（`agent_id` 非空、无 workspace 行） | 保持原 `/runs` 语义 |

---

## 2. 新增存储（全部为 `schema.go` 最终态建表）

`internal/systemdb/schema.go` 的 `systemTables` 追加 8 张表，**没有 ALTER、没有版本化迁移**：

| 表 | 用途 |
| --- | --- |
| `sys_agent_thread_collab` | 会话级协作元数据：`thread_kind`、`active_turn_id`（会话占用 CAS）、`version` |
| `sys_agent_turns` | 轮次：发起者、幂等键、状态、路由卡 JSON、群配置快照 |
| `sys_agent_tasks` | 任务：`target_kind`(agent/human)、目标、指令、依赖、状态、`run_id`、接收人、回复 |
| `sys_agent_task_runs` | 任务重试历史（每次 attempt 一行，不覆盖旧 run） |
| `sys_agent_message_meta` | 消息旁挂身份（speaker_kind/speaker_id/kind），不改既有消息表 |
| `sys_agent_turn_events` | 轮次事件（单调 `seq`），刷新与续传的唯一持久来源 |
| `sys_agent_group_cron` | 群定时绑定：`thread_policy`、`thread_kind`、`thread_id`、`owner_user_id` |
| `sys_agent_cron_fires` | 触发去重（`fire_key`） |

逻辑唯一性由 Store 内的项目级互斥 + 先查后写收口（DuckLake 无 PK/UNIQUE）。

### 建表步骤（运维必读）

- 正常启动会走 `EnsureSystemSchema`，自动建表；
- **`TIMER_NO_RUN=1` 会跳过初始化**：此时协作接口返回 `503 workspace_upgrade_required`，
  `GET /agent-workspace-capabilities` 会列出 `missing_tables`；
- 需要手动升级时，按 `schema.go` 里这 8 张表的 DDL 依次执行即可（幂等）。

前端据此降级：`capabilities.collaboration === false` 时不露出协作控件。

---

## 3. 新增/变更接口

基础路径 `/v1/projects/:projectID`。

| 接口 | 权限 | 说明 |
| --- | --- | --- |
| `GET /agent-workspace-capabilities` | read | 协议版本、限制、`missing_tables` |
| `POST /agent-tags/:tagID/members` | write + writeGate | `{agent_ids}` 或 `{new_agent}`（互斥），返回更新后的群 |
| `DELETE /agent-tags/:tagID/members/:agentID` | write + writeGate | 只移出成员，不删除助手 |
| `POST /agent-tags/:tagID/events` | read + writeGate | 告警/统计输入，按 `external_id` 幂等 |
| `POST /agent-threads/:threadID/turns` | read + writeGate | 发起一轮，`mentions` 支持 `{agent_id}` 与 `{kind,id}` |
| `GET /agent-threads/:threadID/turns` | read | 轮次列表 |
| `GET /agent-turns/:turnID` | read | 轮次快照（含任务与 `last_event_seq`） |
| `GET /agent-turns/:turnID/events?after=` | read | 事件回放（终态只回 `end`） |
| `POST /agent-turns/:turnID/actions` | write + writeGate | `cancel` / `retry_task` / `resume` |
| `POST /agent-tasks/:taskID/replies` | read + writeGate | 人工回复（`continue` / `unable`） |
| `GET /agent-human-tasks?assignee=me&status=pending` | read | 待办列表 + 角标 |

### 变更

- `POST /agent-threads`：新增 `thread_kind`；允许 `dispatch_mode=collaborative`；
  `tag_id` 不再强制要求 `auto`；群会话仍禁止 `agent_id`。
- `POST /agent-threads/:threadID/runs`：/runs 只保留**单目标**兼容；多目标仍返回
  `400 multiple_agents_not_supported`（提示改用 `/turns`），并新增活动轮次守卫（`409 turn_busy`）。
- `DELETE /agent-tags/:tagID`：归档**保留**会话的 `tag_id`（历史不搬回 #对话），
  同时归档该群的定时绑定。
- `POST/PATCH /agent-tags`：新增 `routing_prompt`（≤4000）、`fallback_agent_id`（必须是成员）、
  `human_policy`（空或 `disabled`）。

### 新增错误码

`workspace_upgrade_required`(503)、`group_archived`(409)、`group_member_unavailable`(400)、
`turn_busy`(409)、`turn_conflict`(409)、`turn_not_found`(404)、`task_not_found`(404)、
`routing_failed`(502)、`invalid_dispatch_graph`(400)、`human_assignee_required`(409)、
`human_task_forbidden`(403)、`task_not_retryable`(409)、`collaboration_budget_exceeded`(429)。

---

## 4. 执行语义

- **路由**：显式 `@` 优先（逐个校验成员资格）；否则调用路由器（群路由提示词 + 成员能力 + 最近 6 条历史）。
  路由失败依次尝试：群兜底助手 → 请当前人澄清 → 以 `routing_failed` 结束。**不再静默取第一个成员**。
- **串行**：按依赖拓扑推进；上游失败把下游置 `blocked`，不会留下「永远等不到」的任务。
- **互斥**：会话级 `active_turn_id` CAS；`/turns`、`/runs`、Plan、cron 都先检查。
- **等人**：轮次转 `waiting_human` 并**释放**会话占用（不占模型、不阻塞下一轮）；回复后重新抢占并续跑。
- **身份**：`@人` 只能是登录态（`Principal.UserID`）；API Key 主体指派返回 `human_assignee_required`。
  待办只能由接收人本人或项目管理员处理（`human_task_forbidden`）。
- **与工具审批分离**：人工回复只写任务与消息，**不放行任何工具**；工具审批仍走
  `POST /agent-runs/:runID/confirmations/:callID`。
- **重启收敛**：进程启动时把残留 `routing`/`running` 轮次收敛为 `interrupted`（不自动重跑），
  用户可在界面 `resume` 或按任务重试。

### 定时

- 绑定在 cron 任务上（`target_kind=agent` + `tag_id` + `thread_policy`）；
- `reuse_thread`：复用会话，缺失/已归档时新建并回写；`new_thread_per_fire`：每次新建；
- 去重键 = `任务 + 分钟窗口`，重复投递返回 `skipped: duplicate fire`；
- 执行主体是只读 Headless（`apikey:system-scheduler`）：**不产生人工任务**；
  绑定了 `owner_user_id` 时，人能作为候选；需要人而无负责人 → `human_assignee_required`；
- 群归档即暂停调度（绑定一并归档）。

---

## 5. SSE 事件（轮次流）

| 事件 | 载荷 | 说明 |
| --- | --- | --- |
| `route` | `status=routing/done`、`steps[]`（`kind/agent_id/instruction`） | 路由卡 |
| `task` | `task_id`、`index`、`total`、`status`、`message` | 任务状态变化 |
| `human` | `human_id`、`index`、`message` | 轮次停下等人 |
| `turn` | `status`、`index`、`total` | 轮次状态/进度 |
| 子 run 帧 | `run_id` + `agent_id` | token/工具/审批；前端按 `run_id` 归到任务 |
| `end` | `reason` | 轮次结束 |

前端 `onFrameContext` 用 `run_id`/`agent_id` 定位任务；审批按 `runId:callId` 索引，多助手不互相覆盖。

---

## 6. 前端落点

| 文件 | 变更 |
| --- | --- |
| `services/types.ts` | `AgentTurn`/`AgentTask`/`AgentHumanTask`/能力/绑定/`MentionSendTarget` 等接口 |
| `services/http-api-converters.ts` | `auto→collaborative`、群新字段、task/human/turn 帧、`streamAgentTurn` |
| `services/http-api.ts` | `agentTurns.*`、`agentTags.addMembers/removeMember/submitEvent`、`agentThreads.streamTurn/turns` |
| `composables/useAgentWorkspace.ts` | 每群独立分页、展开懒加载、旧响应守卫 |
| `composables/useAgentCollaboration.ts` | 轮次/任务/子 run/审批四层索引，取消、重试、人工回复 |
| `components/agent/AgentWorkspaceSidebar.vue` | 所有群可展开、群行 `+` 菜单、待办角标、类型图标 |
| `components/agent/AgentDispatchCard.vue` | 路由分工卡（步骤 + 状态 + 重试） |
| `components/agent/AgentHumanTaskCard.vue` | 人工待办卡（回复并继续 / 无法处理） |
| `components/modal/AgentGroupMembersModal.vue` | 加入已有 / 新建并加入（两页签） |
| `components/modal/AgentTagModal.vue` | 路由提示词、兜底助手、人工策略 |
| `pages/AgentManager.vue` | 协作轮次接线、多目标发送、成员弹窗、待办角标与轮询 |

`#对话` 仍按 `tag_id='chat'` 分区；固定助手会话继续走 `/runs`（多目标明确拒绝）。

---

## 7. 回退说明

1. **前端回退**：旧前端仍可工作 —— `/runs` 单助手路径保留；`GET /agent-tags`、`GET /agent-threads`
   在旧客户端眼里字段是超集，不影响解析。
2. **停止协作写**：把 `dispatch_mode` 读时映射改回原样即可（`http-api-converters.ts` 的 `isDispatchMode`），
   协作接口仍存在但界面不再调用。
3. **数据回退**：协作表是增量表，不参与旧逻辑读取；直接停止写入即可，无需删表。
   若要彻底移除，先确认无活动轮次，再按 `sys_agent_cron_fires` → `sys_agent_turn_events` →
   `sys_agent_message_meta` → `sys_agent_task_runs` → `sys_agent_tasks` → `sys_agent_turns` →
   `sys_agent_group_cron` → `sys_agent_thread_collab` 顺序删除。
4. **群归档语义变化**：本版归档**保留**会话 `tag_id`（历史留在群里）。若需旧行为（搬回 #对话），
   归档前手动清空这些会话的 `tag_id`。

---

## 8. 验证命令

```bash
# 后端
go build ./internal/... ./cmd/...
go test ./internal/...
go test -count=1 ./internal/api -run 'TestCollab|TestGroupCron|TestGroupMembers|TestGroupEvent|TestTurnEvents|TestWorkspace'
(cd gocloudagent && GOWORK=off go build ./... && GOWORK=off go test ./...)

# 前端
cd ui && npx vue-tsc --noEmit
cd ui && CI=1 npx vitest run
cd ui && npx vite build
```

对应测试文件：

- `internal/api/collab_e2e_test.go`：多助手真的都执行、人工闭环、幂等、占用互斥、归档拒绝、成员增删、事件入口、恢复/取消
- `internal/api/collab_cron_test.go`：两种定时策略、触发去重、归档暂停、绑定校验
- `ui/tests/useAgentWorkspace.test.ts`、`ui/tests/useAgentCollaboration.test.ts`
- `ui/tests/agent-collab-ui.test.ts`、`ui/tests/AgentManager.test.ts`

---

## 9. 写权限口径（v2 §3.4）

- **助手不再有只读/可写配置**：`sys_cloud_agents.writable` 列保留在库中（不做迁移），
  但代码不再读写；DTO / 请求体 / 前端类型 / 助手卡片徽标均不再有该字段。
- `agentbridge.Agent()` 映射出的运行时助手**恒为可写**，提示词固定声明 writable 能力。
- 破坏性命令仍由**人工确认**拦截（`confirmation_required` → `POST /agent-runs/:runID/confirmations/:callID`），
  与写权限配置无关。
- 仍保留的两处「降权」属于运行策略，不是助手配置：
  - BI 运行（`applyBIRunPolicy`）强制只读；
  - 定时任务主体是只读 Headless（`apikey:system-scheduler`）；
  - 系统库写入拒绝（`IsSystemDatabase`）。

## 10. 已知限制

- 首版多任务是**串行**执行（按依赖顺序），不是并行首字；
- 触发去重按分钟窗口，不承诺跨实例 exactly-once；
- `getAgentTagRow` 未做归档行过滤，归档群的读接口仍返回历史配置（写接口已拒绝）；
- 计划模式与协作轮次互斥：群会话里开启 Plan 会被前端拦截并提示新建会话。
