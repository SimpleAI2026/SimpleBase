import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { clickText, readyDb, uiStubs, realDialogStubs, inDialog, installJsdomPolyfills, mountDialog } from '@/test/helpers'
import SqlWorkModal from '@/components/modal/SqlWorkModal.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

/** 真实 ToggleGroup / ToggleGroupItem：断言的是 data-state 与模式切换后的真实参数区。 */
const realToggle = { ...uiStubs, ...realDialogStubs, ToggleGroup: false, ToggleGroupItem: false }

installJsdomPolyfills()

describe('SqlWorkModal', () => {
  beforeEach(() => resetApiMocks())

  it('covers parsers, formatters, and all run modes', async () => {
    const w = await mountDialog(SqlWorkModal, {
      props: { open: true, projectId: 'p', database: readyDb },
      global: { stubs: { ...uiStubs, ...realDialogStubs } }
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
    expect(inDialog().findAll('th').find((el) => el.text() === '错误详情')?.classes()).toContain('text-left')
    expect(inDialog().findAll('th').find((el) => el.text() === '#')?.classes()).not.toContain('text-left')
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

  it('runs a SELECT from the footer button and shows query rows', async () => {
    const w = await mountDialog(SqlWorkModal, {
      props: { open: true, projectId: 'p', database: readyDb },
      global: { stubs: { ...uiStubs, ...realDialogStubs } }
    })
    await inDialog().get('textarea').setValue('SELECT 1')
    await clickText(w, '执行查询')
    await flushPromises()
    expect(api.sql.query).toHaveBeenCalledWith('p', 'db-1', expect.objectContaining({ sql: 'SELECT 1' }))
    expect(inDialog().text()).toContain('1 行')
    expect(inDialog().text()).toContain('request_id: r1')
    const resultTable = inDialog().get('table')
    expect(resultTable.classes()).toContain('[&_th]:text-left')
    expect(resultTable.classes()).toContain('[&_td]:text-left')
  })

  it('hides write modes in readonly and still allows query', async () => {
    const w = await mountDialog(SqlWorkModal, {
      props: { open: true, projectId: 'p', database: readyDb, readonly: true },
      global: { stubs: { ...uiStubs, ...realDialogStubs } }
    })
    expect(inDialog().text()).toContain('系统库只读')
    expect(inDialog().text()).toContain('执行查询')
    await inDialog().get('textarea').setValue('SELECT id FROM users')
    await clickText(w, '执行查询')
    await flushPromises()
    expect(api.sql.query).toHaveBeenCalled()
  })

  it('SqlWorkModal toggles modes, pager, switch, and footer', async () => {
    api.sql.query.mockResolvedValue({
      columns: ['id'],
      rows: [['1']],
      rowCount: 1,
      durationMs: 1,
      requestId: 'r'
    })
    const w = await mountDialog(SqlWorkModal, {
      props: { open: true, projectId: 'p', database: readyDb },
      global: { stubs: realToggle }
    })
    // 真实 ToggleGroup：按文本切模式，data-state 与参数区跟着变
    const mode = (label: string) => inDialog().findAll('button').find((b) => b.text() === label)!
    expect(mode('查询').attributes('data-state')).toBe('on')
    await mode('执行').trigger('click')
    expect(mode('执行').attributes('data-state')).toBe('on')
    expect(mode('查询').attributes('data-state')).toBe('off')
    expect(inDialog().findAll('input[type="number"]')).toHaveLength(0)
    await mode('批量').trigger('click')
    expect(inDialog().text()).toContain('事务')
    await inDialog().get('[data-slot="switch"]').trigger('click')
    expect(inDialog().text()).toContain('独立')
    await mode('查询').trigger('click')
    expect(inDialog().findAll('input[type="number"]')).toHaveLength(1)
    await inDialog().get('textarea').setValue('SELECT 1')
    await inDialog().findAll('button').find((b) => b.text().includes('查询') || inDialog().text() === '查询')?.trigger('click')
    await flushPromises()
    if (inDialog().find('.pager-next').exists()) await inDialog().get('.pager-next').trigger('click')
    await inDialog().findAll('button').find((b) => b.text().includes('关闭'))?.trigger('click')
    w.unmount()
  })

  it('SqlWorkModal inputs, pager, and empty query result', async () => {
    api.sql.query.mockResolvedValue({ columns: [], rows: [], rowCount: 0, durationMs: 1, requestId: 'r' })
    const w = await mountDialog(SqlWorkModal, {
      props: { open: true, projectId: 'p', database: readyDb },
      global: { stubs: realToggle }
    })
    const mode = (label: string) => inDialog().findAll('button').find((b) => b.text() === label)!
    await mode('执行').trigger('click')
    for (const i of inDialog().findAll('input')) {
      if (i.attributes('type') === 'number') await i.setValue('10')
      else await i.setValue('["1"]')
    }
    await mode('批量').trigger('click')
    await mode('查询').trigger('click')
    for (const i of inDialog().findAll('input')) {
      if (i.attributes('type') === 'number') await i.setValue('5')
      else await i.setValue('[]')
    }
    await inDialog().get('textarea').setValue('SELECT 1')
    api.sql.query.mockResolvedValueOnce({
      columns: ['id'],
      rows: [['1'], ['2']],
      rowCount: 2,
      durationMs: 1,
      requestId: 'r'
    })
    await (w.vm as Record<string, any>).run()
    await flushPromises()
    if (inDialog().find('.pager-next').exists()) await inDialog().get('.pager-next').trigger('click')
    w.unmount()
  })


})
