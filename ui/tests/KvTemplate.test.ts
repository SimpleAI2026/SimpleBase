import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import KvCreateKeyModal from '@/components/databases/kv/KvCreateKeyModal.vue'
import KvDetailSheet from '@/components/databases/kv/KvDetailSheet.vue'
import KvTtlModal from '@/components/databases/kv/KvTtlModal.vue'
import KvHashEditor from '@/components/databases/kv/editors/KvHashEditor.vue'
import KvListEditor from '@/components/databases/kv/editors/KvListEditor.vue'
import KvSetEditor from '@/components/databases/kv/editors/KvSetEditor.vue'
import KvZSetEditor from '@/components/databases/kv/editors/KvZSetEditor.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api, isMock: false }
})

const meta = { key: 'k', type: 'string', len: null, ttl_ms: null, mtime_ms: 1, version: 1 }

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

describe('KV 模板交互（编辑器/弹窗按钮与内联回调）', () => {
  beforeEach(() => resetApiMocks())

  it('CreateKeyModal: hash/zset 行增删与类型切换', async () => {
    const w = mount(KvCreateKeyModal, {
      props: { open: true, projectId: 'p' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    vm.keyType = 'hash'
    await flushPromises()
    await findBtn(w, '+ 添加字段')!.trigger('click')
    expect(vm.hashPairs).toHaveLength(2)
    await findBtn(w, '删')!.trigger('click')
    expect(vm.hashPairs).toHaveLength(1)

    vm.keyType = 'zset'
    await flushPromises()
    await findBtn(w, '+ 添加成员')!.trigger('click')
    expect(vm.zsetPairs).toHaveLength(2)
    await findBtn(w, '删')!.trigger('click')
    expect(vm.zsetPairs).toHaveLength(1)

    // SbModal ok 触发 submit
    vm.keyName = 'k1'
    mockExec(() => 1)
    await vm.submit()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'ZSet',
      args: { key: 'k1', items: [] }
    })
    w.unmount()
  })

  it('CreateKeyModal: 选项渲染完整（Select 每个类型）', async () => {
    const w = mount(KvCreateKeyModal, {
      props: { open: true, projectId: 'p' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    for (const t of ['String', 'Hash', 'List', 'Set', 'ZSet']) {
      expect(w.text()).toContain(t)
    }
    expect(w.text()).toContain('永久')
    w.unmount()
  })

  it('TtlModal: 自定义切换与 SbModal ok 提交（PEXPIRE）', async () => {
    const w = mount(KvTtlModal, {
      props: { open: true, projectId: 'p', kvKey: { ...meta, ttl_ms: 25000 } },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    expect(w.text()).toContain('当前剩余约 25 秒')
    mockExec(() => 1)
    await w.find('.sb-ok').trigger('click')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['PEXPIRE', 'k', '25000'] })
    w.unmount()
  })

  it('TtlModal: 永久分支提交 PERSIST；取消关闭', async () => {
    const w = mount(KvTtlModal, {
      props: { open: true, projectId: 'p', kvKey: meta },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    expect(vm.mode).toBe('none')
    mockExec(() => 1)
    await vm.submit()
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['PERSIST', 'k'] })
    await w.find('.sb-cancel').trigger('click')
    expect(w.emitted('update:open')?.at(-1)).toEqual([false])
    w.unmount()
  })

  it('DetailSheet: 编辑器 changed 后 meta 刷新；Sheet close 透传', async () => {
    api.kv.execBatch.mockImplementation(async (_pid: string, bodies: { argvs?: string[] }[]) =>
      Promise.all(
        bodies.map((b) => {
          const cmd = b.argvs?.[0]?.toUpperCase()
          if (cmd === 'TYPE') return 'string'
          if (cmd === 'PTTL') return -1
          return 5
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
    expect(w.emitted('changed')).toBeTruthy()
    // Sheet close 事件透传
    await w.find('.sheet-close').trigger('click')
    expect(w.emitted('update:open')?.at(-1)).toEqual([false])
    w.unmount()
  })

  it('HashEditor: 行内改/存/删按钮触发', async () => {
    mockExec((cmd) => (cmd === 'HGETALL' ? ['f', 'v'] : 1))
    const w = mount(KvHashEditor, {
      props: { projectId: 'p', kvKey: 'k' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    await findBtn(w, '改')!.trigger('click')
    expect(vm.editField).toBe('f')
    vm.editValue = 'v2'
    await findBtn(w, '存')!.trigger('click')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'Hash',
      args: { key: 'k', fields: { f: 'v2' } }
    })
    mockExec((cmd) => (cmd === 'HGETALL' ? [] : 1))
    await w.findAll('.confirm-action').find((c) => c.text().includes('删'))!.trigger('click')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['HDEL', 'k', 'f'] })
    w.unmount()
  })

  it('HashEditor: 添加行按钮与空字段过滤', async () => {
    mockExec((cmd) => (cmd === 'HGETALL' ? [] : 1))
    const w = mount(KvHashEditor, {
      props: { projectId: 'p', kvKey: 'k' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const inputs = w.findAll('input')
    await inputs[0].setValue('a')
    await inputs[1].setValue('b')
    await findBtn(w, '添加')!.trigger('click')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'Hash',
      args: { key: 'k', fields: { a: 'b' } }
    })
    w.unmount()
  })

  it('ListEditor: 头插/尾插/弹头/弹尾/行内编辑按钮', async () => {
    mockExec((cmd) => {
      if (cmd === 'LRANGE') return ['x']
      if (cmd === 'LPOP' || cmd === 'RPOP') return 'x'
      return 'OK'
    })
    const w = mount(KvListEditor, {
      props: { projectId: 'p', kvKey: 'k' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    vm.pushValue = 'a'
    await vm.push('front')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'List',
      args: { key: 'k', elems: ['a'], side: 'front' }
    })
    vm.pushValue = 'b'
    await vm.push('back')
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'List',
      args: { key: 'k', elems: ['b'], side: 'back' }
    })
    mockExec((cmd) => (cmd === 'LRANGE' ? ['x'] : 'OK'))
    await vm.pop('front')
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['LPOP', 'k'] })
    await vm.pop('back')
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['RPOP', 'k'] })
    vm.startEdit(0, 'x')
    expect(vm.editIndex).toBe(0)
    vm.editValue = 'y'
    await vm.saveEdit(0)
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['LSET', 'k', '0', 'y'] })
    w.unmount()
  })

  it('ZSetEditor: 排序切换按钮与行内改分', async () => {
    mockExec((cmd) => (cmd === 'ZRANGE' ? ['e', '5'] : 1))
    const w = mount(KvZSetEditor, {
      props: { projectId: 'p', kvKey: 'k' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    await findBtn(w, '分数升序')!.trigger('click')
    await flushPromises()
    expect(vm.desc).toBe(false)
    mockExec((cmd) => (cmd === 'ZRANGE' ? ['e', '5'] : 1))
    await findBtn(w, '分数降序')!.trigger('click')
    await flushPromises()
    expect(vm.desc).toBe(true)
    await findBtn(w, '改分')!.trigger('click')
    expect(vm.editElem).toBe('e')
    vm.editScore = '6'
    await findBtn(w, '存')!.trigger('click')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'ZSet',
      args: { key: 'k', items: [{ elem: 'e', score: 6 }] }
    })
    mockExec((cmd) => (cmd === 'ZRANGE' ? [] : 1))
    await w.findAll('.confirm-action').find((c) => c.text().includes('删'))!.trigger('click')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['ZREM', 'k', 'e'] })
    w.unmount()
  })

  it('ZSetEditor: 添加成员按钮', async () => {
    mockExec((cmd) => (cmd === 'ZRANGE' ? [] : 1))
    const w = mount(KvZSetEditor, {
      props: { projectId: 'p', kvKey: 'k' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const inputs = w.findAll('input')
    await inputs[0].setValue('m')
    await inputs[1].setValue('2')
    await findBtn(w, '添加')!.trigger('click')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'ZSet',
      args: { key: 'k', items: [{ elem: 'm', score: 2 }] }
    })
    w.unmount()
  })

  it('SetEditor: 随机弹出成功 toast（SPOP）', async () => {
    mockExec((cmd) => (cmd === 'SMEMBERS' ? ['a', 'b'] : 'a'))
    const w = mount(KvSetEditor, {
      props: { projectId: 'p', kvKey: 'k' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    mockExec((cmd) => (cmd === 'SMEMBERS' ? ['b'] : 'a'))
    await vm.popRandom()
    await flushPromises()
    expect(toast.success).toHaveBeenCalledWith('已弹出: a')
    w.unmount()
  })

  it('ListEditor: 弹头/弹尾按钮回调（模板触发）', async () => {
    mockExec((cmd) => {
      if (cmd === 'LRANGE') return ['a', 'b', 'c']
      return 'a'
    })
    const w = mount(KvListEditor, {
      props: { projectId: 'p', kvKey: 'k' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    mockExec((cmd) => {
      if (cmd === 'LRANGE') return ['b', 'c']
      return 'a'
    })
    await findBtn(w, '弹头')!.trigger('click')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['LPOP', 'k'] })
    mockExec((cmd) => {
      if (cmd === 'LRANGE') return ['b']
      return 'c'
    })
    await findBtn(w, '弹尾')!.trigger('click')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['RPOP', 'k'] })
    w.unmount()
  })

  it('DetailSheet: 删除按钮与 SheetFooter ConfirmAction', async () => {
    api.kv.execBatch.mockImplementation(async (_pid: string, bodies: { argvs?: string[] }[]) =>
      Promise.all(bodies.map((b) => (b.argvs?.[0]?.toUpperCase() === 'TYPE' ? 'string' : -1)))
    )
    const w = mount(KvDetailSheet, {
      props: { open: true, projectId: 'p', kvKey: meta },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    api.kv.exec.mockResolvedValueOnce(1)
    await w.find('.confirm-action').trigger('click')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['DEL', 'k'] })
    expect(w.emitted('deleted')).toBeTruthy()
    w.unmount()
  })
})
