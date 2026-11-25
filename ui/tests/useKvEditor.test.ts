// KV 编辑器公共骨架：按键自动加载、写命令固定的「失败提示 / 成功处理 / 重新读取 / 通知父组件」顺序。
import { flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick, ref, unref } from 'vue'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { useKvEditor } from '@/composables/useKvEditor'
import type { KvExecBody } from '@/services/types'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

function setup(initialKey = 'k1') {
  const calls: string[] = []
  const kvKey = ref(initialKey)
  const applied: string[] = []
  const exec = api.kv.exec.mockImplementation(async (_p: string, body: KvExecBody) => {
    calls.push(body.type === 'cmd' ? `exec:${body.argvs.join(' ')}` : `exec:${body.type}`)
    return 'raw'
  })
  const editor = useKvEditor({
    projectId: () => 'p1',
    kvKey: () => kvKey.value,
    fallback: 'components.kvHash.loadFailed',
    read: () => ({ type: 'cmd', argvs: ['GET', kvKey.value] }),
    parse: (r) => `parsed:${String(r)}`,
    apply: (v) => {
      applied.push(v)
    },
    changed: () => calls.push('changed')
  })
  return { editor, calls, kvKey, applied, exec }
}

describe('useKvEditor', () => {
  beforeEach(() => {
    resetApiMocks()
    vi.spyOn(toast, 'success').mockImplementation(() => 'ok')
    vi.spyOn(toast, 'error').mockImplementation(() => 'ok')
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('setup 阶段按当前 key 读取一次并写回；key 变化时重新读取', async () => {
    const { editor, calls, kvKey, applied } = setup()
    // setup 同步触发首次读取（pending 为真），结束后写回。
    expect(unref(editor.state.pending)).toBe(true)
    expect(editor.state.showEmpty).toBe(false)
    expect(calls).toEqual(['exec:GET k1'])
    await flushPromises()
    expect(unref(editor.state.pending)).toBe(false)
    expect(applied).toEqual(['parsed:raw'])

    kvKey.value = 'k2'
    await nextTick()
    await flushPromises()
    expect(calls).toEqual(['exec:GET k1', 'exec:GET k2'])
    expect(applied).toEqual(['parsed:raw', 'parsed:raw'])
  })

  it('读命令失败：只按 fallback 文案提示，保留已有数据、不写回', async () => {
    const { editor, applied } = setup()
    await flushPromises()
    api.kv.exec.mockRejectedValueOnce(new Error('boom'))
    await editor.load()
    expect(toast.error).toHaveBeenCalledTimes(1)
    // 失败的那次不写回：仍只有首次加载的结果。
    expect(applied).toEqual(['parsed:raw'])
    expect(unref(editor.state.refreshing)).toBe(false)
  })

  it('mutate 成功：成功提示 → onSuccess → 重新读取 → 通知父组件', async () => {
    const { editor, calls } = setup()
    await flushPromises()
    calls.length = 0
    const onSuccess = vi.fn(() => calls.push('onSuccess'))
    await editor.mutate({ type: 'Hash', args: { key: 'k1', fields: { a: '1' } } }, {
      failKey: 'components.kvHash.saveFailed',
      successKey: 'components.kvHash.saved',
      onSuccess
    })
    expect(calls).toEqual(['exec:Hash', 'onSuccess', 'exec:GET k1', 'changed'])
    expect(toast.success).toHaveBeenCalledTimes(1)
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('mutate 失败：按 failKey 提示，不触发 onSuccess / 重新读取 / 通知', async () => {
    const { editor, calls } = setup()
    await flushPromises()
    calls.length = 0
    api.kv.exec.mockRejectedValueOnce('raw')
    const onSuccess = vi.fn()
    await editor.mutate({ type: 'cmd', argvs: ['DEL', 'k1'] }, { failKey: 'components.kvHash.deleteFailed', successKey: 'x', onSuccess })
    expect(calls).toEqual([])
    expect(onSuccess).not.toHaveBeenCalled()
    expect(toast.success).not.toHaveBeenCalled()
    expect(toast.error).toHaveBeenCalledTimes(1)
  })

  it('reload=false：用返回值原地更新，不重新读取但仍通知父组件', async () => {
    const { editor, calls } = setup()
    await flushPromises()
    calls.length = 0
    const seen: unknown[] = []
    await editor.mutate({ type: 'cmd', argvs: ['INCR', 'k1'] }, { failKey: 'f', reload: false, onSuccess: (r) => seen.push(r) })
    expect(calls).toEqual(['exec:INCR k1', 'changed'])
    expect(seen).toEqual(['raw'])
  })
})
