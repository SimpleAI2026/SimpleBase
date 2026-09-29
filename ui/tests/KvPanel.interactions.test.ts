import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import KvPanel from '@/components/databases/kv/KvPanel.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api, isMock: false }
})

const sampleKeys = [
  { key: 'session:1', type: 'string', len: null, ttl_ms: null, mtime_ms: 1, version: 1 },
  { key: 'user:1', type: 'hash', len: 3, ttl_ms: 60000, mtime_ms: 1, version: 2 }
]

/** SCAN 第一页 + execBatch(TYPE/PTTL) + 类型长度命令的联动 mock */
function mockScanPage(keys = sampleKeys, cursor = '0') {
  api.kv.exec.mockImplementation(async (_pid: string, body: { type: string; argvs?: string[] }) => {
    const argvs = body.argvs ?? []
    const cmd = argvs[0]?.toUpperCase()
    if (cmd === 'SCAN') return [cursor, keys.map((k) => k.key)]
    if (cmd === 'TYPE') return 'string'
    if (cmd === 'PTTL') return -1
    if (cmd === 'DBSIZE') return keys.length
    return null
  })
  api.kv.execBatch.mockImplementation(async (_pid: string, bodies: { argvs?: string[] }[]) =>
    Promise.all(
      bodies.map((b) => {
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

describe('KvPanel 模板交互', () => {
  beforeEach(() => resetApiMocks())

  it('工具栏按钮触发对应弹窗/抽屉', async () => {
    mockScanPage()
    const w = await mountPanel()
    const vm = w.vm as any
    // 新建 Key（入口在页面标题右侧，面板暴露 openCreate）
    vm.openCreate()
    expect(vm.createOpen).toBe(true)
    // 行操作：TTL / 重命名
    await w.findAll('button').find((b) => b.text() === 'TTL')!.trigger('click')
    expect(vm.ttlOpen).toBe(true)
    await w.findAll('button').find((b) => b.text() === '重命名')!.trigger('click')
    expect(vm.renameOpen).toBe(true)
    expect(vm.renameValue).toBe('session:1')
    w.unmount()
  })

  it('搜索回车触发 reload 并带 MATCH 参数', async () => {
    mockScanPage([])
    const w = await mountPanel()
    api.kv.exec.mockClear()
    mockScanPage([])
    const vm = w.vm as any
    vm.pattern = 'user:*'
    await vm.reload()
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'cmd',
      argvs: expect.arrayContaining(['SCAN', '0', 'COUNT', '100', 'MATCH', 'user:*'])
    })
    // 页面右上角刷新走 reload
    api.kv.exec.mockClear()
    mockScanPage([])
    await vm.reload()
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalled()
    w.unmount()
  })

  it('确认删除按钮（ConfirmAction 插槽）触发 DEL', async () => {
    mockScanPage()
    const w = await mountPanel()
    api.kv.exec.mockImplementation(async () => {
      const argvs: string[] = []
      return argvs.length ? argvs : 1
    })
    mockScanPage([sampleKeys[1]])
    const del = w.findAll('.confirm-action').find((c) => c.text().includes('删除'))!
    api.kv.exec.mockResolvedValueOnce(1)
    await del.trigger('click')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['DEL', 'session:1'] })
    expect(w.text()).not.toContain('session:1')
    w.unmount()
  })

  it('空态渲染提示文案；openCreate 打开新建弹窗', async () => {
    mockScanPage([])
    const w = await mountPanel()
    expect(w.text()).toContain('暂无 Key')
    const vm = w.vm as any
    await w.get('.empty-action').trigger('click')
    expect(vm.createOpen).toBe(true)
    w.unmount()
  })

  it('重命名弹窗：SbModal ok 提交 RENAME', async () => {
    mockScanPage()
    const w = await mountPanel()
    const vm = w.vm as any
    vm.openRename(sampleKeys[0])
    await flushPromises()
    vm.renameValue = 'session:9'
    api.kv.exec.mockResolvedValueOnce('OK')
    mockScanPage([])
    await w.find('.sb-ok').trigger('click')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['RENAME', 'session:1', 'session:9'] })
    expect(vm.renameOpen).toBe(false)
    w.unmount()
  })

  it('重命名失败 toast 报错且弹窗保持打开', async () => {
    mockScanPage()
    api.kv.exec.mockRejectedValueOnce(new Error('conflict'))
    const w = await mountPanel()
    const vm = w.vm as any
    vm.openRename(sampleKeys[0])
    await flushPromises()
    vm.renameValue = 'session:9'
    api.kv.exec.mockRejectedValueOnce(new Error('conflict'))
    await vm.submitRename()
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('conflict')
    expect(vm.renameOpen).toBe(true)
    expect(vm.renaming).toBe(false)
    w.unmount()
  })

  it('重命名同名/空名直接关闭不调 API', async () => {
    mockScanPage()
    const w = await mountPanel()
    const vm = w.vm as any
    vm.openRename(sampleKeys[0])
    await flushPromises()
    api.kv.exec.mockClear()
    mockScanPage()
    vm.renameValue = 'session:1' // 同名
    await vm.submitRename()
    expect(api.kv.exec).not.toHaveBeenCalled()
    expect(vm.renameOpen).toBe(false)
    w.unmount()
  })

  it('删除失败 toast 报错且行保留', async () => {
    mockScanPage()
    const w = await mountPanel()
    const vm = w.vm as any
    expect(vm.rows).toHaveLength(2)
    // DEL 失败
    api.kv.exec.mockImplementation(async (_pid: string, body: { type: string; argvs?: string[] }) => {
      const argvs = body.argvs ?? []
      if (argvs[0]?.toUpperCase() === 'DEL') throw new Error('denied')
      return null
    })
    await vm.removeKey(sampleKeys[0])
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('denied')
    // 失败时行未被本地移除
    expect(vm.rows.some((r: { key: string }) => r.key === 'session:1')).toBe(true)
    expect(vm.rows).toHaveLength(2)
    w.unmount()
  })

  it('加载更多：带游标追加且 hasMore 归零后隐藏', async () => {
    // 第一页带游标
    api.kv.exec.mockImplementation(async (_pid: string, body: { type: string; argvs?: string[] }) => {
      const argvs = body.argvs ?? []
      if (argvs[0]?.toUpperCase() === 'SCAN') {
        return argvs[1] === '0' ? ['c1', ['session:1']] : ['0', ['tag:1']]
      }
      if (argvs[0]?.toUpperCase() === 'TYPE') return 'string'
      if (argvs[0]?.toUpperCase() === 'PTTL') return -1
      return null
    })
    api.kv.execBatch.mockImplementation(async (_pid: string, bodies: { argvs?: string[] }[]) =>
      Promise.all(bodies.map((b) => (b.argvs?.[0]?.toUpperCase() === 'TYPE' ? 'string' : -1)))
    )
    const w = await mountPanel()
    const vm = w.vm as any
    expect(vm.hasMore).toBe(true)
    await vm.loadMore()
    await flushPromises()
    // 第二页 SCAN 用游标 c1
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'cmd',
      argvs: expect.arrayContaining(['SCAN', 'c1'])
    })
    expect(vm.rows).toHaveLength(2)
    expect(vm.hasMore).toBe(false)
    w.unmount()
  })

  it('onCreated 清空筛选并刷新；onDeleted 关闭详情', async () => {
    mockScanPage()
    const w = await mountPanel()
    const vm = w.vm as any
    vm.pattern = 'zzz'
    vm.typeFilter = 'hash'
    api.kv.exec.mockClear()
    mockScanPage()
    await vm.onCreated('newkey')
    await flushPromises()
    expect(vm.pattern).toBe('')
    expect(vm.typeFilter).toBe('all')
    expect(api.kv.exec).toHaveBeenCalled()
    vm.detailOpen = true
    api.kv.exec.mockClear()
    mockScanPage()
    await vm.onDeleted()
    await flushPromises()
    expect(vm.detailOpen).toBe(false)
    expect(api.kv.exec).toHaveBeenCalled()
    w.unmount()
  })

  it('子组件 v-model:open 内联回调关闭弹窗', async () => {
    mockScanPage()
    const w = await mountPanel()
    const vm = w.vm as any
    vm.createOpen = true
    vm.ttlOpen = true
    vm.detailOpen = true
    await flushPromises()
    await w.findComponent({ name: 'KvCreateKeyModal' }).vm.$emit('update:open', false)
    expect(vm.createOpen).toBe(false)
    await w.findComponent({ name: 'KvTtlModal' }).vm.$emit('update:open', false)
    expect(vm.ttlOpen).toBe(false)
    await w.findComponent({ name: 'KvDetailSheet' }).vm.$emit('update:open', false)
    expect(vm.detailOpen).toBe(false)
    // 重命名 SbModal 的 update:open 内联回调
    vm.renameOpen = true
    await flushPromises()
    await w.find('.sb-cancel').trigger('click')
    expect(vm.renameOpen).toBe(false)
    w.unmount()
  })

  it('TTL 全消失后定时器清理', async () => {
    // 第一页带 TTL；第二页不带
    api.kv.exec.mockImplementation(async (_pid: string, body: { type: string; argvs?: string[] }) => {
      const argvs = body.argvs ?? []
      const call = argvs[0]?.toUpperCase()
      if (call === 'SCAN') return ['0', ['k1']]
      if (call === 'PTTL') return body.argvs?.[1] === 'k1' ? 60000 : -1
      if (call === 'TYPE') return 'string'
      return null
    })
    api.kv.execBatch.mockImplementation(async (_pid: string, bodies: { argvs?: string[] }[]) =>
      Promise.all(
        bodies.map((b) => {
          const cmd = b.argvs?.[0]?.toUpperCase()
          if (cmd === 'TYPE') return 'string'
          if (cmd === 'PTTL') return b.argvs?.[1] === 'k1' ? 60000 : -1
          return null
        })
      )
    )
    const w = await mountPanel()
    const vm = w.vm as any
    expect(vm.timer).toBeTruthy()
    // 重载后无 TTL → 定时器清理
    api.kv.exec.mockImplementation(async (_pid: string, body: { type: string; argvs?: string[] }) => {
      const argvs = body.argvs ?? []
      const call = argvs[0]?.toUpperCase()
      if (call === 'SCAN') return ['0', ['k2']]
      if (call === 'TYPE') return 'string'
      if (call === 'PTTL') return -1
      return null
    })
    api.kv.execBatch.mockImplementation(async (_pid: string, bodies: { argvs?: string[] }[]) =>
      Promise.all(bodies.map((b) => (b.argvs?.[0]?.toUpperCase() === 'TYPE' ? 'string' : -1)))
    )
    await vm.reload()
    await flushPromises()
    expect(vm.timer).toBeNull()
    w.unmount()
  })
})
