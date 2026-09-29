import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import KvPanel from '@/components/databases/kv/KvPanel.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api, isMock: false }
})

// SCAN 第一页：两个 key；随后 execBatch 返回 [TYPE, PTTL]，非 string 再取长度
function mockScanPage(keys: string[], cursor = '0') {
  api.kv.exec.mockImplementation(async (_pid: string, body: { type: string; argvs?: string[] }) => {
    const argvs = body.argvs ?? []
    const cmd = argvs[0]?.toUpperCase()
    if (cmd === 'SCAN') return [cursor, keys]
    if (cmd === 'TYPE') return 'string'
    if (cmd === 'PTTL') return -1
    if (cmd === 'DBSIZE') return keys.length
    return null
  })
  api.kv.execBatch.mockImplementation(async (_pid: string, bodies: { argvs?: string[] }[]) =>
    Promise.all(
      bodies.map(async (b) => {
        const cmd = b.argvs?.[0]?.toUpperCase()
        if (cmd === 'TYPE') return 'string'
        if (cmd === 'PTTL') return -1
        return null
      })
    )
  )
}

async function mountPanel(props: Record<string, unknown> = {}) {
  const w = mount(KvPanel, {
    props: { projectId: 'p', ...props },
    global: { stubs: uiStubs }
  })
  await flushPromises()
  return w
}

describe('KvPanel (项目级 Key-Value 列表)', () => {
  beforeEach(() => resetApiMocks())

  it('SCAN 拉取 key 列表并渲染', async () => {
    mockScanPage(['session:1', 'user:1'])
    const w = await mountPanel()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'cmd',
      argvs: expect.arrayContaining(['SCAN', '0', 'COUNT', '100'])
    })
    expect(w.text()).toContain('session:1')
    expect(w.text()).toContain('user:1')
    expect(w.text()).toContain('永久')
    w.unmount()
  })

  it('TTL 倒计时文本：秒/小时分支', async () => {
    const vm0 = (await mountPanel()).vm as any
    expect(vm0.ttlText({ ttl_ms: null })).toBe('永久')
    vm0.fetchedAt = Date.now()
    expect(vm0.ttlText({ ttl_ms: 30_000 })).toBe('30s 后过期')
    expect(vm0.ttlText({ ttl_ms: 0 })).toBe('即将过期')
    expect(vm0.ttlText({ ttl_ms: 7_200_000 })).toBe('2h 后过期')
  })

  it('类型徽章映射', async () => {
    const vm0 = (await mountPanel()).vm as any
    expect(vm0.typeBadge('hash')).toBe('secondary')
    expect(vm0.typeBadge('list')).toBe('outline')
    expect(vm0.typeBadge('set')).toBe('default')
    expect(vm0.typeBadge('zset')).toBe('destructive')
    expect(vm0.typeBadge('string')).toBe('default')
  })

  it('空态展示新建入口；readonly 时 openCreate 不打开弹窗', async () => {
    mockScanPage([])
    const w = await mountPanel()
    expect(w.text()).toContain('暂无 Key')
    expect(w.text()).toContain('新建 Key')
    const vm = w.vm as { openCreate: () => void; createOpen: boolean }
    vm.openCreate()
    expect(vm.createOpen).toBe(true)

    const ro = await mountPanel({ readonly: true })
    expect(ro.text()).toContain('暂无 Key')
    expect(ro.text()).toContain('只读实例')
    expect(ro.text()).not.toContain('新建 Key')
    const roVm = ro.vm as { openCreate: () => void; createOpen: boolean }
    roVm.openCreate()
    expect(roVm.createOpen).toBe(false)
    ro.unmount()
    w.unmount()
  })

  it('游标分页：hasMore 时加载更多并追加', async () => {
    api.kv.exec.mockImplementation(async (_pid: string, body: { type: string; argvs?: string[] }) => {
      const argvs = body.argvs ?? []
      if (argvs[0]?.toUpperCase() === 'SCAN') {
        return argvs[1] === '0' ? ['user:1', ['session:1']] : ['0', ['user:1']]
      }
      if (argvs[0]?.toUpperCase() === 'TYPE') return 'string'
      if (argvs[0]?.toUpperCase() === 'PTTL') return -1
      return null
    })
    api.kv.execBatch.mockImplementation(async (_pid: string, bodies: { argvs?: string[] }[]) =>
      Promise.all(
        bodies.map(async (b) => {
          const cmd = b.argvs?.[0]?.toUpperCase()
          if (cmd === 'TYPE') return 'string'
          if (cmd === 'PTTL') return -1
          return null
        })
      )
    )
    const w = await mountPanel()
    expect(w.text()).toContain('加载更多')
    await w.findAll('button').find((b) => b.text().includes('加载更多'))!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('user:1')
    expect(w.text()).not.toContain('加载更多')
    w.unmount()
  })

  it('pattern 与 type 过滤参数传递到 SCAN', async () => {
    mockScanPage([])
    const w = await mountPanel()
    api.kv.exec.mockClear()
    const vm = w.vm as any
    vm.pattern = 'session:*'
    vm.typeFilter = 'string'
    await vm.reload()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'cmd',
      argvs: expect.arrayContaining(['MATCH', 'session:*', 'TYPE', 'string'])
    })
    w.unmount()
  })

  it('DEL 删除一行并本地移除', async () => {
    mockScanPage(['session:1'])
    const w = await mountPanel()
    api.kv.exec.mockClear()
    api.kv.exec.mockResolvedValue(1)
    const vm = w.vm as any
    await vm.removeKey({ key: 'session:1', type: 'string', len: null, ttl_ms: null })
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['DEL', 'session:1'] })
    expect(vm.rows.some((r: any) => r.key === 'session:1')).toBe(false)
    w.unmount()
  })

  it('RENAME 成功后刷新列表', async () => {
    mockScanPage(['a'])
    const w = await mountPanel()
    api.kv.exec.mockClear()
    api.kv.exec.mockResolvedValue('OK')
    const vm = w.vm as any
    vm.renameTarget = { key: 'a', type: 'string' }
    vm.renameValue = 'b'
    await vm.submitRename()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['RENAME', 'a', 'b'] })
    w.unmount()
  })

  it('加载失败走 toast.error', async () => {
    api.kv.exec.mockRejectedValue(new Error('boom'))
    const toastSpy = vi.fn()
    vi.stubGlobal('toast', { error: toastSpy })
    const w = await mountPanel()
    expect(w.text()).not.toContain('session:1')
    w.unmount()
    vi.unstubAllGlobals()
  })

  it('切换 projectId 重新加载', async () => {
    mockScanPage([])
    const w = await mountPanel({ projectId: 'p1' })
    await w.setProps({ projectId: 'p2' })
    await flushPromises()
    const calls = api.kv.exec.mock.calls.map((c: any[]) => c[0])
    expect(calls).toContain('p2')
    w.unmount()
  })
})
