import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { defineComponent } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { api, resetApiMocks } from '@/test/api-mock'
import { findStarterTemplate } from '@/constants/starter-prompts'
import { useStarterRun, type StageOutcome } from '@/composables/useStarterRun'
import { MAX_ID_RETRIES, useStarterLaunch } from '@/composables/useStarterLaunch'
import { useAuthStore } from '@/stores/auth'
import { useProjectStore } from '@/stores/project'
import { takePendingStarter } from '@/utils/starter-storage'
import type { AgentToolCallCard } from '@/services/types'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

const card = (argv: string[], status = 'success', content = '{"ok":true}'): AgentToolCallCard => ({
  name: 'simplebase',
  arguments: JSON.stringify({ argv }),
  status,
  content
})

/** 成功结果：按阶段给出能通过判定的最小工具调用。 */
const ok = (stage: string, content = '完成'): StageOutcome => {
  const byStage: Record<string, AgentToolCallCard[]> = {
    plan: [],
    data: [card(['sql', 'exec'])],
    func: [card(['function', 'test'])],
    cron: [card(['cron', 'create'])],
    site: [card(['site', 'publish', '--prefix', 'web/'])],
    verify: [card(['site', 'status'])]
  }
  return { ok: true, content, cards: byStage[stage] ?? [] }
}

/** 记录每次 send，并让测试手动结束某一阶段。 */
function harness() {
  const calls: { text: string; stage: string; done: (o: StageOutcome) => void }[] = []
  const run = useStarterRun({ send: (text, stage, done) => calls.push({ text, stage, done }) })
  return { run, calls, last: () => calls[calls.length - 1] }
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.restoreAllMocks()
  resetApiMocks()
  sessionStorage.clear()
})

describe('useStarterRun：分阶段驱动', () => {
  it('上一阶段成功才发下一阶段，顺序与跳过规则正确', () => {
    const { run, calls, last } = harness()
    run.start(findStarterTemplate('portfolio')!, true) // 无 cron
    const seen: string[] = []
    for (let i = 0; i < 10 && calls.length > seen.length; i++) {
      const c = last()
      seen.push(c.stage)
      c.done(ok(c.stage, c.stage === 'verify' ? '<!-- sb:delivery {"tables":["works"]} -->' : '完成'))
    }
    expect(seen).toEqual(['plan', 'data', 'func', 'site', 'verify'])
    expect(run.finished.value).toBe(true)
    expect(run.delivery.value?.tables).toEqual(['works'])
    // 阶段提示来自 i18n，不是空串
    expect(calls.every((c) => c.text.length > 5)).toBe(true)
  })

  it('站点不可发布：跳过 site，verify 仍执行', () => {
    const { run, calls, last } = harness()
    run.start(findStarterTemplate('dashboard')!, false)
    const seen: string[] = []
    while (calls.length > seen.length) {
      const c = last()
      seen.push(c.stage)
      c.done(ok(c.stage))
    }
    expect(seen).toEqual(['plan', 'data', 'func', 'cron', 'verify'])
    expect(run.progress.value.find((p) => p.stage === 'site')?.status).toBe('skipped')
  })

  it('阶段失败停在原处，不发下一阶段', () => {
    const { run, calls, last } = harness()
    run.start(findStarterTemplate('dashboard')!, true)
    last().done(ok('plan'))
    last().done({ ok: true, content: '', cards: [] }) // data：没有成功的建表命令
    expect(calls.map((c) => c.stage)).toEqual(['plan', 'data'])
    expect(run.failure.value?.stage).toBe('data')
    expect(run.progress.value.find((p) => p.stage === 'data')?.status).toBe('failed')
  })

  it('run 异常结束（ok=false）同样停住', () => {
    const { run, calls, last } = harness()
    run.start(findStarterTemplate('dashboard')!, true)
    last().done({ ok: false, content: '半截', cards: [] })
    expect(calls).toHaveLength(1)
    expect(run.failure.value?.stage).toBe('plan')
  })

  it('CLI 信封 ok:false 即使工具 success 也算失败', () => {
    const { run, last } = harness()
    run.start(findStarterTemplate('dashboard')!, true)
    last().done(ok('plan'))
    last().done({ ok: true, content: 'x', cards: [card(['sql', 'exec'], 'success', '{"ok":false}')] })
    expect(run.failure.value?.stage).toBe('data')
  })

  it('重试本阶段：重新发送同一阶段', () => {
    const { run, calls, last } = harness()
    run.start(findStarterTemplate('dashboard')!, true)
    last().done(ok('plan'))
    last().done({ ok: true, content: '', cards: [] })
    run.retry()
    expect(calls.map((c) => c.stage)).toEqual(['plan', 'data', 'data'])
    expect(run.failure.value).toBeNull()
    last().done(ok('data'))
    expect(calls[calls.length - 1].stage).toBe('func')
  })

  it('跳过失败阶段：继续下一阶段', () => {
    const { run, calls, last } = harness()
    run.start(findStarterTemplate('dashboard')!, true)
    last().done(ok('plan'))
    last().done({ ok: true, content: '', cards: [] })
    run.skip()
    expect(run.progress.value.find((p) => p.stage === 'data')?.status).toBe('skipped')
    expect(calls[calls.length - 1].stage).toBe('func')
  })

  it('我来接手 / 手动发消息：退出自动模式，迟到的回调被丢弃', () => {
    const { run, calls, last } = harness()
    run.start(findStarterTemplate('dashboard')!, true)
    const stale = last()
    run.abandon()
    expect(run.active.value).toBe(false)
    stale.done(ok('plan')) // 旧回调不能再推进
    expect(calls).toHaveLength(1)
    // 没有失败时 retry / skip 是空操作
    run.retry()
    run.skip()
    expect(calls).toHaveLength(1)
  })

  it('重试后旧阶段的迟到回调不会重复推进', () => {
    const { run, calls, last } = harness()
    run.start(findStarterTemplate('dashboard')!, true)
    const first = last()
    first.done({ ok: false, content: '', cards: [] })
    run.retry()
    first.done(ok('plan')) // 第一次的回调又来了
    expect(calls.map((c) => c.stage)).toEqual(['plan', 'plan'])
  })

  it('verify 没有交付块也不算失败：退化为普通消息', () => {
    const { run, last } = harness()
    run.start(findStarterTemplate('portfolio')!, false)
    for (const stage of ['plan', 'data', 'func']) last().done(ok(stage))
    last().done({ ok: true, content: '都好了', cards: [] })
    expect(run.finished.value).toBe(true)
    expect(run.delivery.value).toBeNull()
  })
})

function launcherHarness(opts: { loggedIn: boolean; canCreate?: boolean } = { loggedIn: true }) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'home', component: { template: '<div />' } },
      { path: '/console/agents', name: 'agents', component: { template: '<div />' } }
    ]
  })
  const auth = useAuthStore()
  if (opts.loggedIn) auth.apiKey = 'k'
  if (opts.canCreate === false) {
    auth.user = { id: 'u', username: 'a', role: 'admin' } as never
    auth.accessToken = 't'
  }
  let launcher!: ReturnType<typeof useStarterLaunch>
  const Comp = defineComponent({
    setup() {
      launcher = useStarterLaunch()
      return () => null
    }
  })
  mount(Comp, { global: { plugins: [router] } })
  return { launcher, router, auth }
}

describe('useStarterLaunch：一键建项目', () => {
  it('建项目成功：生成 DNS 安全 ID，选中项目并跳转 agents?starter=', async () => {
    api.projects.create.mockResolvedValue({ id: 'ab12cd34', name: '看板', createdAt: 't' })
    api.projects.list.mockResolvedValue([])
    const { launcher, router } = launcherHarness()
    const done = await launcher.launch('dashboard', { name: ' 看板 ', engine: 'sqlite' })
    expect(done).toBe(true)
    const arg = api.projects.create.mock.calls[0][0]
    expect(arg.id).toMatch(/^[a-z0-9]{8}$/)
    expect(arg).toMatchObject({ name: '看板', engine: 'sqlite' })
    expect(useProjectStore().id).toBe('ab12cd34')
    expect(router.currentRoute.value.name).toBe('agents')
    expect(router.currentRoute.value.query.starter).toBe('dashboard')
  })

  it('409 冲突换 ID 重试，最多 3 次', async () => {
    const conflict = Object.assign(new Error('exists'), { status: 409 })
    api.projects.create.mockRejectedValueOnce(conflict).mockRejectedValueOnce(conflict).mockResolvedValue({ id: 'zzzzzzzz', name: 'n', createdAt: 't' })
    api.projects.list.mockResolvedValue([])
    const { launcher } = launcherHarness()
    expect(await launcher.launch('notice', { name: 'n', engine: 'duckdb' })).toBe(true)
    const ids = api.projects.create.mock.calls.map((c) => c[0].id)
    expect(ids).toHaveLength(3)
    expect(new Set(ids).size).toBeGreaterThan(1) // 每次都换了 ID
  })

  it('一直冲突：重试 MAX_ID_RETRIES 次后提示失败，不再跳转', async () => {
    const conflict = Object.assign(new Error('exists'), { status: 409 })
    api.projects.create.mockRejectedValue(conflict)
    const err = vi.spyOn(toast, 'error').mockImplementation(() => '' as never)
    const { launcher, router } = launcherHarness()
    expect(await launcher.launch('dashboard', { name: 'n', engine: 'duckdb' })).toBe(false)
    expect(api.projects.create).toHaveBeenCalledTimes(MAX_ID_RETRIES + 1)
    expect(err).toHaveBeenCalled()
    expect(router.currentRoute.value.name).not.toBe('agents')
  })

  it('非 409 错误不重试（权限 / 配额 / 校验）', async () => {
    api.projects.create.mockRejectedValue(Object.assign(new Error('quota'), { status: 403 }))
    vi.spyOn(toast, 'error').mockImplementation(() => '' as never)
    const { launcher } = launcherHarness()
    expect(await launcher.launch('dashboard', { name: 'n', engine: 'duckdb' })).toBe(false)
    expect(api.projects.create).toHaveBeenCalledTimes(1)
  })

  it('未登录：打开登录框并记 pendingStarter，不建项目', async () => {
    const { launcher, auth } = launcherHarness({ loggedIn: false })
    expect(await launcher.launch('dashboard', { name: '看板', engine: 'sqlite' })).toBe(false)
    expect(auth.loginOpen).toBe(true)
    // 正常引导，不能渲染成红色「重新登录」警示
    expect(auth.loginReason).toBe('')
    expect(api.projects.create).not.toHaveBeenCalled()
    expect(takePendingStarter()).toEqual({ templateId: 'dashboard', name: '看板', engine: 'sqlite' })
  })

  it('只读角色（admin）：提示并终止', async () => {
    const warn = vi.spyOn(toast, 'error').mockImplementation(() => '' as never)
    const { launcher } = launcherHarness({ loggedIn: true, canCreate: false })
    expect(await launcher.launch('dashboard', { name: 'n', engine: 'duckdb' })).toBe(false)
    expect(warn).toHaveBeenCalled()
    expect(api.projects.create).not.toHaveBeenCalled()
  })

  it('未知模板 / 空名字被拒绝', async () => {
    vi.spyOn(toast, 'error').mockImplementation(() => '' as never)
    vi.spyOn(toast, 'warning').mockImplementation(() => '' as never)
    const { launcher } = launcherHarness()
    expect(await launcher.launch('todo', { name: 'n', engine: 'duckdb' })).toBe(false) // B3 模板
    expect(await launcher.launch('dashboard', { name: '  ', engine: 'duckdb' })).toBe(false)
    expect(api.projects.create).not.toHaveBeenCalled()
  })

  it('进行中重复点击不会建第二个项目', async () => {
    let release!: (v: unknown) => void
    api.projects.create.mockReturnValue(new Promise((r) => (release = r)))
    api.projects.list.mockResolvedValue([])
    const { launcher } = launcherHarness()
    const first = launcher.launch('dashboard', { name: 'n', engine: 'duckdb' })
    await flushPromises()
    expect(await launcher.launch('dashboard', { name: 'n', engine: 'duckdb' })).toBe(false)
    release({ id: 'ab12cd34', name: 'n', createdAt: 't' })
    await first
    expect(api.projects.create).toHaveBeenCalledTimes(1)
  })
})
