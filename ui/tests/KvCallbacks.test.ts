import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import DataTabs from '@/components/databases/DataTabs.vue'
import KvCreateKeyModal from '@/components/databases/kv/KvCreateKeyModal.vue'
import KvDetailSheet from '@/components/databases/kv/KvDetailSheet.vue'
import KvPanel from '@/components/databases/kv/KvPanel.vue'
import KvTtlModal from '@/components/databases/kv/KvTtlModal.vue'
import KvListEditor from '@/components/databases/kv/editors/KvListEditor.vue'
import KvSetEditor from '@/components/databases/kv/editors/KvSetEditor.vue'
import KvStringEditor from '@/components/databases/kv/editors/KvStringEditor.vue'
import KvHashEditor from '@/components/databases/kv/editors/KvHashEditor.vue'
import KvZSetEditor from '@/components/databases/kv/editors/KvZSetEditor.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api, isMock: false }
})

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
