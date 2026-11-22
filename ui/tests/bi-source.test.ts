import { describe, expect, it } from 'vitest'
import {
  columnsToFields, decodeSource, documentTable, encodeSource, sameSource, sourceKeyPart, sqlColumnKind, sqlTable, tableTimeFields
} from '@/utils/bi-source'
import { fieldMap } from '@/utils/bi-compat'
import { buildDefaultWidgets, type BiDefaultLabels } from '@/utils/bi-defaults'
import { storageKey } from '@/composables/useBiDashboard'
import type { CollectionField, DocumentItem } from '@/services/types'

const labels: BiDefaultLabels = {
  total: () => 'total', sum: (f) => `sum ${f}`, avg: (f) => `avg ${f}`, trend: () => 'trend',
  histogram: (f) => `hist ${f}`, share: (f) => `share ${f}`, top: (f, n) => `top ${f} ${n}`, dateTrend: (f) => `trend ${f}`
}
const f = (name: string, type: CollectionField['type']): CollectionField => ({ name, type, required: false, unique: false, options: {}, hidden: false, readonly: true })

describe('sqlColumnKind (must mirror the backend sqlColumnKind)', () => {
  // 与 internal/api/schema_aggregate_test.go 的 TestSQLColumnKind 逐项一致：两边口径漂移就会
  // 出现「前端放行、后端 400」或「后端支持、前端不让选」。
  const cases: Record<string, CollectionField['type']> = {
    BIGINT: 'number', INTEGER: 'number', int: 'number', DOUBLE: 'number', 'DECIMAL(10,2)': 'number', REAL: 'number', 'NUMERIC(8, 3)': 'number',
    BOOLEAN: 'bool', bool: 'bool',
    TIMESTAMP: 'date', 'TIMESTAMP WITH TIME ZONE': 'date', DATETIME: 'date', DATE: 'date',
    VARCHAR: 'text', 'VARCHAR(20)': 'text', TEXT: 'text', UUID: 'text', '': 'text', SOMETHING_NEW: 'text',
    JSON: 'json', BLOB: 'json', 'STRUCT(a INTEGER)': 'json', 'INTEGER[]': 'text'
  }
  for (const [input, want] of Object.entries(cases)) {
    it(`${JSON.stringify(input)} → ${want}`, () => expect(sqlColumnKind(input)).toBe(want))
  }
})

describe('columnsToFields', () => {
  it('maps real columns to fields, dropping sensitive and invalid names', () => {
    const out = columnsToFields([
      { name: 'id', type: 'BIGINT', nullable: false },
      { name: 'status', type: 'VARCHAR' },
      { name: 'password_hash', type: 'VARCHAR', sensitive: true },
      { name: 'bad-name', type: 'INT' },
      { name: '1abc', type: 'INT' },
      { name: 'placed_at', type: 'TIMESTAMP' }
    ])
    expect(out.map((x) => [x.name, x.type])).toEqual([['id', 'number'], ['status', 'text'], ['placed_at', 'date']])
    expect(out[0].required).toBe(true)
    expect(out.every((x) => x.readonly && !x.hidden)).toBe(true)
  })
})

describe('tableTimeFields', () => {
  it('lists date columns, preferred names first, and is empty without any', () => {
    const fields = [f('placed_at', 'date'), f('created_at', 'date'), f('note', 'text'), f('updated_at', 'date')]
    expect(tableTimeFields(fields)).toEqual(['created_at', 'updated_at', 'placed_at'])
    expect(tableTimeFields([f('a', 'text'), f('b', 'number')])).toEqual([])
  })
})

describe('source identity', () => {
  it('encodes and decodes with the kind, so a same-named collection and table never collide', () => {
    const c = { kind: 'collection' as const, name: 'events' }
    const t = { kind: 'table' as const, name: 'events' }
    expect(encodeSource(c)).toBe('c:events')
    expect(encodeSource(t)).toBe('t:events')
    expect(decodeSource('c:events')).toEqual(c)
    expect(decodeSource('t:events')).toEqual(t)
    expect(sameSource(c, t)).toBe(false)
    expect(sameSource(c, { ...c })).toBe(true)
    expect(sameSource(null, c)).toBe(false)
    expect(sameSource(null, null)).toBe(false)
  })

  it('rejects malformed values', () => {
    for (const bad of ['', 'events', 'x:events', 'c:', ':events', 'tc:events']) expect(decodeSource(bad), bad).toBeNull()
  })

  it('keeps collection storage keys unchanged and prefixes tables', () => {
    expect(sourceKeyPart({ kind: 'collection', name: 'orders' })).toBe('orders')
    expect(sourceKeyPart({ kind: 'table', name: 'orders' })).toBe('table:orders')
    // 已有集合配置 key 向后兼容；同名表不会串配置。
    expect(storageKey({ projectId: 'p', databaseId: 'd', collection: 'orders' })).toBe('sb_bi_v1:p:d:orders')
    expect(storageKey({ projectId: 'p', databaseId: 'd', collection: 'orders', kind: 'collection' })).toBe('sb_bi_v1:p:d:orders')
    expect(storageKey({ projectId: 'p', databaseId: 'd', collection: 'orders', kind: 'table' })).toBe('sb_bi_v1:p:d:table:orders')
  })
})

describe('built-in time columns belong to collections only', () => {
  it('fieldMap adds created_at / updated_at for collections and not for tables', () => {
    expect([...fieldMap([f('a', 'text')]).keys()]).toEqual(['created_at', 'updated_at', 'a'])
    expect([...fieldMap([f('a', 'text')], false).keys()]).toEqual(['a'])
  })

})

describe('default board for a SQL table', () => {
  const fields = [f('id', 'number'), f('amount', 'number'), f('status', 'text'), f('placed_at', 'date')]

  it('uses the table\'s own time column for the trend, and never created_at', () => {
    const board = buildDefaultWidgets(fields, true, labels, 'placed_at')
    const trend = board.widgets.find((w) => w.id === 'trend')!
    expect(trend.groupBy).toBe('placed_at:day')
    expect(board.widgets.some((w) => w.groupBy?.startsWith('created_at'))).toBe(false)
    // 趋势已占用该时间列，不再为它重复生成第二张趋势图。
    expect(board.widgets.filter((w) => w.groupBy?.startsWith('placed_at'))).toHaveLength(1)
  })

  it('a table without a time column gets no trend; the lone total is full width, KPIs share a row', () => {
    const lone = buildDefaultWidgets([], true, labels, null)
    expect(lone.widgets[0]).toMatchObject({ id: 'kpi-total', span: 12 })
    const board = buildDefaultWidgets([f('amount', 'number'), f('status', 'text')], true, labels, null)
    expect(board.widgets.find((w) => w.id === 'trend')).toBeUndefined()
    expect(board.widgets[0]).toMatchObject({ id: 'kpi-total', span: 6 })
    expect(board.widgets[1]).toMatchObject({ id: 'kpi-sum-amount', span: 6 })
    expect(board.widgets.length).toBeGreaterThan(1)
  })

  it('collections keep the created_at trend by default (regression)', () => {
    const board = buildDefaultWidgets(fields, true, labels)
    expect(board.widgets.find((w) => w.id === 'trend')?.groupBy).toBe('created_at:day')
    // KPI（总记录 + 合计）同一行，趋势图随后独占一行
    expect(board.widgets.slice(0, 3).map((w) => [w.id, w.span])).toEqual([['kpi-total', 6], ['kpi-sum-amount', 6], ['trend', 12]])
  })
})

describe('detail row normalisation', () => {
  it('documentTable: system columns first, registered fields, then unregistered keys', () => {
    const items: DocumentItem[] = [{ id: 'r1', createdAt: 'c', updatedAt: 'u', data: { title: 'A', extra: 1 } }]
    const t = documentTable(items, [f('title', 'text'), { ...f('secret', 'text'), hidden: true }])
    expect(t.columns).toEqual(['id', 'created_at', 'updated_at', 'title', 'extra'])
    expect(t.rows[0]).toEqual({ id: 'r1', created_at: 'c', updated_at: 'u', title: 'A', extra: 1 })
  })

  it('documentTable: a document field named like a system column cannot spoof the real id', () => {
    const t = documentTable([{ id: 'real', createdAt: 'c', updatedAt: 'u', data: { id: 'fake', created_at: 'x' } }], [])
    // 系统列以文档元数据为准，业务数据里的同名键不能覆盖。
    expect(t.rows[0].id).toBe('real')
    expect(t.rows[0].created_at).toBe('c')
    expect(t.columns.filter((c) => c === 'id')).toHaveLength(1)
  })

  it('documentTable: a hidden field never comes back through the row data', () => {
    const hidden = { ...f('secret', 'text'), hidden: true }
    const t = documentTable([{ id: 'r', createdAt: 'c', updatedAt: 'u', data: { title: 'A', secret: 's3' } }], [f('title', 'text'), hidden])
    expect(t.columns).not.toContain('secret')
    expect(t.rows[0]).not.toHaveProperty('secret')
    expect(t.rows[0].title).toBe('A')
  })

  it('sqlTable: aligns array rows with columns and drops sensitive columns', () => {
    const t = sqlTable({
      columns: [{ name: 'id', type: 'BIGINT' }, { name: 'password_hash', type: 'VARCHAR', sensitive: true }, { name: 'name', type: 'VARCHAR' }],
      rows: [[1, null, 'Ada'], [2, null, 'Bob']]
    })
    expect(t.columns).toEqual(['id', 'name'])
    expect(t.rows).toEqual([{ id: 1, name: 'Ada' }, { id: 2, name: 'Bob' }])
  })

  it('sqlTable: empty result keeps the columns', () => {
    const t = sqlTable({ columns: [{ name: 'a', type: 'INT' }], rows: [] })
    expect(t).toEqual({ columns: ['a'], rows: [] })
  })
})
