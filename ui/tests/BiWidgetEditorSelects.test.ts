import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import BiWidgetEditor from '@/components/bi/BiWidgetEditor.vue'
import { Select } from '@/components/ui/select'
import { uiStubs } from '@/test/helpers'
import type { BiWidget, CollectionField } from '@/services/types'

const f = (name: string, type: CollectionField['type']): CollectionField => ({
  name, type, required: false, unique: false, options: {}, hidden: false, readonly: false
})
const fields = [f('amount', 'number'), f('qty', 'number'), f('status', 'select'), f('title', 'text'), f('placed', 'date')]
const widget = (over: Partial<BiWidget> = {}): BiWidget => ({ id: 'w1', title: 'T', type: 'table', metrics: ['count'], span: 6, ...over })

// 弹窗壳与 Tooltip 沿用共享桩；Select / Input / Field 等用真实组件，事件契约才是真实的。
const stubs = {
  ...uiStubs,
  Button: false, Input: false, Field: false, FieldLabel: false, FieldError: false,
  Select: false, SelectTrigger: false, SelectValue: false, SelectContent: false, SelectGroup: false, SelectItem: false,
  Switch: false, teleport: true
}

function mountEditor(w: BiWidget) {
  return mount(BiWidgetEditor, { props: { open: true, widget: w, fields }, global: { stubs } })
}

/**
 * 找到挂着某个 data-* 触发器的那个真实 Select，并让它发出 update:modelValue。
 * Select 的弹层在 jsdom 里无法点选，直接触发它对外的事件，走的仍是编辑器里真实的处理函数。
 */
async function choose(wrapper: VueWrapper, marker: string, value: string) {
  const select = wrapper.findAllComponents(Select).find((s) => s.find(`[${marker}]`).exists())
  if (!select) throw new Error(`no Select containing [${marker}]`)
  select.vm.$emit('update:modelValue', value)
  await flushPromises()
}

const save = async (wrapper: VueWrapper) => {
  await wrapper.get('.sb-ok').trigger('click')
  return wrapper.emitted('save')?.[0]?.[0] as BiWidget | undefined
}

describe('metric editing through real Selects', () => {
  it('changing the aggregation keeps a compatible field and drops an incompatible one', async () => {
    const wrapper = mountEditor(widget({ metrics: ['sum:amount'] }))
    await choose(wrapper, 'data-metric-kind="0"', 'avg')
    // number 字段对 avg 仍合法，保留。
    expect((await save(wrapper))!.metrics).toEqual(['avg:amount'])
  })

  it('switching to count drops the field entirely', async () => {
    const wrapper = mountEditor(widget({ metrics: ['sum:amount'] }))
    await choose(wrapper, 'data-metric-kind="0"', 'count')
    expect((await save(wrapper))!.metrics).toEqual(['count'])
  })

  it('switching to an aggregation that does not accept the field leaves it unset and blocks saving', async () => {
    // distinct 接受任意非 json 字段；换成 sum 则 status（select）不合法，字段被清空。
    const wrapper = mountEditor(widget({ metrics: ['distinct:status'] }))
    await choose(wrapper, 'data-metric-kind="0"', 'sum')
    expect(wrapper.get('[data-testid="editor-problem"]').text()).toContain('度量格式不正确')
    expect(await save(wrapper)).toBeUndefined()
  })

  it('picking a field completes the metric (aggregation first, then field — the real path)', async () => {
    const wrapper = mountEditor(widget({ metrics: ['count'] }))
    // count 不带字段选择器；先选聚合方式后字段选择器才出现。
    expect(wrapper.find('[data-metric-field="0"]').exists()).toBe(false)
    await choose(wrapper, 'data-metric-kind="0"', 'sum')
    expect(wrapper.find('[data-metric-field="0"]').exists()).toBe(true)
    await choose(wrapper, 'data-metric-field="0"', 'qty')
    expect((await save(wrapper))!.metrics).toEqual(['sum:qty'])
  })

  it('offers only number / date fields for min and max', async () => {
    const wrapper = mountEditor(widget({ metrics: ['count'] }))
    await choose(wrapper, 'data-metric-kind="0"', 'min')
    expect(wrapper.find('[data-metric-field="0"]').exists()).toBe(true)
    await choose(wrapper, 'data-metric-field="0"', 'placed')
    expect((await save(wrapper))!.metrics).toEqual(['min:placed'])
  })
})

describe('metric draft state', () => {
  it('a picked aggregation stays picked until its field is chosen (regression: it used to bounce back to count)', async () => {
    const wrapper = mountEditor(widget({ metrics: ['count'] }))
    await choose(wrapper, 'data-metric-kind="0"', 'avg')
    await choose(wrapper, 'data-metric-kind="0"', 'avg')
    expect(wrapper.find('[data-metric-field="0"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="editor-problem"]').text()).toContain('度量格式不正确')
  })

  it('an unknown aggregation in a loaded widget falls back to count', () => {
    const wrapper = mountEditor(widget({ metrics: ['median:amount'] }))
    expect(wrapper.find('[data-metric-field="0"]').exists()).toBe(false)
  })
})

describe('grouping through real Selects', () => {
  it('a date field defaults to day buckets and the unit can be changed', async () => {
    const wrapper = mountEditor(widget({ type: 'line', groupBy: undefined }))
    await choose(wrapper, 'data-field="group"', 'placed')
    await choose(wrapper, 'data-field="unit"', 'month')
    expect((await save(wrapper))!.groupBy).toBe('placed:month')
  })

  it('a number field defaults to bin=10, a category field is used as is', async () => {
    const num = mountEditor(widget({ type: 'column', groupBy: undefined }))
    await choose(num, 'data-field="group"', 'amount')
    expect((await save(num))!.groupBy).toBe('amount:bin=10')

    const cat = mountEditor(widget({ type: 'bar', groupBy: undefined }))
    await choose(cat, 'data-field="group"', 'status')
    expect((await save(cat))!.groupBy).toBe('status')
  })

  it('choosing "no grouping" clears group-by and the series that depended on it', async () => {
    const wrapper = mountEditor(widget({ groupBy: 'status', series: 'title' }))
    await choose(wrapper, 'data-field="group"', '__none__')
    const out = (await save(wrapper))!
    expect(Object.keys(out)).not.toContain('groupBy')
    expect(Object.keys(out)).not.toContain('series')
  })

  it('moving the group-by onto the current series field clears that series', async () => {
    const wrapper = mountEditor(widget({ groupBy: 'status', series: 'title' }))
    await choose(wrapper, 'data-field="group"', 'title')
    const out = (await save(wrapper))!
    expect(out.groupBy).toBe('title')
    expect(Object.keys(out)).not.toContain('series')
  })

  it('a series can be chosen and cleared', async () => {
    const wrapper = mountEditor(widget({ groupBy: 'status' }))
    await choose(wrapper, 'data-field="series"', 'title')
    expect((await save(wrapper))!.series).toBe('title')
    const again = mountEditor(widget({ groupBy: 'status', series: 'title' }))
    await choose(again, 'data-field="series"', '__none__')
    expect(Object.keys((await save(again))!)).not.toContain('series')
  })
})

describe('scatter fields and layout through real Selects', () => {
  const scatter = () => widget({ type: 'scatter', metrics: [], x: 'amount', y: 'qty' })

  it('sets and clears the size and series channels', async () => {
    const wrapper = mountEditor(scatter())
    await choose(wrapper, 'data-field="size"', 'qty')
    await choose(wrapper, 'data-field="series"', 'status')
    const out = (await save(wrapper))!
    expect(out).toMatchObject({ size: 'qty', series: 'status' })

    const again = mountEditor({ ...scatter(), size: 'qty', series: 'status' })
    await choose(again, 'data-field="size"', '__none__')
    await choose(again, 'data-field="series"', '__none__')
    const cleared = (await save(again))!
    expect(Object.keys(cleared)).not.toContain('size')
    expect(Object.keys(cleared)).not.toContain('series')
  })

  it('swaps the X and Y fields', async () => {
    const wrapper = mountEditor(scatter())
    await choose(wrapper, 'data-field="x"', 'qty')
    await choose(wrapper, 'data-field="y"', 'amount')
    expect(await save(wrapper)).toMatchObject({ x: 'qty', y: 'amount' })
  })

  it('changes the width', async () => {
    const wrapper = mountEditor(widget())
    await choose(wrapper, 'data-field="span"', '12')
    expect((await save(wrapper))!.span).toBe(12)
  })
})

describe('numeric inputs', () => {
  it('ignores a non-numeric limit and accepts a numeric one', async () => {
    const wrapper = mountEditor(widget())
    await wrapper.get('[data-field="limit"]').setValue('abc')
    expect(Object.keys((await save(wrapper)) ?? {})).not.toContain('limit')
    const ok = mountEditor(widget())
    await ok.get('[data-field="limit"]').setValue('15')
    expect((await save(ok))!.limit).toBe(15)
  })

  it('a limit above the backend range is rejected', async () => {
    const wrapper = mountEditor(widget())
    await wrapper.get('[data-field="limit"]').setValue('9999')
    await flushPromises()
    expect(wrapper.get('[data-testid="editor-problem"]').text()).toContain('数量超出允许范围')
    expect(await save(wrapper)).toBeUndefined()
  })

  it('scatter allows a larger limit than aggregates', async () => {
    const wrapper = mountEditor(widget({ type: 'scatter', metrics: [], x: 'amount', y: 'qty' }))
    await wrapper.get('[data-field="limit"]').setValue('1500')
    expect((await save(wrapper))!.limit).toBe(1500)
  })
})
