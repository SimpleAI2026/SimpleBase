import { beforeEach, describe, expect, it } from 'vitest'
import type { BiWidget, CollectionField } from '@/services/types'
import {
  MAX_WIDGETS,
  clearConfig,
  dropInvalidWidgets,
  loadConfig,
  newWidgetId,
  saveConfig,
  storageKey,
  useBiDashboard,
  type BiScope
} from '@/composables/useBiDashboard'
import type { BiDefaultLabels } from '@/utils/bi-defaults'

const f = (name: string, type: CollectionField['type']): CollectionField => ({
  name, type, required: false, unique: false, options: {}, hidden: false, readonly: false
})
const labels: BiDefaultLabels = {
  total: () => 'TOTAL', sum: (x) => `sum ${x}`, avg: (x) => `avg ${x}`, trend: () => 'TREND',
  histogram: (x) => `hist ${x}`, share: (x) => `share ${x}`, top: (x, n) => `top${n} ${x}`, dateTrend: (x) => `trend ${x}`
}
const scope: BiScope = { projectId: 'p1', databaseId: 'db1', collection: 'orders' }
const schema = [f('amount', 'number'), f('status', 'select')]
const w = (over: Partial<BiWidget> = {}): BiWidget => ({ id: 'w1', title: 'T', type: 'auto', metrics: ['count'], span: 6, ...over })

/** 内存版 Storage，便于断言写入内容。 */
function memoryStorage(initial: Record<string, string> = {}): Storage & { dump: () => Record<string, string> } {
  const data = new Map(Object.entries(initial))
  return {
    get length() { return data.size },
    clear: () => data.clear(),
    getItem: (k) => data.get(k) ?? null,
    key: (i) => [...data.keys()][i] ?? null,
    removeItem: (k) => void data.delete(k),
    setItem: (k, v) => void data.set(k, v),
    dump: () => Object.fromEntries(data)
  }
}

describe('storage helpers', () => {
  it('keys by project + database + collection', () => {
    expect(storageKey(scope)).toBe('sb_bi_v1:p1:db1:orders')
    expect(storageKey({ ...scope, collection: 'users' })).not.toBe(storageKey(scope))
    expect(storageKey({ ...scope, projectId: 'p2' })).not.toBe(storageKey(scope))
  })

  it('round-trips a config', () => {
    const s = memoryStorage()
    saveConfig(scope, [w()], s)
    expect(loadConfig(scope, s)).toEqual({ version: 1, widgets: [w()] })
  })

  it('treats missing, corrupt and wrong-version data as no config', () => {
    expect(loadConfig(scope, memoryStorage())).toBeNull()
    expect(loadConfig(scope, memoryStorage({ [storageKey(scope)]: '{坏的' }))).toBeNull()
    expect(loadConfig(scope, memoryStorage({ [storageKey(scope)]: JSON.stringify({ version: 2, widgets: [w()] }) }))).toBeNull()
    expect(loadConfig(scope, memoryStorage({ [storageKey(scope)]: JSON.stringify({ version: 1, widgets: 'x' }) }))).toBeNull()
    expect(loadConfig(scope, undefined)).toBeNull()
  })

  it('drops malformed widgets and caps the count', () => {
    const many = Array.from({ length: MAX_WIDGETS + 5 }, (_, i) => w({ id: `w${i}` }))
    const raw = JSON.stringify({ version: 1, widgets: [{ id: 1 }, { id: 'x', title: 't', type: 'auto', metrics: [1], span: 6 }, { ...w(), span: 5 }, ...many] })
    const loaded = loadConfig(scope, memoryStorage({ [storageKey(scope)]: raw }))!
    expect(loaded.widgets).toHaveLength(MAX_WIDGETS)
    expect(loaded.widgets.every((x) => x.id.startsWith('w'))).toBe(true)
  })

  it('swallows storage failures', () => {
    const broken = { ...memoryStorage(), setItem: () => { throw new Error('quota') }, removeItem: () => { throw new Error('x') } } as unknown as Storage
    expect(() => saveConfig(scope, [w()], broken)).not.toThrow()
    expect(() => clearConfig(scope, broken)).not.toThrow()
    expect(() => saveConfig(scope, [w()], undefined)).not.toThrow()
  })

  it('drops widgets whose fields no longer exist', () => {
    const { kept, dropped } = dropInvalidWidgets([w({ id: 'a' }), w({ id: 'b', metrics: ['sum:gone'] }), w({ id: 'c', groupBy: 'status', type: 'pie' })], schema)
    expect(kept.map((x) => x.id)).toEqual(['a', 'c'])
    expect(dropped).toBe(1)
  })

  it('generates unique ids', () => {
    expect(new Set(Array.from({ length: 50 }, () => newWidgetId())).size).toBe(50)
  })
})

describe('useBiDashboard', () => {
  let s: ReturnType<typeof memoryStorage>
  beforeEach(() => { s = memoryStorage() })

  it('builds the default board when nothing is saved, without writing storage', () => {
    const d = useBiDashboard(labels, s)
    d.load(scope, schema, true)
    expect(d.widgets.value.map((x) => x.id)).toContain('kpi-total')
    expect(d.needsSchema.value).toBe(false)
    expect(s.dump()).toEqual({})
  })

  it('flags an unregistered schema', () => {
    const d = useBiDashboard(labels, s)
    d.load(scope, [], false)
    expect(d.needsSchema.value).toBe(true)
    expect(d.widgets.value.map((x) => x.id)).toEqual(['kpi-total', 'trend'])
  })

  it('restores a saved board and drops widgets with missing fields, writing the cleaned board back', () => {
    saveConfig(scope, [w({ id: 'a' }), w({ id: 'b', metrics: ['sum:gone'] })], s)
    const d = useBiDashboard(labels, s)
    d.load(scope, schema, true)
    expect(d.widgets.value.map((x) => x.id)).toEqual(['a'])
    expect(d.dropped.value).toBe(1)
    expect(loadConfig(scope, s)!.widgets.map((x) => x.id)).toEqual(['a'])
    // 再次加载不再重复提示。
    d.load(scope, schema, true)
    expect(d.dropped.value).toBe(0)
  })

  it('persists add / update / remove / duplicate', () => {
    const d = useBiDashboard(labels, s)
    d.load(scope, schema, true)
    const before = d.widgets.value.length
    d.add(w({ id: 'mine', title: 'Mine' }))
    expect(d.widgets.value).toHaveLength(before + 1)
    expect(loadConfig(scope, s)!.widgets.at(-1)!.id).toBe('mine')

    d.update(w({ id: 'mine', title: 'Renamed' }))
    expect(d.widgets.value.find((x) => x.id === 'mine')!.title).toBe('Renamed')

    const copy = d.duplicate('mine')!
    expect(copy.id).not.toBe('mine')
    // 副本紧跟在原图表后面，且度量数组不共享引用。
    const ids = d.widgets.value.map((x) => x.id)
    expect(ids.indexOf(copy.id)).toBe(ids.indexOf('mine') + 1)
    copy.metrics.push('x')
    expect(d.widgets.value.find((x) => x.id === 'mine')!.metrics).toEqual(['count'])

    d.remove('mine')
    expect(d.widgets.value.some((x) => x.id === 'mine')).toBe(false)
    expect(loadConfig(scope, s)!.widgets.some((x) => x.id === 'mine')).toBe(false)
    expect(d.duplicate('missing')).toBeNull()
  })

  it('refuses to grow past the widget cap', () => {
    const d = useBiDashboard(labels, s)
    d.load(scope, schema, true)
    for (let i = 0; i < MAX_WIDGETS + 3; i++) d.add(w({ id: `x${i}` }))
    expect(d.widgets.value).toHaveLength(MAX_WIDGETS)
    expect(d.duplicate(d.widgets.value[0].id)).toBeNull()
  })

  it('add reports whether it succeeded, so callers never show a false "added"', () => {
    const d = useBiDashboard(labels, s)
    d.load(scope, schema, true)
    d.widgets.value = Array.from({ length: MAX_WIDGETS - 1 }, (_, i) => w({ id: `f${i}` }))
    expect(d.add(w({ id: 'last' }))).toBe(true)
    expect(d.widgets.value).toHaveLength(MAX_WIDGETS)
    expect(d.add(w({ id: 'overflow' }))).toBe(false)
    expect(d.widgets.value.map((x) => x.id)).not.toContain('overflow')
  })

  it('reset restores the defaults and clears the saved config', () => {
    const d = useBiDashboard(labels, s)
    d.load(scope, schema, true)
    d.add(w({ id: 'mine' }))
    expect(s.dump()[storageKey(scope)]).toBeDefined()
    d.reset()
    expect(d.widgets.value.some((x) => x.id === 'mine')).toBe(false)
    expect(s.dump()[storageKey(scope)]).toBeUndefined()
    expect(d.widgets.value.map((x) => x.id)).toContain('kpi-total')
  })

  it('clear empties the board and stops persisting', () => {
    const d = useBiDashboard(labels, s)
    d.load(scope, schema, true)
    d.clear()
    expect(d.widgets.value).toEqual([])
    d.add(w({ id: 'orphan' }))
    expect(s.dump()).toEqual({})
  })

  it('keeps boards for different collections separate', () => {
    const a = useBiDashboard(labels, s)
    a.load(scope, schema, true)
    a.add(w({ id: 'only-orders' }))
    const b = useBiDashboard(labels, s)
    b.load({ ...scope, collection: 'users' }, schema, true)
    expect(b.widgets.value.some((x) => x.id === 'only-orders')).toBe(false)
  })
})
