import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import KvCreateKeyModal from '@/components/databases/kv/KvCreateKeyModal.vue'
import KvTtlModal from '@/components/databases/kv/KvTtlModal.vue'
import KvPanel from '@/components/databases/kv/KvPanel.vue'
import KvListEditor from '@/components/databases/kv/editors/KvListEditor.vue'
import KvSetEditor from '@/components/databases/kv/editors/KvSetEditor.vue'
import KvStringEditor from '@/components/databases/kv/editors/KvStringEditor.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api, isMock: false }
})

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
