import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import KvDetailSheet from '@/components/databases/kv/KvDetailSheet.vue'
import KvHashEditor from '@/components/databases/kv/editors/KvHashEditor.vue'
import KvListEditor from '@/components/databases/kv/editors/KvListEditor.vue'
import KvSetEditor from '@/components/databases/kv/editors/KvSetEditor.vue'
import KvStringEditor from '@/components/databases/kv/editors/KvStringEditor.vue'
import KvZSetEditor from '@/components/databases/kv/editors/KvZSetEditor.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api, isMock: false }
})

const meta = { key: 'k', type: 'string', len: null, ttl_ms: null, mtime_ms: 1, version: 1 }
const editorProps = { projectId: 'p', kvKey: 'k' }

function mockExec(impl: (cmd: string, argvs: string[]) => unknown) {
  api.kv.exec.mockImplementation(async (_pid: string, body: { type: string; argvs?: string[] }) => {
    if (body.type !== 'cmd') return null
    const argvs = body.argvs ?? []
    return impl(argvs[0]?.toUpperCase() ?? '', argvs)
  })
}

describe('KV 编辑器与 DetailSheet 分支补盲', () => {
  beforeEach(() => resetApiMocks())

  // ---------- KvDetailSheet ----------
  it('DetailSheet: 未选 key / 不支持类型 / readonly 无删除按钮', async () => {
    const w0 = mount(KvDetailSheet, {
      props: { open: true, projectId: 'p', kvKey: null },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    expect(w0.text()).toContain('未选择 Key')
    w0.unmount()

    // 未知类型 → 无编辑器渲染（空内容区）
    api.kv.execBatch.mockImplementation(async (_p, bodies) => Promise.all(bodies.map(() => null)))
    const w1 = mount(KvDetailSheet, {
      props: { open: true, projectId: 'p', kvKey: { ...meta, type: 'unknown' as never } },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    expect(w1.findComponent({ name: 'KvStringEditor' }).exists()).toBe(false)
    w1.unmount()

    // readonly → 无 SheetFooter 删除
    api.kv.execBatch.mockImplementation(async (_p, bodies) =>
      Promise.all(bodies.map((b) => (b.argvs?.[0]?.toUpperCase() === 'TYPE' ? 'string' : -1)))
    )
    const w2 = mount(KvDetailSheet, {
      props: { open: true, projectId: 'p', kvKey: meta, readonly: true },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    expect(w2.find('.confirm-action').exists()).toBe(false)
    w2.unmount()
  })

  it('DetailSheet: 各类型渲染对应编辑器（含元素数展示）', async () => {
    const editors = {
      string: 'KvStringEditor',
      hash: 'KvHashEditor',
      list: 'KvListEditor',
      set: 'KvSetEditor',
      zset: 'KvZSetEditor'
    } as const
    for (const [t, name] of Object.entries(editors)) {
      api.kv.execBatch.mockImplementation(async (_p, bodies) =>
        Promise.all(bodies.map((b) => (b.argvs?.[0]?.toUpperCase() === 'TYPE' ? t : b.argvs?.[0]?.toUpperCase() === 'PTTL' ? -1 : 3)))
      )
      const w = mount(KvDetailSheet, {
        props: { open: true, projectId: 'p', kvKey: { ...meta, type: t as never, len: 3 } },
        global: { stubs: uiStubs }
      })
      await flushPromises()
      expect(w.findComponent({ name }).exists()).toBe(true)
      if (t !== 'string') expect(w.text()).toContain('3 个元素')
      w.unmount()
    }
  })

  it('DetailSheet: meta 加载失败回退到 kvKey 展示', async () => {
    api.kv.execBatch.mockRejectedValueOnce(new Error('meta-load-fail'))
    const w = mount(KvDetailSheet, {
      props: { open: true, projectId: 'p', kvKey: meta },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    expect(vm.meta).toEqual(meta)
    w.unmount()
  })

  // ---------- 编辑器 readonly/loading 失败分支 ----------
  it('HashEditor: readonly 渲染与加载失败', async () => {
    api.kv.exec.mockRejectedValueOnce('raw')
    const w = mount(KvHashEditor, { props: { ...editorProps, readonly: true }, global: { stubs: uiStubs } })
    await flushPromises()
    const { toast } = await import('vue-sonner')
    expect(toast.error).toHaveBeenCalledWith('加载失败')
    expect(w.text()).toContain('暂无字段')
    // 保存失败兜底文案
    api.kv.exec.mockRejectedValueOnce('x')
    const vm = w.vm as any
    vm.editField = 'f'
    vm.editValue = 'v'
    await vm.saveEdit('f')
    expect(toast.error).toHaveBeenCalledWith('保存失败')
    w.unmount()
  })

  it('ListEditor: 加载失败与保存兜底', async () => {
    api.kv.exec.mockRejectedValueOnce('raw')
    const w = mount(KvListEditor, { props: editorProps, global: { stubs: uiStubs } })
    await flushPromises()
    const { toast } = await import('vue-sonner')
    expect(toast.error).toHaveBeenCalledWith('加载失败')
    toast.error.mockClear()
    mockExec((cmd) => (cmd === 'LRANGE' ? ['a'] : 'OK'))
    api.kv.exec.mockRejectedValueOnce('x')
    await (w.vm as any).pop('front')
    expect(toast.error).toHaveBeenCalledWith('弹出失败')
    toast.error.mockClear()
    api.kv.exec.mockRejectedValueOnce('y')
    ;(w.vm as any).pushValue = 'v'
    await (w.vm as any).push('back')
    expect(toast.error).toHaveBeenCalledWith('插入失败')
    w.unmount()
  })

  it('SetEditor: 全部失败兜底文案与 Error 实例路径', async () => {
    mockExec((cmd) => (cmd === 'SMEMBERS' ? ['a'] : 1))
    const w = mount(KvSetEditor, { props: editorProps, global: { stubs: uiStubs } })
    await flushPromises()
    const vm = w.vm as any
    const { toast } = await import('vue-sonner')
    api.kv.exec.mockRejectedValueOnce('x')
    vm.newMember = 'n'
    await vm.addMember()
    expect(toast.error).toHaveBeenCalledWith('添加失败')
    api.kv.exec.mockRejectedValueOnce('y')
    await vm.removeMember('a')
    expect(toast.error).toHaveBeenCalledWith('移除失败')
    api.kv.exec.mockRejectedValueOnce('z')
    await vm.load()
    expect(toast.error).toHaveBeenCalledWith('加载失败')
    // Error 实例路径
    api.kv.exec.mockRejectedValueOnce(new Error('boom'))
    await vm.load()
    expect(toast.error).toHaveBeenCalledWith('boom')
    api.kv.exec.mockRejectedValueOnce(new Error('add-boom'))
    vm.newMember = 'n2'
    await vm.addMember()
    expect(toast.error).toHaveBeenCalledWith('add-boom')
    api.kv.exec.mockRejectedValueOnce(new Error('rm-boom'))
    await vm.removeMember('a')
    expect(toast.error).toHaveBeenCalledWith('rm-boom')
    w.unmount()
  })

  it('StringEditor: readonly 无操作栏与加载失败', async () => {
    api.kv.exec.mockRejectedValueOnce('raw')
    const w = mount(KvStringEditor, { props: { ...editorProps, readonly: true }, global: { stubs: uiStubs } })
    await flushPromises()
    const { toast } = await import('vue-sonner')
    expect(toast.error).toHaveBeenCalledWith('加载失败')
    expect(w.text()).not.toContain('INCR')
    mockExec((cmd) => (cmd === 'GET' ? 'v' : 'OK'))
    await (w.vm as any).load()
    await flushPromises()
    // readonly 时保存失败兜底
    api.kv.exec.mockRejectedValueOnce('x')
    const vm = w.vm as any
    vm.value = 'changed'
    await vm.save()
    expect(toast.error).toHaveBeenCalledWith('保存失败')
    // 加载兜底：非 Error 拒绝
    api.kv.exec.mockRejectedValueOnce('raw2')
    await vm.load()
    expect(toast.error).toHaveBeenCalledWith('加载失败')
    w.unmount()
  })

  it('ZSetEditor: 失败兜底与 readonly 渲染', async () => {
    mockExec((cmd) => (cmd === 'ZRANGE' ? ['e', '1'] : 1))
    const w = mount(KvZSetEditor, { props: editorProps, global: { stubs: uiStubs } })
    await flushPromises()
    const vm = w.vm as any
    const { toast } = await import('vue-sonner')
    api.kv.exec.mockRejectedValueOnce('x')
    vm.newElem = 'm'
    vm.newScore = '1'
    await vm.addMember()
    expect(toast.error).toHaveBeenCalledWith('添加失败')
    api.kv.exec.mockRejectedValueOnce('y')
    await vm.removeMember('e')
    expect(toast.error).toHaveBeenCalledWith('移除失败')
    api.kv.exec.mockRejectedValueOnce('z')
    await vm.load()
    expect(toast.error).toHaveBeenCalledWith('加载失败')
    // readonly 渲染空态（重置 mock 返回空成员）
    mockExec((cmd) => (cmd === 'ZRANGE' ? [] : 1))
    const ro = mount(KvZSetEditor, { props: { ...editorProps, readonly: true }, global: { stubs: uiStubs } })
    await flushPromises()
    expect(ro.text()).toContain('暂无成员')
    ro.unmount()
    w.unmount()
  })
})
