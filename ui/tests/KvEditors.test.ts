import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import KvStringEditor from '@/components/databases/kv/editors/KvStringEditor.vue'
import KvHashEditor from '@/components/databases/kv/editors/KvHashEditor.vue'
import KvListEditor from '@/components/databases/kv/editors/KvListEditor.vue'
import KvSetEditor from '@/components/databases/kv/editors/KvSetEditor.vue'
import KvZSetEditor from '@/components/databases/kv/editors/KvZSetEditor.vue'

import KvDetailSheet from '@/components/databases/kv/KvDetailSheet.vue'
import DataTabs from '@/components/databases/DataTabs.vue'
import KvCreateKeyModal from '@/components/databases/kv/KvCreateKeyModal.vue'
import KvPanel from '@/components/databases/kv/KvPanel.vue'
import KvTtlModal from '@/components/databases/kv/KvTtlModal.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

const base = { projectId: 'p', kvKey: 'k' }

/** exec mock 快捷方式：按命令名返回固定回复 */
function mockExec(impl: (cmd: string, argvs: string[]) => unknown) {
  api.kv.exec.mockImplementation(async (_pid: string, body: { type: string; argvs?: string[] }) => {
    if (body.type !== 'cmd') return null
    const argvs = body.argvs ?? []
    return impl(argvs[0]?.toUpperCase() ?? '', argvs)
  })
}

describe('KV 类型编辑器', () => {
  beforeEach(() => resetApiMocks())

  it('string: 加载值并保存', async () => {
    mockExec((cmd) => (cmd === 'GET' ? 'hello' : 'OK'))
    const w = mount(KvStringEditor, { props: base, global: { stubs: uiStubs } })
    await flushPromises()
    expect((w.find('textarea').element as HTMLTextAreaElement).value).toBe('hello')
    await w.find('textarea').setValue('world')
    const vm = w.vm as any
    await vm.save()
    // 保存走类型化 String（keep_ttl 保持原 TTL）
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'String',
      args: { key: 'k', value: 'world', keep_ttl: true }
    })
    w.unmount()
  })

  it('string: INCR 步进（INCRBY）', async () => {
    mockExec((cmd, argvs) => {
      if (cmd === 'GET') return '5'
      if (cmd === 'INCRBY') return Number(argvs[2]) + 5
      return null
    })
    const w = mount(KvStringEditor, { props: base, global: { stubs: uiStubs } })
    await flushPromises()
    const vm = w.vm as any
    vm.delta = '3'
    await vm.incr()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['INCRBY', 'k', '3'] })
    expect((w.find('textarea').element as HTMLTextAreaElement).value).toBe('8')
    w.unmount()
  })

  it('hash: 渲染字段并添加（HGETALL 扁平数组）', async () => {
    mockExec((cmd) => (cmd === 'HGETALL' ? ['name', 'alice'] : 1))
    const w = mount(KvHashEditor, { props: base, global: { stubs: uiStubs } })
    await flushPromises()
    expect(w.text()).toContain('name')
    expect(w.text()).toContain('alice')
    const vm = w.vm as any
    vm.newField = 'age'
    vm.newValue = '30'
    await vm.addField()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'Hash',
      args: { key: 'k', fields: { age: '30' } }
    })
    w.unmount()
  })

  it('list: 头插/弹尾（LPUSH / RPOP）', async () => {
    mockExec((cmd) => {
      if (cmd === 'LRANGE') return ['a', 'b']
      if (cmd === 'RPOP') return 'b'
      return 2
    })
    const w = mount(KvListEditor, { props: base, global: { stubs: uiStubs } })
    await flushPromises()
    expect(w.text()).toContain('a')
    const vm = w.vm as any
    vm.pushValue = 'z'
    await vm.push('front')
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'List',
      args: { key: 'k', elems: ['z'], side: 'front' }
    })
    await vm.pop('back')
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['RPOP', 'k'] })
    w.unmount()
  })

  it('set: 添加与移除成员（Set / SREM）', async () => {
    mockExec((cmd) => (cmd === 'SMEMBERS' ? ['x'] : 1))
    const w = mount(KvSetEditor, { props: base, global: { stubs: uiStubs } })
    await flushPromises()
    expect(w.text()).toContain('x')
    const vm = w.vm as any
    vm.newMember = 'y'
    await vm.addMember()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'Set', args: { key: 'k', elems: ['y'] } })
    await vm.removeMember('x')
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['SREM', 'k', 'x'] })
    w.unmount()
  })

  it('zset: 渲染成员与改分（ZRANGE WITHSCORES 扁平数组）', async () => {
    mockExec((cmd) => (cmd === 'ZRANGE' ? ['a', '90'] : 1))
    const w = mount(KvZSetEditor, { props: base, global: { stubs: uiStubs } })
    await flushPromises()
    expect(w.text()).toContain('a')
    expect(w.text()).toContain('90')
    const vm = w.vm as any
    vm.newElem = 'b'
    vm.newScore = '85'
    await vm.addMember()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'ZSet',
      args: { key: 'k', items: [{ elem: 'b', score: 85 }] }
    })
    w.unmount()
  })

  it('zset: 排序切换带 REV 参数重新加载', async () => {
    mockExec((cmd) => (cmd === 'ZRANGE' ? [] : 1))
    const w = mount(KvZSetEditor, { props: base, global: { stubs: uiStubs } })
    await flushPromises()
    api.kv.exec.mockClear()
    mockExec((cmd) => (cmd === 'ZRANGE' ? [] : 1))
    await w.findAll('button').find((b) => b.text().includes('分数降序'))!.trigger('click')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'cmd',
      argvs: ['ZRANGE', 'k', '0', '199', 'WITHSCORES', 'REV']
    })
    w.unmount()
  })

  it('hash: 编辑保存/删除字段与失败 toast', async () => {
    mockExec((cmd) => (cmd === 'HGETALL' ? ['name', 'alice'] : 1))
    const w = mount(KvHashEditor, { props: base, global: { stubs: uiStubs } })
    await flushPromises()
    const vm = w.vm as any
    vm.startEdit({ field: 'name', value: 'alice' })
    vm.editValue = 'bob'
    await vm.saveEdit('name')
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'Hash',
      args: { key: 'k', fields: { name: 'bob' } }
    })
    api.kv.exec.mockRejectedValueOnce(new Error('boom'))
    await vm.removeField('name')
    expect(toast.error).toHaveBeenCalledWith('boom')
    api.kv.exec.mockRejectedValueOnce('oops')
    vm.newField = 'age'
    vm.newValue = '1'
    await vm.addField()
    expect(toast.error).toHaveBeenCalledWith('保存失败')
    w.unmount()
  })

  it('list: 越界保存失败 toast（LSET）', async () => {
    mockExec((cmd) => (cmd === 'LRANGE' ? ['a'] : 'OK'))
    const w = mount(KvListEditor, { props: base, global: { stubs: uiStubs } })
    await flushPromises()
    const vm = w.vm as any
    api.kv.exec.mockRejectedValueOnce(new Error('range'))
    vm.startEdit(0, 'a')
    vm.editValue = 'b'
    await vm.saveEdit(0)
    expect(toast.error).toHaveBeenCalledWith('range')
    api.kv.exec.mockRejectedValueOnce(new Error('push-fail'))
    vm.pushValue = 'x'
    await vm.push('front')
    expect(toast.error).toHaveBeenCalledWith('push-fail')
    api.kv.exec.mockRejectedValueOnce(new Error('pop-fail'))
    await vm.pop('front')
    expect(toast.error).toHaveBeenCalledWith('pop-fail')
    w.unmount()
  })

  it('set: 弹出失败 toast；空态与 readonly 渲染', async () => {
    mockExec((cmd) => (cmd === 'SMEMBERS' ? [] : 1))
    const w = mount(KvSetEditor, { props: base, global: { stubs: uiStubs } })
    await flushPromises()
    expect(w.text()).toContain('集合为空')
    api.kv.popSet?.mockClear?.()
    const vm = w.vm as any
    api.kv.exec.mockRejectedValueOnce(new Error('pop'))
    await vm.popRandom()
    expect(toast.error).toHaveBeenCalledWith('pop')
    api.kv.exec.mockRejectedValueOnce('raw')
    await vm.removeMember('x')
    expect(toast.error).toHaveBeenCalledWith('移除失败')
    api.kv.exec.mockRejectedValueOnce('raw2')
    vm.newMember = 'y'
    await vm.addMember()
    expect(toast.error).toHaveBeenCalledWith('添加失败')
    api.kv.exec.mockRejectedValueOnce('raw3')
    await vm.load()
    expect(toast.error).toHaveBeenCalledWith('加载失败')
    api.kv.exec.mockRejectedValueOnce('raw4')
    await vm.popRandom()
    expect(toast.error).toHaveBeenCalledWith('弹出失败')
    const ro = mount(KvSetEditor, { props: { ...base, readonly: true }, global: { stubs: uiStubs } })
    await flushPromises()
    expect(ro.text()).toContain('集合为空')
    ro.unmount()
    w.unmount()
  })

  it('zset: 改分失败 toast 与成员移除', async () => {
    mockExec((cmd) => (cmd === 'ZRANGE' ? ['a', '1'] : 1))
    const w = mount(KvZSetEditor, { props: base, global: { stubs: uiStubs } })
    await flushPromises()
    const vm = w.vm as any
    api.kv.exec.mockRejectedValueOnce(new Error('score-fail'))
    vm.startEdit({ elem: 'a', score: 1 })
    vm.editScore = '2'
    await vm.saveScore('a')
    expect(toast.error).toHaveBeenCalledWith('score-fail')
    api.kv.exec.mockRejectedValueOnce(new Error('rm'))
    await vm.removeMember('a')
    expect(toast.error).toHaveBeenCalledWith('rm')
    api.kv.exec.mockRejectedValueOnce(new Error('add'))
    vm.newElem = 'b'
    vm.newScore = '1'
    await vm.addMember()
    expect(toast.error).toHaveBeenCalledWith('add')
    w.unmount()
  })

  it('string: 加载与保存失败 toast；readonly 无操作栏', async () => {
    mockExec((cmd) => (cmd === 'GET' ? 'v' : 'OK'))
    api.kv.exec.mockRejectedValueOnce(new Error('save-fail'))
    const w = mount(KvStringEditor, { props: base, global: { stubs: uiStubs } })
    await flushPromises()
    const vm = w.vm as any
    vm.value = 'w'
    await vm.save()
    expect(toast.error).toHaveBeenCalledWith('save-fail')
    api.kv.exec.mockRejectedValueOnce(new Error('load-fail'))
    await vm.load()
    expect(toast.error).toHaveBeenCalledWith('load-fail')
    api.kv.exec.mockRejectedValueOnce(new Error('boom'))
    vm.delta = '1'
    await vm.incr()
    expect(toast.error).toHaveBeenCalledWith('boom')
    const ro = mount(KvStringEditor, { props: { ...base, readonly: true }, global: { stubs: uiStubs } })
    await flushPromises()
    expect(ro.text()).not.toContain('INCR')
    ro.unmount()
    w.unmount()
  })

  it('所有编辑器：readonly 模式加载正常', async () => {
    const opts = { props: { ...base, readonly: true }, global: { stubs: uiStubs } }
    const hs = mount(KvHashEditor, opts)
    const ls = mount(KvListEditor, opts)
    const zs = mount(KvZSetEditor, opts)
    await flushPromises()
    expect(hs.text()).toContain('暂无字段')
    expect(ls.text()).toContain('列表为空')
    expect(zs.text()).toContain('暂无成员')
    hs.unmount()
    ls.unmount()
    zs.unmount()
  })

  it('string: 非 string 响应回退空值；非 Error 拒绝走兜底文案', async () => {
    mockExec(() => null)
    const w = mount(KvStringEditor, { props: base, global: { stubs: uiStubs } })
    await flushPromises()
    expect((w.find('textarea').element as HTMLTextAreaElement).value).toBe('')
    api.kv.exec.mockRejectedValueOnce('raw')
    await (w.vm as any).load()
    expect(toast.error).toHaveBeenCalledWith('加载失败')
    api.kv.exec.mockRejectedValueOnce('raw2')
    const vm = w.vm as any
    vm.value = 'x'
    await vm.save()
    expect(toast.error).toHaveBeenCalledWith('保存失败')
    w.unmount()
  })
})

{
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
}

{
const meta = { key: 'k', type: 'string', len: null, ttl_ms: null, mtime_ms: 1, version: 1 }

function mockExec(impl: (cmd: string, argvs: string[]) => unknown) {
  api.kv.exec.mockImplementation(async (_pid: string, body: { type: string; argvs?: string[] }) => {
    if (body.type !== 'cmd') return null
    const argvs = body.argvs ?? []
    return impl(argvs[0]?.toUpperCase() ?? '', argvs)
  })
}

describe('KV 模板内联回调与 v-model', () => {
  beforeEach(() => resetApiMocks())

  it('DataTabs: 集合文档缓存回调事件透传（无 kv 页签）', async () => {
    api.db.collections.mockResolvedValue([])
    const w = mount(DataTabs, {
      props: { projectId: 'p', database: { id: 'db-1', name: 'n', status: 'ready', createdAt: '', updatedAt: '', documentCount: 0 } as any },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    expect(w.text()).toContain('集合文档')
    expect(w.text()).not.toContain('Key-Value')
    // 空态下 CollectionPanel 的 empty-action 触发 create-collection
    await w.find('.empty-action').trigger('click')
    expect(w.emitted('create-collection')).toBeTruthy()
    w.unmount()
  })

  it('CreateKeyModal: 类型切换与表单状态 v-model', async () => {
    const w = mount(KvCreateKeyModal, {
      props: { open: true, projectId: 'p' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    vm.keyName = 'k1'
    expect(vm.keyName).toBe('k1')
    // Select 类型切换到每种
    for (const t of ['hash', 'list', 'set', 'zset', 'string']) {
      vm.keyType = t
      await flushPromises()
    }
    expect(vm.keyType).toBe('string')
    // ttlMode 与秒数
    vm.ttlMode = 'custom'
    vm.ttlSeconds = '5'
    expect(vm.canSubmit).toBe(true)
    // update:open 回调
    await w.vm.$emit('update:open', false)
    w.unmount()
  })

  it('KvPanel: pattern/typeFilter v-model 与各弹窗开合状态', async () => {
    api.kv.exec.mockImplementation(async (_p, b) => (b.argvs?.[0] === 'SCAN' ? ['0', []] : null))
    api.kv.execBatch.mockImplementation(async (_p, bodies) => Promise.all(bodies.map(() => 'string')))
    const w = mount(KvPanel, { props: { projectId: 'p' }, global: { stubs: uiStubs } })
    await flushPromises()
    const vm = w.vm as any
    vm.pattern = 'x'
    vm.typeFilter = 'hash'
    expect(vm.pattern).toBe('x')
    expect(vm.typeFilter).toBe('hash')
    // 子弹窗开合状态
    vm.createOpen = true
    vm.createOpen = false
    vm.detailOpen = true
    vm.detailOpen = false
    vm.ttlOpen = true
    vm.ttlOpen = false
    vm.renameOpen = true
    vm.renameValue = 'n'
    expect(vm.renameOpen).toBe(true)
    expect(vm.createOpen).toBe(false)
    expect(vm.detailOpen).toBe(false)
    expect(vm.ttlOpen).toBe(false)
    w.unmount()
  })

  it('TtlModal: mode/seconds v-model 与 PEXPIRE 提交', async () => {
    const w = mount(KvTtlModal, {
      props: { open: true, projectId: 'p', kvKey: meta },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    vm.mode = 'custom'
    vm.seconds = '10'
    await vm.submit()
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['PEXPIRE', 'k', '10000'] })
    await w.vm.$emit('update:open', false)
    w.unmount()
  })

  it('ListEditor: 行内编辑与操作按钮回调', async () => {
    mockExec((cmd) => (cmd === 'LRANGE' ? ['a', 'b'] : 'OK'))
    const w = mount(KvListEditor, {
      props: { projectId: 'p', kvKey: 'k' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    vm.startEdit(1, 'b')
    vm.editValue = 'z'
    await vm.saveEdit(1)
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['LSET', 'k', '1', 'z'] })
    mockExec((cmd) => (cmd === 'LRANGE' ? ['a'] : 'OK'))
    vm.pushValue = 'n'
    await vm.push('front')
    vm.pushValue = 'n2'
    await vm.push('back')
    await vm.pop('front')
    await vm.pop('back')
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['LPOP', 'k'] })
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['RPOP', 'k'] })
    w.unmount()
  })

  it('SetEditor: 移除按钮回调与 newMember v-model', async () => {
    mockExec((cmd) => (cmd === 'SMEMBERS' ? ['x', 'y'] : 1))
    const w = mount(KvSetEditor, {
      props: { projectId: 'p', kvKey: 'k' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    vm.newMember = 'z'
    expect(vm.newMember).toBe('z')
    mockExec((cmd) => (cmd === 'SMEMBERS' ? ['y'] : 1))
    await vm.removeMember('x')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['SREM', 'k', 'x'] })
    w.unmount()
  })

  it('StringEditor: 值 v-model 与类型化保存（keep_ttl）', async () => {
    mockExec((cmd) => (cmd === 'GET' ? 'v' : 'OK'))
    const w = mount(KvStringEditor, {
      props: { projectId: 'p', kvKey: 'k' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    vm.value = 'changed'
    expect(vm.value).toBe('changed')
    await vm.save()
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'String',
      args: { key: 'k', value: 'changed', keep_ttl: true }
    })
    w.unmount()
  })

  it('DetailSheet: 编辑器 changed 回调联动 meta 刷新', async () => {
    api.kv.execBatch.mockImplementation(async (_pid: string, bodies: { argvs?: string[] }[]) =>
      Promise.all(
        bodies.map((b) => {
          const cmd = b.argvs?.[0]?.toUpperCase()
          if (cmd === 'TYPE') return 'string'
          if (cmd === 'PTTL') return -1
          return 0
        })
      )
    )
    const w = mount(KvDetailSheet, {
      props: { open: true, projectId: 'p', kvKey: meta },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    vm.onChanged()
    await flushPromises()
    expect(api.kv.execBatch).toHaveBeenCalledTimes(2)
    w.unmount()
  })

  it('HashEditor / ZSetEditor: 添加回调链', async () => {
    mockExec((cmd) => (cmd === 'HGETALL' ? ['f', 'v'] : 1))
    const h = mount(KvHashEditor, {
      props: { projectId: 'p', kvKey: 'k' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const hvm = h.vm as any
    hvm.newField = 'g'
    hvm.newValue = 'w'
    await hvm.addField()
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'Hash',
      args: { key: 'k', fields: { g: 'w' } }
    })
    h.unmount()

    mockExec((cmd) => (cmd === 'ZRANGE' ? ['e', '1'] : 1))
    const z = mount(KvZSetEditor, {
      props: { projectId: 'p', kvKey: 'k' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const zvm = z.vm as any
    zvm.newElem = 'm'
    zvm.newScore = '3'
    await zvm.addMember()
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'ZSet',
      args: { key: 'k', items: [{ elem: 'm', score: 3 }] }
    })
    z.unmount()
  })
})
}

{
const meta = { key: 'k', type: 'string', len: null, ttl_ms: null, mtime_ms: 1, version: 1 }

// 透传 v-model 与 keydown 的 stub（原生 keydown 冒泡 + v-model）
const keydownStubs = {
  ...uiStubs,
  Input: {
    props: ['modelValue', 'id', 'type', 'placeholder', 'disabled'],
    emits: ['update:modelValue'],
    inheritAttrs: false,
    template:
      '<input :id="id" :type="type || \'text\'" :value="modelValue" :placeholder="placeholder" :disabled="disabled" @input="$emit(\'update:modelValue\', $event.target.value)" @keydown.enter="$emit(\'keydown\', $event)" />'
  },
  Textarea: {
    props: ['modelValue', 'id', 'rows'],
    emits: ['update:modelValue'],
    template:
      '<textarea :id="id" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" @keydown.enter="$emit(\'keydown\', $event)"></textarea>'
  }
}

function findBtn(w: ReturnType<typeof mount>, text: string) {
  return w.findAll('button').find((b) => b.text().includes(text))
}

function mockExec(impl: (cmd: string, argvs: string[]) => unknown) {
  api.kv.exec.mockImplementation(async (_pid: string, body: { type: string; argvs?: string[] }) => {
    if (body.type !== 'cmd') return null
    const argvs = body.argvs ?? []
    return impl(argvs[0]?.toUpperCase() ?? '', argvs)
  })
}

describe('KV 模板 keydown/事件回调', () => {
  beforeEach(() => resetApiMocks())

  it('ListEditor: 行内编辑回车保存（LSET）', async () => {
    mockExec((cmd) => (cmd === 'LRANGE' ? ['a', 'b'] : 'OK'))
    const w = mount(KvListEditor, {
      props: { projectId: 'p', kvKey: 'k' },
      global: { stubs: keydownStubs }
    })
    await flushPromises()
    await findBtn(w, '改')!.trigger('click')
    await flushPromises()
    const inline = w.findAll('input')[0]
    await inline.setValue('z')
    await inline.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['LSET', 'k', '0', 'z'] })
    w.unmount()
  })

  it('ListEditor: 新元素回车尾插', async () => {
    mockExec((cmd) => (cmd === 'LRANGE' ? ['a'] : 2))
    const w = mount(KvListEditor, {
      props: { projectId: 'p', kvKey: 'k' },
      global: { stubs: keydownStubs }
    })
    await flushPromises()
    const input = w.find('input[placeholder="新元素"]')
    await input.setValue('n')
    await input.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'List',
      args: { key: 'k', elems: ['n'], side: 'back' }
    })
    w.unmount()
  })

  it('SetEditor: 新成员回车添加；× 按钮移除（SREM）', async () => {
    mockExec((cmd) => (cmd === 'SMEMBERS' ? ['x'] : 1))
    const w = mount(KvSetEditor, {
      props: { projectId: 'p', kvKey: 'k' },
      global: { stubs: keydownStubs }
    })
    await flushPromises()
    const input = w.find('input[placeholder="新成员"]')
    await input.setValue('y')
    await input.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'Set', args: { key: 'k', elems: ['y'] } })
    mockExec((cmd) => (cmd === 'SMEMBERS' ? [] : 1))
    await w.find('button[title="移除"]').trigger('click')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['SREM', 'k', 'x'] })
    w.unmount()
  })

  it('StringEditor: 保存按钮与 INCR 按钮', async () => {
    mockExec((cmd) => (cmd === 'GET' ? '1' : 4))
    const w = mount(KvStringEditor, {
      props: { projectId: 'p', kvKey: 'k' },
      global: { stubs: keydownStubs }
    })
    await flushPromises()
    await w.find('textarea').setValue('2')
    await findBtn(w, '保存')!.trigger('click')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'String',
      args: { key: 'k', value: '2', keep_ttl: true }
    })
    await w.find('input[placeholder="步进"]').setValue('3')
    await findBtn(w, 'INCR')!.trigger('click')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['INCRBY', 'k', '3'] })
    w.unmount()
  })

  it('CreateKeyModal: 各类型表单输入与提交（类型化写入）', async () => {
    const w = mount(KvCreateKeyModal, {
      props: { open: true, projectId: 'p' },
      global: { stubs: keydownStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    // string 值 textarea
    await w.find('#kv-create-string').setValue('sv')
    vm.keyName = 's1'
    await vm.submit()
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'String',
      args: { key: 's1', value: 'sv' }
    })

    // list/set textarea
    vm.keyType = 'list'
    await flushPromises()
    await w.find('#kv-create-list').setValue('a\nb')
    vm.keyName = 'l1'
    await vm.submit()
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'List',
      args: { key: 'l1', elems: ['a', 'b'], side: 'back' }
    })

    vm.keyType = 'set'
    await flushPromises()
    await w.find('#kv-create-set').setValue('m\nn')
    vm.keyName = 't1'
    await vm.submit()
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'Set',
      args: { key: 't1', elems: ['m', 'n'] }
    })
    w.unmount()
  })

  it('TtlModal: 秒数输入与 SbModal ok 提交（PEXPIRE 毫秒）', async () => {
    const w = mount(KvTtlModal, {
      props: { open: true, projectId: 'p', kvKey: meta },
      global: { stubs: keydownStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    vm.mode = 'custom'
    await flushPromises()
    await w.find('input[type="number"]').setValue('45')
    await w.find('.sb-ok').trigger('click')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['PEXPIRE', 'k', '45000'] })
    w.unmount()
  })

  it('KvPanel: 重命名输入框回车提交（RENAME）', async () => {
    api.kv.exec.mockImplementation(async (_p, b) =>
      b.argvs?.[0] === 'SCAN' ? ['0', []] : 'OK'
    )
    api.kv.execBatch.mockImplementation(async (_p, bodies) => Promise.all(bodies.map(() => 'string')))
    const w = mount(KvPanel, {
      props: { projectId: 'p', readonly: false },
      global: { stubs: keydownStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    vm.openRename(meta)
    await flushPromises()
    await w.find('#kv-rename').setValue('nk')
    api.kv.exec.mockImplementation(async (_p, b) =>
      b.argvs?.[0] === 'SCAN' ? ['0', []] : 'OK'
    )
    await w.find('#kv-rename').trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['RENAME', 'k', 'nk'] })
    w.unmount()
  })
})
}
