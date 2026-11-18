import { mount, flushPromises } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import BiWidgetEditor from '@/components/bi/BiWidgetEditor.vue'
import { uiStubs } from '@/test/helpers'
import type { BiWidget, CollectionField } from '@/services/types'

const f = (name: string, type: CollectionField['type']): CollectionField => ({
  name, type, required: false, unique: false, options: {}, hidden: false, readonly: false
})
const fields = [f('amount', 'number'), f('qty', 'number'), f('status', 'select'), f('title', 'text'), f('placed', 'date'), f('payload', 'json')]
const widget = (over: Partial<BiWidget> = {}): BiWidget => ({ id: 'w1', title: '状态占比', type: 'pie', metrics: ['count'], groupBy: 'status', span: 6, ...over })

// Select / Switch / Button / Input / Field 用真实组件：要验证的是真实 DOM 属性与事件契约。
// 弹窗壳（SbModal）与 Tooltip 家族沿用共享桩。
const stubs = {
  ...uiStubs,
  Button: false, Input: false, Field: false, FieldLabel: false, FieldError: false,
  Select: false, SelectTrigger: false, SelectValue: false, SelectContent: false, SelectGroup: false, SelectItem: false,
  Switch: false, teleport: true
}

function mountEditor(props: { widget?: BiWidget | null; open?: boolean } = {}) {
  return mount(BiWidgetEditor, {
    props: { open: props.open ?? true, widget: props.widget ?? null, fields },
    global: { stubs }
  })
}

const root = (w: ReturnType<typeof mountEditor>) => w.get('[data-testid="bi-editor"]')
const typeBtn = (w: ReturnType<typeof mountEditor>, type: string) => w.get(`[data-type="${type}"]`)
const saved = (w: ReturnType<typeof mountEditor>) => w.emitted('save')?.[0]?.[0] as BiWidget | undefined

describe('BiWidgetEditor new widget', () => {
  it('starts blank: auto type, a count metric, and cannot be saved without a title', async () => {
    const wrapper = mountEditor()
    expect(root(wrapper).find('[data-field="title"]').element).toHaveProperty('value', '')
    expect(wrapper.text()).toContain('请填写图表标题')
    await wrapper.get('.sb-ok').trigger('click')
    expect(wrapper.emitted('save')).toBeUndefined()
    expect(wrapper.get('.sb-modal').attributes('data-title')).toBe('添加图表')
  })

  it('saves a titled KPI widget with a fresh id and trimmed text', async () => {
    const wrapper = mountEditor()
    await root(wrapper).get('[data-field="title"]').setValue('  总数  ')
    await wrapper.get('.sb-ok').trigger('click')
    const out = saved(wrapper)!
    expect(out).toMatchObject({ title: '总数', type: 'auto', metrics: ['count'], span: 6 })
    expect(out.id).toMatch(/^w-/)
    expect(wrapper.emitted('update:open')?.at(-1)).toEqual([false])
    // 未设置的可选键不会写成 undefined。
    expect(Object.keys(out)).not.toContain('groupBy')
    expect(Object.keys(out)).not.toContain('filter')
  })
})

describe('BiWidgetEditor editing', () => {
  it('prefills from the widget and shows the edit title', () => {
    const wrapper = mountEditor({ widget: widget() })
    expect(wrapper.get('.sb-modal').attributes('data-title')).toBe('编辑图表')
    expect((root(wrapper).get('[data-field="title"]').element as HTMLInputElement).value).toBe('状态占比')
    expect(typeBtn(wrapper, 'pie').attributes('data-state')).toBe('on')
  })

  it('keeps the id and edits the title, filter, limit', async () => {
    const wrapper = mountEditor({ widget: widget() })
    await root(wrapper).get('[data-field="title"]').setValue('新标题')
    await root(wrapper).get('[data-field="filter"]').setValue('  amount>1  ')
    await root(wrapper).get('[data-field="limit"]').setValue('5')
    await wrapper.get('.sb-ok').trigger('click')
    expect(saved(wrapper)).toMatchObject({ id: 'w1', title: '新标题', filter: 'amount>1', limit: 5, groupBy: 'status', type: 'pie' })
  })

  it('does not mutate the original widget until saved', async () => {
    const original = widget()
    const wrapper = mountEditor({ widget: original })
    await root(wrapper).get('[data-field="title"]').setValue('changed')
    expect(original.title).toBe('状态占比')
  })

  it('an emptied filter or limit is dropped rather than saved as an empty value', async () => {
    const wrapper = mountEditor({ widget: widget({ filter: 'a=1', limit: 5 }) })
    await root(wrapper).get('[data-field="filter"]').setValue('   ')
    await root(wrapper).get('[data-field="limit"]').setValue('')
    await wrapper.get('.sb-ok').trigger('click')
    const out = saved(wrapper)!
    expect(Object.keys(out)).not.toContain('filter')
    expect(Object.keys(out)).not.toContain('limit')
  })
})

describe('BiWidgetEditor metrics', () => {
  it('adds and removes metrics, keeps at least one, caps at six', async () => {
    const wrapper = mountEditor({ widget: widget({ type: 'table' }) })
    expect(wrapper.findAll('[data-testid="metric-row"]')).toHaveLength(1)
    expect(wrapper.get('[data-remove-metric="0"]').attributes('disabled')).toBeDefined()
    for (let i = 0; i < 8; i++) await wrapper.get('[data-action="add-metric"]').trigger('click')
    expect(wrapper.findAll('[data-testid="metric-row"]')).toHaveLength(6)
    expect(wrapper.get('[data-action="add-metric"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-remove-metric="5"]').trigger('click')
    expect(wrapper.findAll('[data-testid="metric-row"]')).toHaveLength(5)
  })

  it('an incomplete metric (kind picked, field missing) blocks saving with a reason', async () => {
    const wrapper = mountEditor({ widget: widget({ type: 'table', metrics: ['sum:'] }) })
    await flushPromises()
    expect(wrapper.get('[data-testid="editor-problem"]').text()).toContain('度量格式不正确')
    await wrapper.get('.sb-ok').trigger('click')
    expect(wrapper.emitted('save')).toBeUndefined()
  })
})

describe('BiWidgetEditor type compatibility', () => {
  it('greys incompatible types and explains the problem for the chosen one', async () => {
    const wrapper = mountEditor({ widget: widget({ type: 'pie', groupBy: 'placed:day' }) })
    await flushPromises()
    expect(wrapper.get('[data-testid="editor-problem"]').text()).toContain('饼图的分组需要是分类字段')
    expect(typeBtn(wrapper, 'pie').attributes('disabled')).toBeDefined()
    expect(typeBtn(wrapper, 'line').attributes('disabled')).toBeUndefined()
    await wrapper.get('.sb-ok').trigger('click')
    expect(wrapper.emitted('save')).toBeUndefined()
    // 改成合法类型后问题消失，可以保存。
    await typeBtn(wrapper, 'line').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="editor-problem"]').exists()).toBe(false)
    await wrapper.get('.sb-ok').trigger('click')
    expect(saved(wrapper)).toMatchObject({ type: 'line', groupBy: 'placed:day' })
  })

  it('shows what auto resolves to', async () => {
    const wrapper = mountEditor({ widget: widget({ type: 'auto', groupBy: 'placed:day' }) })
    await flushPromises()
    expect(wrapper.get('[data-testid="inferred"]').text()).toBe('已推断为：折线图')
  })
})

describe('BiWidgetEditor style switches', () => {
  it('shows the area switch for lines and writes it', async () => {
    const wrapper = mountEditor({ widget: widget({ type: 'line', groupBy: 'placed:day' }) })
    const sw = wrapper.get('[data-switch="area"]')
    expect(sw.attributes('data-state')).toBe('unchecked')
    await sw.trigger('click')
    expect(sw.attributes('data-state')).toBe('checked')
    await wrapper.get('.sb-ok').trigger('click')
    expect(saved(wrapper)).toMatchObject({ type: 'line', area: true })
  })

  it('shows the donut switch for pies only', async () => {
    const pie = mountEditor({ widget: widget() })
    expect(pie.find('[data-switch="donut"]').exists()).toBe(true)
    expect(pie.find('[data-switch="area"]').exists()).toBe(false)
    await pie.get('[data-switch="donut"]').trigger('click')
    await pie.get('.sb-ok').trigger('click')
    expect(saved(pie)).toMatchObject({ donut: true })

    const bar = mountEditor({ widget: widget({ type: 'bar' }) })
    expect(bar.find('[data-switch="donut"]').exists()).toBe(false)
    expect(bar.find('[data-switch="area"]').exists()).toBe(false)
  })

  it('an auto widget that resolves to a pie also gets the donut switch', () => {
    const wrapper = mountEditor({ widget: widget({ type: 'auto', groupBy: 'status' }) })
    expect(wrapper.find('[data-switch="donut"]').exists()).toBe(true)
  })
})

describe('BiWidgetEditor scatter', () => {
  it('shows X / Y / size / series instead of metrics and grouping', () => {
    const wrapper = mountEditor({ widget: widget({ type: 'scatter', metrics: [], groupBy: undefined, x: 'amount', y: 'qty' }) })
    for (const key of ['x', 'y', 'size', 'series']) expect(wrapper.find(`[data-field="${key}"]`).exists(), key).toBe(true)
    expect(wrapper.find('[data-action="add-metric"]').exists()).toBe(false)
    expect(wrapper.find('[data-field="group"]').exists()).toBe(false)
  })

  it('saves a valid scatter with no metrics', async () => {
    const wrapper = mountEditor({ widget: widget({ type: 'scatter', metrics: [], groupBy: undefined, x: 'amount', y: 'qty', size: 'qty' }) })
    await wrapper.get('.sb-ok').trigger('click')
    const out = saved(wrapper)!
    expect(out).toMatchObject({ type: 'scatter', x: 'amount', y: 'qty', size: 'qty', metrics: [] })
    expect(Object.keys(out)).not.toContain('groupBy')
  })

  it('a scatter without both axes cannot be saved', async () => {
    const wrapper = mountEditor({ widget: widget({ type: 'scatter', metrics: [], groupBy: undefined, x: 'amount' }) })
    await flushPromises()
    expect(wrapper.get('[data-testid="editor-problem"]').text()).toContain('散点图需要同时选择 X 与 Y')
    await wrapper.get('.sb-ok').trigger('click')
    expect(wrapper.emitted('save')).toBeUndefined()
  })

  it('switching to scatter clears aggregate settings; switching back restores a count metric', async () => {
    const wrapper = mountEditor({ widget: widget({ type: 'auto', metrics: ['sum:amount'], groupBy: 'status' }) })
    await typeBtn(wrapper, 'scatter').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-field="x"]').exists()).toBe(true)
    expect(wrapper.find('[data-action="add-metric"]').exists()).toBe(false)
    await typeBtn(wrapper, 'table').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-testid="metric-row"]')).toHaveLength(1)
    expect(wrapper.find('[data-field="x"]').exists()).toBe(false)
  })
})

describe('BiWidgetEditor grouping', () => {
  it('shows the time bucket only for date fields and the bin step only for number fields', () => {
    const date = mountEditor({ widget: widget({ type: 'line', groupBy: 'placed:week' }) })
    expect(date.find('[data-field="unit"]').exists()).toBe(true)
    expect(date.find('[data-field="bin"]').exists()).toBe(false)
    const num = mountEditor({ widget: widget({ type: 'column', groupBy: 'amount:bin=25' }) })
    expect(num.find('[data-field="bin"]').exists()).toBe(true)
    expect((num.get('[data-field="bin"]').element as HTMLInputElement).value).toBe('25')
    expect(num.find('[data-field="unit"]').exists()).toBe(false)
    const cat = mountEditor({ widget: widget({ type: 'bar', groupBy: 'title' }) })
    expect(cat.find('[data-field="unit"]').exists()).toBe(false)
    expect(cat.find('[data-field="bin"]').exists()).toBe(false)
  })

  it('editing the bin step rewrites group_by; an invalid step is kept visible and blocks saving', async () => {
    const wrapper = mountEditor({ widget: widget({ type: 'column', groupBy: 'amount:bin=10' }) })
    await wrapper.get('[data-field="bin"]').setValue('2.5')
    await wrapper.get('.sb-ok').trigger('click')
    expect(saved(wrapper)).toMatchObject({ groupBy: 'amount:bin=2.5' })

    const bad = mountEditor({ widget: widget({ type: 'column', groupBy: 'amount:bin=10' }) })
    await bad.get('[data-field="bin"]').setValue('0')
    await flushPromises()
    expect(bad.get('[data-testid="editor-problem"]').text()).toContain('分组维度格式不正确')
    await bad.get('.sb-ok').trigger('click')
    expect(bad.emitted('save')).toBeUndefined()
  })

  it('has no series choice until a group-by field is picked', () => {
    const wrapper = mountEditor({ widget: widget({ type: 'table', groupBy: undefined }) })
    expect(wrapper.get('[data-field="series"]').attributes('disabled')).toBeDefined()
  })

  it('offers no json fields for grouping', () => {
    const wrapper = mountEditor({ widget: widget() })
    expect(wrapper.html()).not.toContain('payload')
  })
})

describe('BiWidgetEditor reopening', () => {
  it('reloads the draft each time it opens, so a cancelled edit does not leak', async () => {
    const wrapper = mountEditor({ widget: widget(), open: false })
    await wrapper.setProps({ open: true })
    await root(wrapper).get('[data-field="title"]').setValue('half edited')
    await wrapper.setProps({ open: false })
    await wrapper.setProps({ open: true })
    expect((root(wrapper).get('[data-field="title"]').element as HTMLInputElement).value).toBe('状态占比')
  })

  it('opening for a new widget after editing starts blank', async () => {
    const wrapper = mountEditor({ widget: widget(), open: true })
    await wrapper.setProps({ open: false })
    await wrapper.setProps({ widget: null })
    await wrapper.setProps({ open: true })
    expect((root(wrapper).get('[data-field="title"]').element as HTMLInputElement).value).toBe('')
    expect(wrapper.get('.sb-modal').attributes('data-title')).toBe('添加图表')
  })
})

describe('BiWidgetEditor sort', () => {
  it('shows a sort control only when the chart aggregates, and a direction once a column is chosen', async () => {
    const grouped = mountEditor({ widget: { id: 'w1', title: 'T', type: 'column', metrics: ['count'], groupBy: 'status', span: 6 } })
    expect(grouped.find('[data-field="sort"]').exists()).toBe(true)
    // 没有选择排序列时不显示方向选择器。
    expect(grouped.find('[data-field="sort-dir"]').exists()).toBe(false)
    const sorted = mountEditor({ widget: { id: 'w2', title: 'T', type: 'bar', metrics: ['count'], groupBy: 'status', sort: '-count', span: 6 } })
    expect(sorted.find('[data-field="sort-dir"]').exists()).toBe(true)

    // 散点不聚合，没有排序概念。
    const scatter = mountEditor({ widget: { id: 'w3', title: 'S', type: 'scatter', metrics: [], x: 'amount', y: 'qty', span: 6 } })
    expect(scatter.find('[data-field="sort"]').exists()).toBe(false)
  })

  it('drops a sort that points at a column the response no longer contains', async () => {
    // 度量被删掉后留下的陈旧 sort：打开时可保存，但保存结果里不能再带着它（否则请求必然 400）。
    const wrapper = mountEditor({ widget: { id: 'w1', title: 'T', type: 'column', metrics: ['count'], groupBy: 'status', sort: '-sum_amount', span: 6 } })
    expect(wrapper.get('.sb-ok').attributes('disabled')).toBeUndefined()
    await wrapper.get('.sb-ok').trigger('click')
    const saved = wrapper.emitted('save')![0][0] as BiWidget
    expect(saved.sort).toBeUndefined()
  })
})
