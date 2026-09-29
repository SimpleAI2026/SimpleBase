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

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api, isMock: false }
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
