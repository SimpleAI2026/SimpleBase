import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { defineComponent } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { api, resetApiMocks } from '@/test/api-mock'
import { MAX_ID_RETRIES, useStarterLaunch } from '@/composables/useStarterLaunch'
import { useAuthStore } from '@/stores/auth'
import { useProjectStore } from '@/stores/project'
import { takePendingStarter } from '@/utils/starter-storage'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

function launcherHarness(opts: { loggedIn: boolean; canCreate?: boolean } = { loggedIn: true }) {
  setActivePinia(createPinia())
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
  beforeEach(() => {
    vi.clearAllMocks()
    resetApiMocks()
    sessionStorage.clear()
  })
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
