import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import KvCreateKeyModal from '@/components/databases/kv/KvCreateKeyModal.vue'
import KvTtlModal from '@/components/databases/kv/KvTtlModal.vue'
import KvDetailSheet from '@/components/databases/kv/KvDetailSheet.vue'

import { toast } from 'vue-sonner'
import KvHashEditor from '@/components/databases/kv/editors/KvHashEditor.vue'
import KvListEditor from '@/components/databases/kv/editors/KvListEditor.vue'
import KvSetEditor from '@/components/databases/kv/editors/KvSetEditor.vue'
import KvZSetEditor from '@/components/databases/kv/editors/KvZSetEditor.vue'
vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api, isMock: false }
})

const meta = { key: 'k', type: 'string', len: null, ttl_ms: null, mtime_ms: 1, version: 1 }

describe('KvCreateKeyModal (新建 Key)', () => {
  beforeEach(() => resetApiMocks())

  async function openWith() {
    const w = mount(KvCreateKeyModal, {
      props: { open: true, projectId: 'p' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    return w
  }

  it('打开时重置表单；空 key 不能提交', async () => {
    const w = await openWith()
    const vm = w.vm as any
    expect(vm.keyName).toBe('')
    expect(vm.keyType).toBe('string')
    expect(vm.canSubmit).toBe(false)
    w.unmount()
  })

  it('string 创建走类型化 String，带 TTL', async () => {
    const w = await openWith()
    const vm = w.vm as any
    vm.keyName = 's1'
    vm.stringValue = 'hello'
    vm.ttlMode = 'custom'
    vm.ttlSeconds = '30'
    expect(vm.canSubmit).toBe(true)
    await vm.submit()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'String',
      args: { key: 's1', value: 'hello', ttl_ms: 30000 }
    })
    expect(w.emitted('created')?.[0]).toEqual(['s1'])
    expect(w.emitted('update:open')?.at(-1)).toEqual([false])
    w.unmount()
  })

  it('string 无 TTL 时不带 ttl_ms 字段', async () => {
    const w = await openWith()
    const vm = w.vm as any
    vm.keyName = 's2'
    vm.stringValue = 'v'
    await vm.submit()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'String',
      args: { key: 's2', value: 'v' }
    })
    w.unmount()
  })

  it('hash/list/set/zset 分别走对应类型化写入；TTL 统一走 ttl_ms', async () => {
    const w = await openWith()
    const vm = w.vm as any
    vm.keyName = 'h1'
    vm.keyType = 'hash'
    vm.hashPairs = [{ field: 'a', value: '1' }, { field: '', value: '' }]
    vm.ttlMode = 'custom'
    vm.ttlSeconds = '10'
    await vm.submit()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'Hash',
      args: { key: 'h1', fields: { a: '1' }, ttl_ms: 10000 }
    })

    api.kv.exec.mockClear()
    vm.keyName = 'l1'
    vm.keyType = 'list'
    vm.listText = 'x\n y \n'
    await vm.submit()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'List',
      args: { key: 'l1', elems: ['x', 'y'], side: 'back', ttl_ms: 10000 }
    })

    api.kv.exec.mockClear()
    vm.keyName = 't1'
    vm.keyType = 'set'
    vm.setText = 'a\nb'
    vm.ttlMode = 'none'
    await vm.submit()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'Set',
      args: { key: 't1', elems: ['a', 'b'] }
    })

    api.kv.exec.mockClear()
    vm.keyName = 'z1'
    vm.keyType = 'zset'
    vm.zsetPairs = [{ elem: 'e', score: '2.5' }]
    await vm.submit()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'ZSet',
      args: { key: 'z1', items: [{ elem: 'e', score: 2.5 }] }
    })
    w.unmount()
  })

  it('校验：类型必填项缺失时不能提交', async () => {
    const w = await openWith()
    const vm = w.vm as any
    vm.keyName = 'x'
    vm.keyType = 'hash'
    vm.hashPairs = [{ field: '', value: '' }]
    expect(vm.canSubmit).toBe(false)
    vm.keyType = 'list'
    expect(vm.canSubmit).toBe(false)
    vm.keyType = 'zset'
    vm.zsetPairs = [{ elem: 'e', score: '' }]
    expect(vm.canSubmit).toBe(false)
    vm.keyType = 'string'
    vm.ttlMode = 'custom'
    vm.ttlSeconds = '-1'
    expect(vm.canSubmit).toBe(false)
    w.unmount()
  })

  it('提交失败 toast 报错且不关闭', async () => {
    api.kv.exec.mockRejectedValueOnce(new Error('dup'))
    const w = await openWith()
    const vm = w.vm as any
    vm.keyName = 's'
    await vm.submit()
    const { toast } = await import('vue-sonner')
    expect(toast.error).toHaveBeenCalledWith('dup')
    expect(w.emitted('update:open')).toBeFalsy()
    w.unmount()
  })

  it('open 变化重置表单；非 Error 拒绝走兜底文案', async () => {
    const w = await openWith()
    const vm = w.vm as any
    vm.keyName = 'dirty'
    vm.keyType = 'zset'
    await w.setProps({ open: false })
    await w.setProps({ open: true })
    expect(vm.keyName).toBe('')
    expect(vm.keyType).toBe('string')
    api.kv.exec.mockRejectedValueOnce('raw')
    vm.keyName = 's2'
    await vm.submit()
    const { toast } = await import('vue-sonner')
    expect(toast.error).toHaveBeenCalledWith('创建失败')
    w.unmount()
  })
})

describe('KvTtlModal (TTL 设置)', () => {
  beforeEach(() => resetApiMocks())

  it('无 TTL 默认永久；走 PERSIST', async () => {
    const w = mount(KvTtlModal, {
      props: { open: true, projectId: 'p', kvKey: meta },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    expect(vm.mode).toBe('none')
    await vm.submit()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['PERSIST', 'k'] })

    vm.mode = 'custom'
    vm.seconds = '60'
    await vm.submit()
    // 自定义秒数走 PEXPIRE（毫秒）
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['PEXPIRE', 'k', '60000'] })
    expect(w.emitted('changed')).toBeTruthy()
    w.unmount()
  })

  it('有 TTL 时预填秒数', async () => {
    const w = mount(KvTtlModal, {
      props: { open: true, projectId: 'p', kvKey: { ...meta, ttl_ms: 30000 } },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    expect(vm.mode).toBe('custom')
    expect(vm.seconds).toBe('30')
    expect(w.text()).toContain('当前剩余约 30 秒')
    w.unmount()
  })

  it('kvKey 为 null 时提交直接返回；watch 不预填', async () => {
    const w = mount(KvTtlModal, {
      props: { open: true, projectId: 'p', kvKey: null },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    await vm.submit()
    expect(api.kv.exec).not.toHaveBeenCalled()
    expect(vm.mode).toBe('none')
    w.unmount()
  })

  it('失败 toast 报错', async () => {
    api.kv.exec.mockRejectedValueOnce(new Error('boom'))
    const w = mount(KvTtlModal, {
      props: { open: true, projectId: 'p', kvKey: meta },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    await vm.submit()
    const { toast } = await import('vue-sonner')
    expect(toast.error).toHaveBeenCalledWith('boom')
    w.unmount()
  })
})

describe('KvDetailSheet (详情抽屉)', () => {
  beforeEach(() => resetApiMocks())

  /** DetailSheet 用 execBatch 取 TYPE/PTTL/长度拼 meta */
  function mockMeta(type: string, pttl = -1, len = 3) {
    api.kv.execBatch.mockImplementation(async (_pid: string, bodies: { argvs?: string[] }[]) =>
      Promise.all(
        bodies.map((b) => {
          const cmd = b.argvs?.[0]?.toUpperCase()
          if (cmd === 'TYPE') return type
          if (cmd === 'PTTL') return pttl
          return len
        })
      )
    )
  }

  it('按类型选择编辑器并展示 meta', async () => {
    mockMeta('hash')
    const w = mount(KvDetailSheet, {
      props: { open: true, projectId: 'p', kvKey: { ...meta, key: 'h', type: 'hash' } },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    expect(w.text()).toContain('hash')
    expect(w.text()).toContain('3 个元素')
    expect(w.findComponent({ name: 'KvHashEditor' }).exists()).toBe(true)
    w.unmount()
  })

  it('各类型渲染对应编辑器', async () => {
    const editors = {
      string: 'KvStringEditor',
      hash: 'KvHashEditor',
      list: 'KvListEditor',
      set: 'KvSetEditor',
      zset: 'KvZSetEditor'
    } as const
    for (const [t, name] of Object.entries(editors)) {
      mockMeta(t)
      const w = mount(KvDetailSheet, {
        props: { open: true, projectId: 'p', kvKey: { ...meta, type: t as never } },
        global: { stubs: uiStubs }
      })
      await flushPromises()
      expect(w.findComponent({ name }).exists()).toBe(true)
      w.unmount()
    }
  })

  it('删除 key（DEL）后 emit deleted 并关闭', async () => {
    mockMeta('string')
    const w = mount(KvDetailSheet, {
      props: { open: true, projectId: 'p', kvKey: meta },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    await vm.removeKey()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['DEL', 'k'] })
    expect(w.emitted('deleted')).toBeTruthy()
    expect(w.emitted('update:open')?.at(-1)).toEqual([false])
    w.unmount()
  })

  it('meta 加载失败回退传入 kvKey', async () => {
    api.kv.execBatch.mockRejectedValueOnce(new Error('x'))
    const w = mount(KvDetailSheet, {
      props: { open: true, projectId: 'p', kvKey: meta },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    expect(vm.meta).toEqual(meta)
    w.unmount()
  })

  it('未选 key 显示占位；readonly 无删除按钮', async () => {
    const w0 = mount(KvDetailSheet, {
      props: { open: true, projectId: 'p', kvKey: null },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    expect(w0.text()).toContain('未选择 Key')
    w0.unmount()

    mockMeta('string')
    const w1 = mount(KvDetailSheet, {
      props: { open: true, projectId: 'p', kvKey: meta, readonly: true },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    expect(w1.find('.confirm-action').exists()).toBe(false)
    w1.unmount()
  })
})

{
const meta = { key: 'k', type: 'string', len: null, ttl_ms: null, mtime_ms: 1, version: 1 }

// v-model 完整 stub：setValue → update:modelValue 生效
const vmStubs = {
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

describe('KV 表单 v-model 双向绑定补盲', () => {
  beforeEach(() => resetApiMocks())

  it('CreateKeyModal: hash 行输入/增删行；zset 同构', async () => {
    const w = mount(KvCreateKeyModal, {
      props: { open: true, projectId: 'p' },
      global: { stubs: vmStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    vm.keyType = 'hash'
    await flushPromises()
    const hashInputs = w.findAll('input').filter((i) => i.attributes('placeholder') === '字段名' || i.attributes('placeholder') === '值')
    await hashInputs[0].setValue('f1')
    await hashInputs[1].setValue('v1')
    expect(vm.hashPairs[0].field).toBe('f1')
    expect(vm.hashPairs[0].value).toBe('v1')
    // 增行 → 删行按钮（splice 内联回调）
    await w.findAll('button').find((b) => b.text().includes('+ 添加字段'))!.trigger('click')
    expect(vm.hashPairs).toHaveLength(2)
    await w.findAll('button').find((b) => b.text() === '删')!.trigger('click')
    expect(vm.hashPairs).toHaveLength(1)

    vm.keyType = 'zset'
    await flushPromises()
    const zInputs = w.findAll('input').filter((i) => i.attributes('placeholder') === '成员' || i.attributes('placeholder') === '分数')
    await zInputs[0].setValue('e1')
    await zInputs[1].setValue('2.5')
    expect(vm.zsetPairs[0].elem).toBe('e1')
    expect(vm.zsetPairs[0].score).toBe('2.5')
    await w.findAll('button').find((b) => b.text().includes('+ 添加成员'))!.trigger('click')
    expect(vm.zsetPairs).toHaveLength(2)
    await w.findAll('button').find((b) => b.text() === '删')!.trigger('click')
    expect(vm.zsetPairs).toHaveLength(1)
    w.unmount()
  })

  it('CreateKeyModal: string 值与 ttl 秒数输入；SbModal ok（带 ttl_ms）', async () => {
    const w = mount(KvCreateKeyModal, {
      props: { open: true, projectId: 'p' },
      global: { stubs: vmStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    await w.find('#kv-create-key').setValue('kk')
    await w.find('#kv-create-string').setValue('vv')
    vm.ttlMode = 'custom'
    await flushPromises()
    await w.find('input[type="number"]').setValue('7')
    expect(vm.ttlSeconds).toBe('7')
    await w.find('.sb-ok').trigger('click')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'String',
      args: { key: 'kk', value: 'vv', ttl_ms: 7000 }
    })
    expect(w.emitted('created')?.[0]).toEqual(['kk'])
    w.unmount()
  })

  it('TtlModal: update:open 回调与取消按钮', async () => {
    const w = mount(KvTtlModal, {
      props: { open: true, projectId: 'p', kvKey: meta },
      global: { stubs: vmStubs }
    })
    await flushPromises()
    await w.find('.sb-cancel').trigger('click')
    expect(w.emitted('update:open')?.at(-1)).toEqual([false])
    w.unmount()
  })
})
}

{
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
}
