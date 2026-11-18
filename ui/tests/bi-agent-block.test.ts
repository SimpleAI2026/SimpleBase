import { describe, expect, it } from 'vitest'
import type { CollectionField } from '@/services/types'
import {
  MAX_BLOCKS,
  MAX_BLOCK_CHARS,
  MAX_TITLE_CHARS,
  defaultSpan,
  parseChartBlocks,
  parseChartBlocksDetailed,
  stripChartBlocks,
  validateAgentWidget
} from '@/utils/bi-agent-block'
import { ALL_TYPES } from '@/utils/bi-compat'

const f = (name: string, type: CollectionField['type'], hidden = false): CollectionField => ({
  name, type, required: false, unique: false, options: {}, hidden, readonly: false
})
const schema = [f('amount', 'number'), f('qty', 'number'), f('status', 'select'), f('paid', 'bool'), f('placed', 'date'), f('secret', 'text', true)]
const block = (o: unknown) => `<!-- sb:chart ${JSON.stringify(o)} -->`
const parse = (o: unknown) => parseChartBlocks(`说明\n${block(o)}`)[0]
const validate = (o: Record<string, unknown>) => validateAgentWidget(parse({ version: 1, title: 'T', type: 'auto', metrics: ['count'], ...o }), schema, 'id1')

describe('parseChartBlocks', () => {
  it('extracts blocks and strips them from the prose', () => {
    const text = `近 30 天各状态金额。\n${block({ version: 1, title: 'A', type: 'bar', metrics: ['count'] })}\n${block({ version: 1, title: 'B', type: 'pie', metrics: ['count'] })}`
    const blocks = parseChartBlocks(text)
    expect(blocks.map((b) => b.title)).toEqual(['A', 'B'])
    expect(stripChartBlocks(text)).toBe('近 30 天各状态金额。')
  })

  it('maps snake_case keys and keeps only whitelisted ones', () => {
    const b = parse({
      version: 1, title: 'T', type: 'line', metrics: ['count', 'sum:amount'], group_by: 'placed:day', series: 'status',
      filter: 'amount>1', sort: '-count', limit: 20, area: true, donut: true,
      // 下面这些不在白名单：布局与标识必须被丢弃。
      id: 'hacked', span: 12, sql: 'DROP TABLE x', html: '<script>x</script>'
    })
    expect(b).toEqual({
      version: 1, title: 'T', type: 'line', metrics: ['count', 'sum:amount'], groupBy: 'placed:day', series: 'status',
      filter: 'amount>1', sort: '-count', limit: 20, area: true, donut: true
    })
    expect(b).not.toHaveProperty('id')
    expect(b).not.toHaveProperty('span')
    expect(b).not.toHaveProperty('sql')
  })

  it('parses scatter keys', () => {
    expect(parse({ version: 1, title: 'S', type: 'scatter', x: 'amount', y: 'qty', size: 'qty' })).toMatchObject({ x: 'amount', y: 'qty', size: 'qty', metrics: [] })
  })

  it('takes at most four blocks', () => {
    const text = Array.from({ length: 7 }, (_, i) => block({ version: 1, title: `c${i}`, type: 'auto', metrics: ['count'] })).join('\n')
    expect(parseChartBlocks(text)).toHaveLength(MAX_BLOCKS)
  })

  it('skips broken, oversized and non-object blocks but keeps the good ones', () => {
    const huge = block({ version: 1, title: 'x'.repeat(MAX_BLOCK_CHARS), type: 'auto', metrics: ['count'] })
    const text = [
      '<!-- sb:chart {坏的 json -->',
      '<!-- sb:chart [1,2] -->',
      '<!-- sb:chart "str" -->',
      '<!-- sb:chart null -->',
      huge,
      block({ version: 1, title: 'ok', type: 'auto', metrics: ['count'] })
    ].join('\n')
    expect(parseChartBlocks(text).map((b) => b.title)).toEqual(['ok'])
  })

  it('returns nothing for empty or absurdly long content', () => {
    expect(parseChartBlocks('')).toEqual([])
    expect(parseChartBlocks('没有块')).toEqual([])
    expect(parseChartBlocks('x'.repeat(300 * 1024))).toEqual([])
  })

  it('drops non-string metrics and defaults missing fields', () => {
    const b = parse({ title: 'T', metrics: ['count', 5, null, '  sum:amount  ', ''] })
    expect(b.metrics).toEqual(['count', 'sum:amount'])
    expect(b.version).toBe(1)
    expect(b.type).toBe('auto')
  })

  it('tolerates prose around the block and extra whitespace', () => {
    const text = `前文 <!--   sb:chart   {"version":1,"title":"T","type":"auto","metrics":["count"]}   --> 后文`
    expect(parseChartBlocks(text)).toHaveLength(1)
    expect(stripChartBlocks(text)).toBe('前文  后文')
  })
})

describe('validateAgentWidget', () => {
  it('accepts a valid block and assigns id and span itself', () => {
    const r = validate({ type: 'stacked_bar', metrics: ['sum:amount'], group_by: 'status', series: 'paid' })
    expect(r.ok).toBe(true)
    if (r.ok) {
      expect(r.widget).toMatchObject({ id: 'id1', title: 'T', type: 'stacked_bar', metrics: ['sum:amount'], groupBy: 'status', series: 'paid', span: 8 })
    }
  })

  it('accepts every one of the nine chart types with a fitting config', () => {
    const cases: Record<string, Record<string, unknown>> = {
      auto: { metrics: ['count'] },
      line: { metrics: ['count'], group_by: 'placed:day', area: true },
      column: { metrics: ['count'], group_by: 'amount:bin=10' },
      bar: { metrics: ['count'], group_by: 'status' },
      pie: { metrics: ['count'], group_by: 'status', donut: true },
      table: { metrics: ['count', 'sum:amount'], group_by: 'status' },
      grouped_bar: { metrics: ['count', 'sum:amount'], group_by: 'status' },
      stacked_bar: { metrics: ['count'], group_by: 'status', series: 'paid' },
      scatter: { metrics: [], x: 'amount', y: 'qty', series: 'status' }
    }
    expect(Object.keys(cases).sort()).toEqual([...ALL_TYPES].sort())
    for (const [type, over] of Object.entries(cases)) {
      const r = validate({ type, ...over })
      expect(r.ok, `${type}: ${r.ok ? '' : r.reason}`).toBe(true)
    }
  })

  it('rejects unknown fields, hidden fields and incompatible configs with a reason', () => {
    const reason = (o: Record<string, unknown>) => {
      const r = validate(o)
      return r.ok ? 'ok' : r.reason
    }
    expect(reason({ group_by: 'nope' })).toBe('field_unknown')
    expect(reason({ metrics: ['sum:secret'] })).toBe('field_unknown')
    expect(reason({ metrics: ['sum:status'] })).toBe('metric_needs_number')
    expect(reason({ type: 'stacked_bar', metrics: ['avg:amount'], group_by: 'status', series: 'paid' })).toBe('stacked_needs_additive')
    expect(reason({ type: 'stacked_bar', metrics: ['count'], group_by: 'status' })).toBe('stacked_needs_series')
    expect(reason({ type: 'pie', metrics: ['count'], group_by: 'placed:day' })).toBe('pie_needs_category')
    expect(reason({ type: 'scatter', metrics: [], x: 'status', y: 'qty' })).toBe('scatter_needs_number')
    expect(reason({ type: 'line', metrics: ['count'] })).toBe('chart_needs_group_by')
    expect(reason({ type: 'heatmap' })).toBe('type_unknown')
    expect(reason({ metrics: [] })).toBe('metrics_required')
  })

  it('rejects unsupported versions and missing titles', () => {
    const v2 = validateAgentWidget({ ...parse({ version: 1, title: 'T', metrics: ['count'] }), version: 2 }, schema, 'x')
    expect(v2).toMatchObject({ ok: false, reason: 'version_unsupported' })
    const noTitle = validate({ title: '   ' })
    expect(noTitle).toMatchObject({ ok: false, reason: 'title_required' })
  })

  it('clamps limit into the backend range instead of rejecting', () => {
    const agg = validate({ metrics: ['count'], group_by: 'status', limit: 99999 })
    expect(agg.ok && agg.widget.limit).toBe(500)
    const low = validate({ metrics: ['count'], group_by: 'status', limit: -5 })
    expect(low.ok && low.widget.limit).toBe(1)
    const frac = validate({ metrics: ['count'], group_by: 'status', limit: 12.9 })
    expect(frac.ok && frac.widget.limit).toBe(12)
    const pts = validate({ type: 'scatter', metrics: [], x: 'amount', y: 'qty', limit: 99999 })
    expect(pts.ok && pts.widget.limit).toBe(2000)
  })

  it('truncates long titles and strips control characters; markup is kept as inert text', () => {
    const r = validate({ title: `<img src=x onerror=alert(1)>${'a'.repeat(100)}\u0007` })
    expect(r.ok).toBe(true)
    if (r.ok) {
      expect(r.widget.title.length).toBeLessThanOrEqual(MAX_TITLE_CHARS)
      expect(r.widget.title).not.toMatch(/[\u0000-\u001f]/)
      // 不做 HTML 转义：标题只会经 {{ }} 当纯文本渲染，永远不进 v-html。
      expect(r.widget.title.startsWith('<img')).toBe(true)
    }
  })

  it('passes filter through untouched for the backend to validate', () => {
    const r = validate({ filter: 'status="open" && amount>=100' })
    expect(r.ok && r.widget.filter).toBe('status="open" && amount>=100')
  })

  it('an over-long filter rejects the block instead of silently widening the query', () => {
    // 边界：2048 字符放行，2049 拒绝。超长条件绝不能被悄悄丢掉——那会让「带限制的图」变成「全量图」。
    const ok = validate({ filter: 'a'.repeat(2048) })
    expect(ok.ok && ok.widget.filter).toHaveLength(2048)
    const bad = validate({ filter: 'a'.repeat(2049) })
    expect(bad).toMatchObject({ ok: false, reason: 'filter_too_long' })
    expect(validate({ sort: 'b'.repeat(3000) })).toMatchObject({ ok: false, reason: 'sort_too_long' })
  })

  it('failed validation still returns a usable title for the message', () => {
    const r = validate({ title: 'Bad', group_by: 'nope' })
    expect(r).toMatchObject({ ok: false, title: 'Bad' })
  })
})

describe('defaultSpan', () => {
  it('picks widths by chart shape', () => {
    expect(defaultSpan('pie', false)).toBe(4)
    expect(defaultSpan('auto', false)).toBe(4)
    expect(defaultSpan('line', false)).toBe(8)
    expect(defaultSpan('table', false)).toBe(8)
    expect(defaultSpan('scatter', false)).toBe(8)
    expect(defaultSpan('column', false)).toBe(6)
    expect(defaultSpan('bar', true)).toBe(8)
  })
})

describe('parseChartBlocksDetailed', () => {
  it('explains every broken block even when mixed with good ones', () => {
    const huge = block({ version: 1, title: 'x'.repeat(MAX_BLOCK_CHARS), type: 'auto', metrics: ['count'] })
    const text = [
      '<!-- sb:chart {坏的 json -->',
      '<!-- sb:chart [1,2] -->',
      huge,
      block({ version: 1, title: 'ok', type: 'auto', metrics: ['count'] })
    ].join('\n')
    const { blocks, broken } = parseChartBlocksDetailed(text)
    expect(blocks.map((b) => b.title)).toEqual(['ok'])
    expect(broken.map((b) => b.reason)).toEqual(['bad_json', 'block_not_object', 'block_too_large'])
  })

  it('keeps the legacy parseChartBlocks contract (good blocks only)', () => {
    const text = `<!-- sb:chart nope -->\n${block({ version: 1, title: 'ok', type: 'auto', metrics: ['count'] })}`
    expect(parseChartBlocks(text).map((b) => b.title)).toEqual(['ok'])
  })

  it('returns nothing for empty or oversized content', () => {
    expect(parseChartBlocksDetailed('')).toEqual({ blocks: [], broken: [] })
    expect(parseChartBlocksDetailed('x'.repeat(300 * 1024))).toEqual({ blocks: [], broken: [] })
  })
})
