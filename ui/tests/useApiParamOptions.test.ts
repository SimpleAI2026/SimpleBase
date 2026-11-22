import { flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useApiParamOptions } from '@/composables/useApiParamOptions'
import { useProjectStore } from '@/stores/project'
import { api, resetApiMocks } from '@/test/api-mock'

vi.mock('@/services/api', async () => {
  const module = await import('@/test/api-mock')
  return { api: module.api }
})

describe('useApiParamOptions', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    resetApiMocks()
  })

  it('project 不发请求，直接取自项目 store', async () => {
    useProjectStore().projects = [{ id: 'p1', name: '项目一', createdAt: '' }]
    const { options } = useApiParamOptions(ref('p1'))
    expect(await options('project')).toEqual([{ value: 'p1', label: '项目一', hint: 'p1' }])
    expect(api.databases.list).not.toHaveBeenCalled()
  })

  it('同一 项目+kind+依赖 命中缓存，只请求一次', async () => {
    api.databases.list.mockResolvedValue([{ id: 'db1', name: '主库' }])
    const { options } = useApiParamOptions(ref('p1'))
    const first = await options('databases')
    const second = await options('databases')
    expect(first).toEqual([{ value: 'db1', label: '主库', hint: 'db1' }])
    expect(second).toBe(first)
    expect(api.databases.list).toHaveBeenCalledTimes(1)
  })

  it('依赖值不同使用不同缓存键，依赖键顺序无关', async () => {
    api.db.collections.mockImplementation(async (_p: string, d: string) => [`c-${d}`])
    const { options } = useApiParamOptions(ref('p1'))
    expect(await options('collections', { databaseID: 'a' })).toEqual([{ value: 'c-a', label: 'c-a' }])
    expect(await options('collections', { databaseID: 'b' })).toEqual([{ value: 'c-b', label: 'c-b' }])
    await options('collections', { databaseID: 'a' })
    expect(api.db.collections).toHaveBeenCalledTimes(2)
    api.db.collections.mockClear()
    await options('collections', { x: '1', y: '2' })
    await options('collections', { y: '2', x: '1' })
    expect(api.db.collections).toHaveBeenCalledTimes(1)
  })

  it('切换项目后缓存清空并按新项目请求', async () => {
    api.databases.list.mockResolvedValue([])
    const pid = ref('p1')
    const { options } = useApiParamOptions(pid)
    await options('databases')
    pid.value = 'p2'
    await flushPromises()
    await options('databases')
    expect(api.databases.list).toHaveBeenNthCalledWith(1, 'p1')
    expect(api.databases.list).toHaveBeenNthCalledWith(2, 'p2')
  })

  it('数据源失败时 reject，且失败不进缓存，下次可重试', async () => {
    api.databases.list.mockRejectedValueOnce(new Error('boom')).mockResolvedValueOnce([{ id: 'db1', name: 'db1' }])
    const { options } = useApiParamOptions(ref('p1'))
    await expect(options('databases')).rejects.toThrow('boom')
    await flushPromises()
    expect(await options('databases')).toEqual([{ value: 'db1', label: 'db1' }])
    expect(api.databases.list).toHaveBeenCalledTimes(2)
  })

  it('未知 kind 返回空数组', async () => {
    const { options } = useApiParamOptions(ref('p1'))
    expect(await options('no-such-kind')).toEqual([])
  })

  it('覆盖其余数据源的映射', async () => {
    api.databases.schema.mockResolvedValue({ tables: [{ name: 't1' }] })
    api.agents.list.mockResolvedValue([{ id: 'a1', name: '助手' }])
    api.agentThreads.list.mockResolvedValue([{ id: 'th1', title: '会话' }])
    api.cronjobs.list.mockResolvedValue([{ id: 'j1', name: '任务' }])
    api.gofunctions.list.mockResolvedValue([{ name: 'fn' }])
    api.triggers.list.mockResolvedValue([{ id: 'tr1', target: 'fn' }])
    api.sandboxes.list.mockResolvedValue([{ id: 's1', name: '沙盒' }])
    api.mcpServers.list.mockResolvedValue({ servers: [{ id: 'm1', name: 'mcp' }], builtin: [] })
    api.apiKeys.list.mockResolvedValue([{ id: 'k1' }])
    api.backups.list.mockResolvedValue([{ id: 'b1', createdAt: '2026-01-01' }])
    const { options } = useApiParamOptions(ref('p1'))
    expect(await options('tables', { databaseID: 'd' })).toEqual([{ value: 't1', label: 't1' }])
    expect(await options('agents')).toEqual([{ value: 'a1', label: '助手', hint: 'a1' }])
    expect(await options('agent-threads')).toEqual([{ value: 'th1', label: '会话', hint: 'th1' }])
    expect(await options('cron-jobs')).toEqual([{ value: 'j1', label: '任务', hint: 'j1' }])
    expect(await options('gofunctions')).toEqual([{ value: 'fn', label: 'fn' }])
    expect(await options('function-triggers')).toEqual([{ value: 'tr1', label: 'fn', hint: 'tr1' }])
    expect(await options('sandboxes')).toEqual([{ value: 's1', label: '沙盒', hint: 's1' }])
    expect(await options('mcp-servers')).toEqual([{ value: 'm1', label: 'mcp', hint: 'm1' }])
    expect(await options('api-keys')).toEqual([{ value: 'k1', label: 'k1' }])
    expect(await options('backups')).toEqual([{ value: 'b1', label: 'b1', hint: '2026-01-01' }])
  })
})
