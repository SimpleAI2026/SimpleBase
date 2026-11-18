import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import BiChartTypePicker from '@/components/bi/BiChartTypePicker.vue'
import { uiStubs } from '@/test/helpers'
import type { BiWidget, CollectionField } from '@/services/types'

const f = (name: string, type: CollectionField['type']): CollectionField => ({
  name, type, required: false, unique: false, options: {}, hidden: false, readonly: false
})
const fields = [f('amount', 'number'), f('qty', 'number'), f('status', 'select'), f('placed', 'date')]
const w = (over: Partial<BiWidget>): BiWidget => ({ id: 'w', title: 't', type: 'auto', metrics: ['count'], span: 6, ...over })

// Button 用真实组件（disabled 状态要落在真实 DOM 属性上），Tooltip 家族沿用共享桩。
const stubs = { ...uiStubs, Button: false }
const mountPicker = (props: Record<string, unknown>) => mount(BiChartTypePicker, { props: props as never, global: { stubs } })
const button = (wrapper: ReturnType<typeof mountPicker>, type: string) => wrapper.get(`[data-type="${type}"]`)

describe('BiChartTypePicker', () => {
  it('renders the nine types in prototype order with Chinese labels', () => {
    const wrapper = mountPicker({ modelValue: 'auto' })
    expect(wrapper.findAll('[data-type]').map((b) => b.attributes('data-type'))).toEqual(['auto', 'line', 'column', 'bar', 'pie', 'table', 'grouped_bar', 'stacked_bar', 'scatter'])
    expect(wrapper.findAll('[data-type]').map((b) => b.text())).toEqual(['自动', '折线图', '柱状图', '条形图', '饼图', '表格', '对比条形图', '堆积条形图', '散点图'])
  })

  it('marks the selected type through real attributes', () => {
    const wrapper = mountPicker({ modelValue: 'bar' })
    expect(button(wrapper, 'bar').attributes('data-state')).toBe('on')
    expect(button(wrapper, 'bar').attributes('aria-pressed')).toBe('true')
    expect(button(wrapper, 'column').attributes('data-state')).toBe('off')
    expect(button(wrapper, 'column').attributes('aria-pressed')).toBe('false')
  })

  it('emits the clicked type', async () => {
    const wrapper = mountPicker({ modelValue: 'auto' })
    await button(wrapper, 'pie').trigger('click')
    await button(wrapper, 'scatter').trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([['pie'], ['scatter']])
  })

  it('without a widget (page-level picker) every type is selectable', () => {
    const wrapper = mountPicker({ modelValue: 'auto' })
    for (const el of wrapper.findAll('[data-type]')) expect(el.attributes('disabled')).toBeUndefined()
  })

  it('greys out types the widget cannot satisfy, and keeps compatible ones enabled', () => {
    // 时间桶 + 1 个度量：折线 / 柱 / 条 / 表格可以；饼图、堆积、对比、散点不行。
    const wrapper = mountPicker({ modelValue: 'auto', widget: w({ groupBy: 'placed:day' }), fields })
    const disabled = wrapper.findAll('[data-type]').filter((b) => b.attributes('disabled') !== undefined).map((b) => b.attributes('data-type'))
    expect(disabled).toEqual(['pie', 'grouped_bar', 'stacked_bar', 'scatter'])
    for (const type of ['auto', 'line', 'column', 'bar', 'table']) expect(button(wrapper, type).attributes('disabled'), type).toBeUndefined()
  })

  it('the editor can enter scatter setup even before X / Y are configured (no dead end)', () => {
    const strict = mountPicker({ modelValue: 'auto', widget: w({ groupBy: 'placed:day' }), fields })
    expect(button(strict, 'scatter').attributes('disabled')).toBeDefined()
    const editor = mountPicker({ modelValue: 'auto', widget: w({ groupBy: 'placed:day' }), fields, allowScatterSetup: true })
    expect(button(editor, 'scatter').attributes('disabled')).toBeUndefined()
    // 只放开散点：其他类型仍按配置置灰。
    expect(button(editor, 'pie').attributes('disabled')).toBeDefined()
    expect(button(editor, 'stacked_bar').attributes('disabled')).toBeDefined()
  })

  it('leaving scatter is never a dead end: every type is reachable while scatter is selected in the editor', () => {
    const half = w({ metrics: [], x: 'amount' })
    const editor = mountPicker({ modelValue: 'scatter', widget: half, fields, allowScatterSetup: true })
    for (const el of editor.findAll('[data-type]')) expect(el.attributes('disabled'), el.attributes('data-type')).toBeUndefined()
    // 页面级严格模式下仍然按配置置灰。
    const strict = mountPicker({ modelValue: 'scatter', widget: half, fields })
    expect(strict.get('[data-type="table"]').attributes('disabled')).toBeDefined()
  })

  it('a disabled type does not emit when clicked', async () => {
    const wrapper = mountPicker({ modelValue: 'auto', widget: w({ groupBy: 'placed:day' }), fields })
    await button(wrapper, 'pie').trigger('click')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('explains why a type is unavailable in the tooltip', () => {
    const wrapper = mountPicker({ modelValue: 'auto', widget: w({ groupBy: 'placed:day' }), fields })
    expect(wrapper.text()).toContain('饼图的分组需要是分类字段')
    expect(wrapper.text()).toContain('堆积条形图需要选择拆分系列')
    expect(wrapper.text()).toContain('散点图需要同时选择 X 与 Y')
  })

  it('stacked bar needs an additive metric', () => {
    const ok = mountPicker({ modelValue: 'auto', widget: w({ groupBy: 'status', series: 'status', metrics: ['sum:amount'] }), fields })
    expect(button(ok, 'stacked_bar').attributes('disabled')).toBeUndefined()
    const bad = mountPicker({ modelValue: 'auto', widget: w({ groupBy: 'status', series: 'status', metrics: ['avg:amount'] }), fields })
    expect(button(bad, 'stacked_bar').attributes('disabled')).toBeDefined()
    expect(bad.text()).toContain('只支持记录数与求和')
  })

  it('scatter widgets only fit scatter and table', () => {
    const wrapper = mountPicker({ modelValue: 'scatter', widget: w({ metrics: [], x: 'amount', y: 'qty' }), fields })
    const enabled = wrapper.findAll('[data-type]').filter((b) => b.attributes('disabled') === undefined).map((b) => b.attributes('data-type'))
    expect(enabled).toEqual(['auto', 'table', 'scatter'])
  })

  it('shows what auto resolved to, but only while auto is selected', () => {
    const auto = mountPicker({ modelValue: 'auto', inferred: 'stacked_bar' })
    expect(auto.get('[data-testid="inferred"]').text()).toBe('已推断为：堆积条形图')
    const fixed = mountPicker({ modelValue: 'bar', inferred: 'stacked_bar' })
    expect(fixed.find('[data-testid="inferred"]').exists()).toBe(false)
    const none = mountPicker({ modelValue: 'auto', inferred: null })
    expect(none.find('[data-testid="inferred"]').exists()).toBe(false)
    const kpi = mountPicker({ modelValue: 'auto', inferred: 'kpi' })
    expect(kpi.get('[data-testid="inferred"]').text()).toBe('已推断为：指标卡')
  })

  it('picking a type fixes it: the selection no longer follows the inferred value', async () => {
    const wrapper = mountPicker({ modelValue: 'auto', inferred: 'pie' })
    await button(wrapper, 'bar').trigger('click')
    await wrapper.setProps({ modelValue: 'bar' })
    expect(button(wrapper, 'bar').attributes('data-state')).toBe('on')
    expect(button(wrapper, 'auto').attributes('data-state')).toBe('off')
    expect(wrapper.find('[data-testid="inferred"]').exists()).toBe(false)
  })
})
