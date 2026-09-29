import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import KvCreateKeyModal from '@/components/databases/kv/KvCreateKeyModal.vue'
import KvTtlModal from '@/components/databases/kv/KvTtlModal.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api, isMock: false }
})

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
