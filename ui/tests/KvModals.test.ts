import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import KvCreateKeyModal from '@/components/databases/kv/KvCreateKeyModal.vue'
import KvTtlModal from '@/components/databases/kv/KvTtlModal.vue'
import KvDetailSheet from '@/components/databases/kv/KvDetailSheet.vue'

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
