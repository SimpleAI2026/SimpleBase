// 会话工作台的拆分后 composable：待办聚合、发送辅助、会话能力、重命名弹窗、助手目录。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { computed, reactive, ref } from 'vue'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { useHumanTasks } from '@/composables/useHumanTasks'
import { collectMentionTargets, COLLAB_WARNINGS, notifyCollabError } from '@/composables/useAgentSender'
import { useRenameThread, useThreadCapabilities } from '@/composables/useThreadCapabilities'
import { useAgentDirectory } from '@/composables/useAgentDirectory'
import type { AgentHumanTask, AgentTask, AgentThread } from '@/services/types'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

const thread = (id: string, over: Partial<AgentThread> = {}): AgentThread => ({ id, title: id, created_at: 't', updated_at: 't', ...over })
const humanTask = (over: Partial<AgentHumanTask>): AgentHumanTask => ({ taskId: 'h', threadId: 't1', ...over }) as AgentHumanTask

describe('useHumanTasks', () => {
  beforeEach(() => resetApiMocks())

  it('按任务自带的群聚合；缺失时回退到会话所属群，再回退 general', async () => {
    const ws = { pages: reactive({ g1: { threads: [thread('t1')] }, g2: { threads: [thread('t2')] } }) }
    api.agentTurns.humanTasks.mockResolvedValue({
      tasks: [
        humanTask({ taskId: 'a', tagId: 'g2', threadId: 't1' }),
        humanTask({ taskId: 'b', threadId: 't1' }),
        humanTask({ taskId: 'c', threadId: 't2' }),
        humanTask({ taskId: 'd', threadId: 'unknown' })
      ]
    })
    const h = useHumanTasks({ projectId: () => 'p1', ws: ws as never })
    await h.loadHumanTasks()
    expect(api.agentTurns.humanTasks).toHaveBeenCalledWith('p1', 'me', 'pending')
    expect(h.pendingByGroup.value).toEqual({ g2: 2, g1: 1, general: 1 })
  })

  it('没有项目不请求；失败（未登录 / 旧环境）静默清空角标', async () => {
    const ws = { pages: reactive({}) }
    const none = useHumanTasks({ projectId: () => '', ws: ws as never })
    await none.loadHumanTasks()
    expect(api.agentTurns.humanTasks).not.toHaveBeenCalled()

    const h = useHumanTasks({ projectId: () => 'p1', ws: ws as never })
    h.humanTasks.value = [humanTask({ taskId: 'x' })]
    api.agentTurns.humanTasks.mockRejectedValueOnce(new Error('401'))
    await h.loadHumanTasks()
    expect(h.humanTasks.value).toEqual([])
    expect(h.pendingByGroup.value).toEqual({})
  })
})

describe('useAgentSender 的纯辅助', () => {
  it('collectMentionTargets：助手去重去空白，human 单独标记', () => {
    expect(collectMentionTargets([])).toEqual({ agentIds: [], wantsHuman: false })
    expect(
      collectMentionTargets([{ agent_id: ' a1 ' }, { agent_id: 'a1' }, { agent_id: '  ' }, { kind: 'human' }, { agent_id: 'a2' }])
    ).toEqual({ agentIds: ['a1', 'a2'], wantsHuman: true })
  })

  it('notifyCollabError：登记的错误码 warning，其余 error（非 Error 用兜底文案）', () => {
    for (const code of Object.keys(COLLAB_WARNINGS)) {
      vi.mocked(toast.warning).mockClear()
      notifyCollabError(Object.assign(new Error('x'), { code }))
      expect(toast.warning).toHaveBeenCalledTimes(1)
    }
    vi.mocked(toast.error).mockClear()
    notifyCollabError(new Error('boom'))
    expect(toast.error).toHaveBeenCalledWith('boom')
    notifyCollabError('weird')
    expect(toast.error).toHaveBeenLastCalledWith('运行失败')
  })
})

describe('useThreadCapabilities', () => {
  const agent = (module: string, tools: string[] = []) => ({ id: 'a', module, tool_ids: tools, name: 'A' }) as never
  function make(over: { thread?: Partial<AgentThread> | null; agent?: unknown; modules?: Array<{ id: string; sandbox_available?: boolean }>; tasks?: AgentTask[] }) {
    const activeThread = computed(() => (over.thread === null ? undefined : thread('t', over.thread ?? {})))
    return useThreadCapabilities({
      activeThread,
      activeThreadAgent: computed(() => over.agent as never),
      activeThreadAgentName: computed(() => 'A'),
      modules: ref((over.modules ?? []) as never),
      collabTasks: ref(over.tasks ?? [])
    })
  }

  it('无会话、collaborative 都按协作会话；固定助手的会话不是（auto 已下线）', () => {
    expect(make({ thread: null }).isCollaborativeThread.value).toBe(true)
    expect(make({ thread: { dispatch_mode: 'collaborative' } }).isCollaborativeThread.value).toBe(true)
    expect(make({ thread: {} }).isCollaborativeThread.value).toBe(false)
  })

  it('轮次任务只在协作会话展示，等人的任务单独过滤', () => {
    const tasks = [{ id: '1', targetKind: 'agent' }, { id: '2', targetKind: 'human' }] as AgentTask[]
    const collab = make({ thread: { dispatch_mode: 'collaborative' }, tasks })
    expect(collab.activeTurnTasks.value).toHaveLength(2)
    expect(collab.humanTasksInTurn.value.map((x) => x.id)).toEqual(['2'])
    expect(make({ thread: {}, tasks }).activeTurnTasks.value).toEqual([])
  })

  it('能力提示：沙盒不可用 / CLI 技能缺失；模块在则无提示；沙盒工具改变提示文案', () => {
    expect(make({ agent: agent('sandbox'), modules: [{ id: 'sandbox', sandbox_available: false }] }).activeCapabilityError.value).not.toBe('')
    expect(make({ agent: agent('sandbox'), modules: [{ id: 'sandbox', sandbox_available: true }] }).activeCapabilityError.value).toBe('')
    expect(make({ agent: agent('kv'), modules: [] }).activeCapabilityError.value).not.toBe('')
    expect(make({ agent: agent('kv'), modules: [{ id: 'kv' }] }).activeCapabilityError.value).toBe('')
    expect(make({ agent: undefined }).activeCapabilityError.value).toBe('')

    const withSandbox = make({ agent: agent('database', ['sandbox_exec']), modules: [{ id: 'database' }] })
    expect(withSandbox.activeAgentHasSandboxTools.value).toBe(true)
    expect(withSandbox.toolsHint.value).not.toBe(make({ agent: agent('database'), modules: [{ id: 'database' }] }).toolsHint.value)
    const broken = make({ agent: agent('kv'), modules: [] })
    expect(broken.toolsHint.value).toBe(broken.activeCapabilityError.value)
  })
})

describe('useRenameThread', () => {
  it('start 回填标题；submit 提交新标题并关闭；相同或空标题直接关闭不请求', async () => {
    const rename = vi.fn().mockResolvedValue(undefined)
    const r = useRenameThread({ threadsOf: () => [thread('t1', { title: '旧' })], rename })
    r.start('missing')
    expect(r.open.value).toBe(false)

    r.start('t1')
    expect([r.open.value, r.value.value]).toEqual([true, '旧'])
    await r.submit()
    expect(rename).not.toHaveBeenCalled()
    expect(r.open.value).toBe(false)

    r.start('t1')
    r.value.value = '  新  '
    const pending = r.submit()
    expect(r.saving.value).toBe(true)
    await pending
    expect(rename).toHaveBeenCalledWith('t1', '新')
    expect([r.open.value, r.saving.value]).toEqual([false, false])

    r.start('t1')
    r.value.value = '   '
    await r.submit()
    expect(rename).toHaveBeenCalledTimes(1)
  })

  it('提交失败也复位 saving（不留半开状态）', async () => {
    const r = useRenameThread({ threadsOf: () => [thread('t1', { title: '旧' })], rename: () => Promise.reject(new Error('x')) })
    r.start('t1')
    r.value.value = '新'
    await expect(r.submit()).rejects.toThrow('x')
    expect(r.saving.value).toBe(false)
  })
})

describe('useAgentDirectory', () => {
  beforeEach(() => resetApiMocks())

  it('加载助手与模块；项目在请求期间切换则丢弃旧响应', async () => {
    let project = 'p1'
    api.agents.modules.mockResolvedValue([{ id: 'database' }])
    api.agents.list.mockResolvedValue([{ id: 'a1', name: 'Database', module: 'database', builtin_key: 'database' }])
    const d = useAgentDirectory({ projectId: () => project })
    await d.loadAgents()
    expect(d.agents.value.map((a) => a.id)).toEqual(['a1'])
    expect(d.modules.value).toEqual([{ id: 'database' }])
    expect(d.rawAgentById.value.get('a1')?.module).toBe('database')
    expect(d.agentNamesById.value.a1).toBeTruthy()

    let resolve!: (v: unknown) => void
    api.agents.list.mockReturnValueOnce(new Promise((r) => { resolve = r }))
    const loading = d.loadAgents()
    project = 'p2'
    await Promise.resolve()
    resolve([{ id: 'stale', name: 'S', module: 'x' }])
    await loading
    expect(d.agents.value.map((a) => a.id)).toEqual(['a1'])
  })

  it('模型候选：用 /agents/models；无候选时回退默认模型；失败清空并提示', async () => {
    api.agents.models.mockResolvedValueOnce({ default_model: 'gpt-x', models: [] })
    const d = useAgentDirectory({ projectId: () => 'p1' })
    await d.loadModelOptions()
    expect([d.defaultModel.value, d.modelOptions.value]).toEqual(['gpt-x', ['gpt-x']])

    api.agents.models.mockResolvedValueOnce({ default_model: 'm1', models: [{ name: 'm1' }, { name: 'm2' }] })
    await d.loadModelOptions()
    expect(d.modelOptions.value).toEqual(['m1', 'm2'])

    api.agents.models.mockRejectedValueOnce(new Error('offline'))
    await d.loadModelOptions()
    expect([d.defaultModel.value, d.modelOptions.value]).toEqual(['', []])
    expect(toast.error).toHaveBeenCalledWith('offline')
  })

  it('模块加载失败只提示，不影响助手列表', async () => {
    api.agents.modules.mockRejectedValueOnce(new Error('mod-offline'))
    api.agents.list.mockResolvedValue([{ id: 'a1', name: 'N', module: 'm' }])
    const d = useAgentDirectory({ projectId: () => 'p1' })
    await d.loadAgents()
    expect(toast.error).toHaveBeenCalledWith('mod-offline')
    expect(d.agents.value).toHaveLength(1)
  })
})
