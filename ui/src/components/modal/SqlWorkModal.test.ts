import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '../../test/api-mock'
import { readyDb, uiStubs } from '../../test/helpers'
import SqlWorkModal from './SqlWorkModal.vue'

vi.mock('../../services/api', async () => {
  const m = await import('../../test/api-mock')
  return { api: m.api, isMock: false }
})

describe('SqlWorkModal', () => {
  beforeEach(() => resetApiMocks())

  it('covers parsers, formatters, and all run modes', async () => {
    const w = mount(SqlWorkModal, {
      props: { open: true, projectId: 'p', database: readyDb },
      global: { stubs: uiStubs }
    })
    const vm = w.vm as any
    expect(vm.formatCell(null)).toBe('NULL')
    expect(vm.formatCell('x')).toBe('x')
    expect(vm.formatCell({ a: 1 })).toContain('a')
    vm.argsText = ''
    expect(vm.parseArgs()).toEqual([])
    vm.argsText = '[1]'
    expect(vm.parseArgs()).toEqual([1])
    vm.argsText = '{"a":1}'
    expect(() => vm.parseArgs()).toThrow()
    vm.argsText = '{bad'
    expect(() => vm.parseArgs()).toThrow()

    vm.sqlText = 'INSERT INTO t VALUES (1)\n-- skip\nUPDATE t SET x=1 -- args=["a"]\nUPDATE t SET y=1 -- args=[not-json]'
    const batch = vm.parseBatch()
    expect(batch.length).toBe(3)
    expect(batch[1].args).toEqual(['a'])

    vm.sqlText = 'INSERT INTO t'
    expect(vm.sqlHint).toContain('write_in_read_only')
    vm.sqlText = 'SELECT 1'
    vm.argsText = ''
    await vm.run()
    expect(api.sql.query).toHaveBeenCalled()

    vm.mode = 'execute'
    await vm.run()
    expect(api.sql.execute).toHaveBeenCalled()

    vm.mode = 'batch'
    vm.sqlText = 'INSERT 1\nINSERT 2'
    api.sql.batch.mockResolvedValueOnce({
      results: [{ rowsAffected: 1 }, { rowsAffected: 1 }],
      durability: 'ok',
      durationMs: 1,
      requestId: 'b'
    })
    await vm.run()
    expect(toast.success).toHaveBeenCalled()
    vm.sqlText = 'INSERT 1\nINSERT 2'
    api.sql.batch.mockResolvedValueOnce({
      results: [{ errorCode: 'e', errorMessage: 'bad' }],
      durability: 'ok',
      durationMs: 1,
      requestId: 'b',
      error: { failedIndex: 0, code: 'e', message: 'bad' }
    })
    await vm.run()
    api.sql.batch.mockResolvedValueOnce({
      results: [{ errorCode: 'e' }, { rowsAffected: 1 }],
      durability: 'ok',
      durationMs: 1,
      requestId: 'b'
    })
    await vm.run()
    vm.sqlText = '-- only'
    await vm.run()
    expect(toast.warning).toHaveBeenCalledWith('请输入至少一条 SQL')

    api.sql.query.mockRejectedValueOnce(new Error('q'))
    vm.mode = 'query'
    vm.sqlText = 'SELECT 1'
    await vm.run()
    expect(toast.error).toHaveBeenCalledWith('q')

    vm.onOpenChange(false)
    vm.resetAll()
    await w.setProps({ database: null })
    vm.sqlText = 'SELECT 1'
    await vm.run()
    expect(toast.warning).toHaveBeenCalledWith('请先选择数据库')
    await w.setProps({ readonly: true, database: readyDb, open: true })
    await flushPromises()
    expect(vm.mode).toBe('query')
    w.unmount()
  })
})
