// 文件说明：http-api/agent.ts — `httpApi` 的云助手（助手、会话、标签、轮次、审批、MCP）部分；只在这里出现对应的 URL 与 wire → UI 转换。
// 与 http-api.ts、其余域文件共同组成 `Api` 的唯一实现，不要在页面里直接 import 本文件。
import { http } from '@/services/http'
import type { AgentModuleInfo, AgentRun, ActiveAgentRun, Api } from '@/services/types'
import {
  toAgentApproval,
  toAgentHumanTask,
  toAgentTag,
  toAgentTurn,
  toAgentWorkspaceCapabilities,
  fromAgentTagInput,
  toAgentMessage,
  toAgentThread,
  toCloudAgent,
  toMcpBuiltinServer,
  toMcpServer,
  toMcpTestResult
} from '@/services/converters/agent'
import { attachAgentRun, attachAgentTurn, streamAgentRun, streamAgentTurn } from '@/services/agent-stream'
import { projectPath } from '@/services/paths'
import { listOf, type WireRecord } from '@/services/wire'

/** 工具调用确认：会话内确认卡与审批队列共用同一个端点。 */
const confirmToolCall: Api['agentThreads']['confirm'] = (projectId, runId, callId, approve) =>
  http
    .post(projectPath(projectId, 'agent-runs', runId, 'confirmations', callId), { approve })
    .then(() => undefined)

export const agentApi = {
  agents: {
    modules: (projectId) =>
      http.get(projectPath(projectId, 'agents', 'modules')).then((r) => listOf<AgentModuleInfo>(r.data?.modules)),
    list: (projectId) =>
      http.get(projectPath(projectId, 'agents')).then((r) => listOf(r.data?.agents, toCloudAgent)),
    create: (projectId, body) =>
      http.post(projectPath(projectId, 'agents'), body).then((r) => toCloudAgent(r.data)),
    patch: (projectId, agentId, body) =>
      http
        .patch(projectPath(projectId, 'agents', agentId), body)
        .then((r) => toCloudAgent(r.data)),
    remove: (projectId, agentId) =>
      http.delete(projectPath(projectId, 'agents', agentId)).then(() => undefined),
    models: (projectId) =>
      http.get(projectPath(projectId, 'agents', 'models')).then((r) => ({
        default_model: String(r.data?.default_model || ''),
        models: listOf(r.data?.models).map((m: WireRecord) => ({
          provider: String(m?.provider || ''),
          name: String(m?.name || '')
        }))
      }))
  },

  agentThreads: {
    /** 协作协议能力探测。注意是项目级独立路由，不在 /agent-threads 下（后端 routes_agent.go）。 */
    capabilities: (projectId) =>
      http.get(projectPath(projectId, 'agent-workspace-capabilities')).then((r) => toAgentWorkspaceCapabilities(r.data)),
    list: (projectId, agentId = '') =>
      http.get(projectPath(projectId, 'agent-threads'), { params: agentId ? { agent_id: agentId } : {} }).then((r) => listOf(r.data?.threads, toAgentThread)),
    /** 分页（可按助手或标签过滤）。 */
    pageBy: (projectId, q = {}) => {
      const params: Record<string, unknown> = {}
      if (q.limit !== undefined) params.limit = q.limit
      if (q.cursor) params.cursor = q.cursor
      if (q.agentId) params.agent_id = q.agentId
      if (q.tagId) params.tag_id = q.tagId
      return http.get(projectPath(projectId, 'agent-threads'), { params }).then((r) => ({
        threads: listOf(r.data?.threads, toAgentThread),
        next_cursor: String(r.data?.next_cursor || '')
      }))
    },
    create: (projectId, opts = {}) => {
      const body: Record<string, unknown> = {
        ...(opts.title !== undefined ? { title: opts.title } : {}),
        ...(opts.agentId ? { agent_id: opts.agentId } : {}),
        ...(opts.tagId ? { tag_id: opts.tagId } : {}),
        ...(opts.dispatchMode ? { dispatch_mode: opts.dispatchMode } : {}),
        ...(opts.threadKind ? { thread_kind: opts.threadKind } : {}),
        ...(opts.source ? { source: opts.source } : {})
      }
      return http.post(projectPath(projectId, 'agent-threads'), body).then((r) => toAgentThread(r.data))
    },
    /** 单个会话详情（深链补齐）。 */
    get: (projectId, threadId) =>
      http.get(projectPath(projectId, 'agent-threads', threadId)).then((r) => toAgentThread(r.data)),
    rename: (projectId, threadId, title) =>
      http.patch(projectPath(projectId, 'agent-threads', threadId), { title }).then((r) => toAgentThread(r.data)),
    remove: (projectId, threadId) =>
      http
        .delete(projectPath(projectId, 'agent-threads', threadId))
        .then(() => undefined),
    runs: (projectId, threadId, limit = 20) =>
      http.get(projectPath(projectId, 'agent-threads', threadId, 'runs'), { params: { limit } })
        .then((r) => listOf<AgentRun>(r.data?.runs)),
    turns: (projectId, threadId, limit = 20) =>
      http.get(projectPath(projectId, 'agent-threads', threadId, 'turns'), { params: { limit } })
        .then((r) => listOf(r.data?.turns, toAgentTurn)),
    messages: (projectId, threadId) =>
      http
        .get(projectPath(projectId, 'agent-threads', threadId, 'messages'))
        .then((r) => listOf(r.data?.messages, toAgentMessage)),
    streamRun: streamAgentRun,
    /** v3：Plan 协作轮次（多目标 @）。 */
    streamTurn: streamAgentTurn,
    attachTurn: attachAgentTurn,
    attachRun: attachAgentRun,
    cancel: (projectId, runId) =>
      http
        .post(projectPath(projectId, 'agent-runs', runId, 'cancel'))
        .then((r) => r.data as AgentRun),
    active: (projectId) =>
      http
        .get(projectPath(projectId, 'agent-runs', 'active'))
        .then((r) => listOf<ActiveAgentRun>(r.data?.runs)),
    confirm: confirmToolCall
  },

  /** 工作台标签 CRUD。 */
  agentTags: {
    list: (projectId) =>
      http.get(projectPath(projectId, 'agent-tags')).then((r) => listOf(r.data?.tags, toAgentTag)),
    create: (projectId, input) =>
      http.post(projectPath(projectId, 'agent-tags'), fromAgentTagInput(input)).then((r) => toAgentTag(r.data)),
    patch: (projectId, tagId, input) =>
      http
        .patch(projectPath(projectId, 'agent-tags', tagId), fromAgentTagInput(input))
        .then((r) => toAgentTag(r.data)),
    remove: (projectId, tagId) =>
      http.delete(projectPath(projectId, 'agent-tags', tagId)).then(() => undefined),
    /** v2：加入已有成员，或「新建助手并加入」（二者互斥，newAgent 优先）。 */
    addMembers: (projectId, tagId, body) =>
      http.post(projectPath(projectId, 'agent-tags', tagId, 'members'), body)
        .then((r) => toAgentTag(r.data))
  },

  /** v2：协作轮次与人工待办。 */
  agentTurns: {
    get: (projectId, turnId) =>
      http.get(projectPath(projectId, 'agent-turns', turnId)).then((r) => toAgentTurn(r.data)),
    /** 取消整轮 / 重试单个任务 / 恢复。 */
    act: (projectId, turnId, body) =>
      http.post(projectPath(projectId, 'agent-turns', turnId, 'actions'), body)
        .then((r) => toAgentTurn(r.data)),
    /** 人工回复（continue / unable）；与工具审批严格分离。 */
    reply: (projectId, taskId, body) =>
      http.post(projectPath(projectId, 'agent-tasks', taskId, 'replies'), body)
        .then((r) => toAgentTurn(r.data)),
    confirmPlan: (projectId, turnId, body) =>
      http.post(projectPath(projectId, 'agent-turns', turnId, 'plan-confirmations'), {
        approve: body.approve,
        edits: body.edits ?? [],
        ...(body.expectedVersion !== undefined ? { expected_version: body.expectedVersion } : {})
      }).then((r) => toAgentTurn(r.data)),
    /** 待办列表与角标；assignee='me' 表示当前登录用户。 */
    humanTasks: (projectId, assignee = 'me', status = 'pending') =>
      http.get(projectPath(projectId, 'agent-human-tasks'), { params: { assignee, status } })
        .then((r) => ({
          tasks: listOf(r.data?.tasks, toAgentHumanTask),
          pending: Number(r.data?.pending || 0)
        }))
  },

  agentApprovals: {
    list: (projectId, status = '', limit = 50) =>
      http
        .get(projectPath(projectId, 'agent-approvals'), {
          params: { ...(status ? { status } : {}), limit }
        })
        .then((r) => ({
          approvals: listOf(r.data?.approvals, toAgentApproval),
          pending: Number(r.data?.pending || 0)
        })),
    resolve: confirmToolCall
  },

  mcpServers: {
    list: (projectId) =>
      http.get(projectPath(projectId, 'mcp', 'servers')).then((r) => {
        const servers = listOf(r.data?.servers)
        const builtin = listOf(r.data?.builtin)
        return { servers: servers.map(toMcpServer), builtin: builtin.map(toMcpBuiltinServer) }
      }),
    create: (projectId, body) =>
      http.post(projectPath(projectId, 'mcp', 'servers'), body).then((r) => toMcpServer(r.data)),
    update: (projectId, serverId, body) =>
      http
        .patch(projectPath(projectId, 'mcp', 'servers', serverId), body)
        .then((r) => toMcpServer(r.data)),
    remove: (projectId, serverId) =>
      http.delete(projectPath(projectId, 'mcp', 'servers', serverId)).then(() => undefined),
    test: (projectId, serverId) =>
      http
        .post(projectPath(projectId, 'mcp', 'servers', serverId, 'test'))
        .then((r) => toMcpTestResult(r.data)),
    setBuiltinEnabled: (projectId, name, enabled) =>
      http
        .put(projectPath(projectId, 'mcp', 'builtin', name, 'state'), { enabled })
        .then(() => undefined),
    testBuiltin: (projectId, name) =>
      http
        .post(projectPath(projectId, 'mcp', 'builtin', name, 'test'))
        .then((r) => toMcpTestResult(r.data))
  },
} satisfies Partial<Api>
