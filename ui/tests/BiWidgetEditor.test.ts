import { mount, flushPromises } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import BiWidgetEditor from '@/components/bi/BiWidgetEditor.vue'
import { dialogTitle, inDialog, installJsdomPolyfills, modalOkDisabled, okModal, realDialogStubs, uiStubs } from '@/test/helpers'
import type { BiWidget, CollectionField } from '@/services/types'

const f = (name: string, type: CollectionField['type']): CollectionField => ({
  name, type, required: false, unique: false, options: {}, hidden: false, readonly: false
})
const fields = [f('amount', 'number'), f('qty', 'number'), f('status', 'select'), f('title', 'text'), f('placed', 'date'), f('payload', 'json')]
const widget = (over: Partial<BiWidget> = {}): BiWidget => ({ id: 'w1', title: '状态占比', type: 'pie', metrics: ['count'], groupBy: 'status', span: 6, ...over })

// Select / Switch / Button / Input / Field 与弹窗壳（SbModal → 真实 Dialog）都用真实组件：
// 要验证的是真实 DOM 属性与事件契约。Tooltip 家族沿用共享桩。
installJsdomPolyfills()

const stubs = {
  ...uiStubs,
  ...realDialogStubs,
  Input: false, Field: false, FieldLabel: false, FieldError: false,
  Select: false, SelectTrigger: false, SelectValue: false, SelectContent: false, SelectGroup: false, SelectItem: false,
  Switch: false
}

/** 真实 Dialog 的内容在下一个 tick 才挂到 body：挂载后等一次，调用方即可同步断言。 */
async function mountEditor(props: { widget?: BiWidget | null; open?: boolean } = {}) {
  const wrapper = mount(BiWidgetEditor, {
    attachTo: document.body,
    props: { open: props.open ?? true, widget: props.widget ?? null, fields },
    global: { stubs }
  })
  await flushPromises()
  return wrapper
}
type Editor = Awaited<ReturnType<typeof mountEditor>>

const root = (_w: Editor) => inDialog().get('[data-testid="bi-editor"]')
const typeBtn = (_w: Editor, type: string) => inDialog().get(`[data-type="${type}"]`)
const saved = (w: Editor) => w.emitted('save')?.[0]?.[0] as BiWidget | undefined

describe('BiWidgetEditor new widget', () => {
  it('starts blank: auto type, a count metric, and cannot be saved without a title', async () => {
    const wrapper = await mountEditor()
    expect(root(wrapper).find('[data-field="title"]').element).toHaveProperty('value', '')
    expect(inDialog().text()).toContain('请填写图表标题')
    // 校验不通过时「保存」是禁用的，用户点不了；这里断言禁用态而不是去点它
    expect(modalOkDisabled()).toBe(true)
    expect(wrapper.emitted('save')).toBeUndefined()
    expect(dialogTitle()).toBe('添加图表')
  })

  it('saves a titled KPI widget with a fresh id and trimmed text', async () => {
    const wrapper = await mountEditor()
    await root(wrapper).get('[data-field="title"]').setValue('  总数  ')
    await okModal()
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
  it('prefills from the widget and shows the edit title', async () => {
    const wrapper = await mountEditor({ widget: widget() })
    expect(dialogTitle()).toBe('编辑图表')
    expect((root(wrapper).get('[data-field="title"]').element as HTMLInputElement).value).toBe('状态占比')
    expect(typeBtn(wrapper, 'pie').attributes('data-state')).toBe('on')
  })

  it('keeps the id and edits the title, filter, limit', async () => {
    const wrapper = await mountEditor({ widget: widget() })
    await root(wrapper).get('[data-field="title"]').setValue('新标题')
    await root(wrapper).get('[data-field="filter"]').setValue('  amount>1  ')
    await root(wrapper).get('[data-field="limit"]').setValue('5')
    await okModal()
    expect(saved(wrapper)).toMatchObject({ id: 'w1', title: '新标题', filter: 'amount>1', limit: 5, groupBy: 'status', type: 'pie' })
  })

  it('does not mutate the original widget until saved', async () => {
    const original = widget()
    const wrapper = await mountEditor({ widget: original })
    await root(wrapper).get('[data-field="title"]').setValue('changed')
    expect(original.title).toBe('状态占比')
  })

  it('an emptied filter or limit is dropped rather than saved as an empty value', async () => {
    const wrapper = await mountEditor({ widget: widget({ filter: 'a=1', limit: 5 }) })
    await root(wrapper).get('[data-field="filter"]').setValue('   ')
    await root(wrapper).get('[data-field="limit"]').setValue('')
    await okModal()
    const out = saved(wrapper)!
    expect(Object.keys(out)).not.toContain('filter')
    expect(Object.keys(out)).not.toContain('limit')
  })
})

describe('BiWidgetEditor metrics', () => {
  it('adds and removes metrics, keeps at least one, caps at six', async () => {
    const wrapper = await mountEditor({ widget: widget({ type: 'table' }) })
    expect(inDialog().findAll('[data-testid="metric-row"]')).toHaveLength(1)
    expect(inDialog().get('[data-remove-metric="0"]').attributes('disabled')).toBeDefined()
    for (let i = 0; i < 8; i++) await inDialog().get('[data-action="add-metric"]').trigger('click')
    expect(inDialog().findAll('[data-testid="metric-row"]')).toHaveLength(6)
    expect(inDialog().get('[data-action="add-metric"]').attributes('disabled')).toBeDefined()
    await inDialog().get('[data-remove-metric="5"]').trigger('click')
    expect(inDialog().findAll('[data-testid="metric-row"]')).toHaveLength(5)
  })

  it('an incomplete metric (kind picked, field missing) blocks saving with a reason', async () => {
    const wrapper = await mountEditor({ widget: widget({ type: 'table', metrics: ['sum:'] }) })
    await flushPromises()
    expect(inDialog().get('[data-testid="editor-problem"]').text()).toContain('度量格式不正确')
    // 校验不通过时「保存」是禁用的，用户点不了；这里断言禁用态而不是去点它
    expect(modalOkDisabled()).toBe(true)
    expect(wrapper.emitted('save')).toBeUndefined()
  })
})

describe('BiWidgetEditor type compatibility', () => {
  it('greys incompatible types and explains the problem for the chosen one', async () => {
    const wrapper = await mountEditor({ widget: widget({ type: 'pie', groupBy: 'placed:day' }) })
    await flushPromises()
    expect(inDialog().get('[data-testid="editor-problem"]').text()).toContain('饼图的分组需要是分类字段')
    expect(typeBtn(wrapper, 'pie').attributes('disabled')).toBeDefined()
    expect(typeBtn(wrapper, 'line').attributes('disabled')).toBeUndefined()
    // 校验不通过时「保存」是禁用的，用户点不了；这里断言禁用态而不是去点它
    expect(modalOkDisabled()).toBe(true)
    expect(wrapper.emitted('save')).toBeUndefined()
    // 改成合法类型后问题消失，可以保存。
    await typeBtn(wrapper, 'line').trigger('click')
    await flushPromises()
    expect(inDialog().find('[data-testid="editor-problem"]').exists()).toBe(false)
    await okModal()
    expect(saved(wrapper)).toMatchObject({ type: 'line', groupBy: 'placed:day' })
  })

  it('shows what auto resolves to', async () => {
    const wrapper = await mountEditor({ widget: widget({ type: 'auto', groupBy: 'placed:day' }) })
    await flushPromises()
    expect(inDialog().get('[data-testid="inferred"]').text()).toBe('已推断为：折线图')
  })
})

describe('BiWidgetEditor style switches', () => {
  it('shows the area switch for lines and writes it', async () => {
    const wrapper = await mountEditor({ widget: widget({ type: 'line', groupBy: 'placed:day' }) })
    const sw = inDialog().get('[data-switch="area"]')
    expect(sw.attributes('data-state')).toBe('unchecked')
    await sw.trigger('click')
    expect(sw.attributes('data-state')).toBe('checked')
    await okModal()
    expect(saved(wrapper)).toMatchObject({ type: 'line', area: true })
  })

  it('shows the donut switch for pies only', async () => {
    const pie = await mountEditor({ widget: widget() })
    expect(inDialog().find('[data-switch="donut"]').exists()).toBe(true)
    expect(inDialog().find('[data-switch="area"]').exists()).toBe(false)
    await inDialog().get('[data-switch="donut"]').trigger('click')
    await okModal()
    expect(saved(pie)).toMatchObject({ donut: true })

    pie.unmount()
    const bar = await mountEditor({ widget: widget({ type: 'bar' }) })
    expect(inDialog().find('[data-switch="donut"]').exists()).toBe(false)
    expect(inDialog().find('[data-switch="area"]').exists()).toBe(false)
  })

  it('an auto widget that resolves to a pie also gets the donut switch', async () => {
    const wrapper = await mountEditor({ widget: widget({ type: 'auto', groupBy: 'status' }) })
    expect(inDialog().find('[data-switch="donut"]').exists()).toBe(true)
  })
})

describe('BiWidgetEditor scatter', () => {
  it('shows X / Y / size / series instead of metrics and grouping', async () => {
    const wrapper = await mountEditor({ widget: widget({ type: 'scatter', metrics: [], groupBy: undefined, x: 'amount', y: 'qty' }) })
    for (const key of ['x', 'y', 'size', 'series']) expect(inDialog().find(`[data-field="${key}"]`).exists(), key).toBe(true)
    expect(inDialog().find('[data-action="add-metric"]').exists()).toBe(false)
    expect(inDialog().find('[data-field="group"]').exists()).toBe(false)
  })

  it('saves a valid scatter with no metrics', async () => {
    const wrapper = await mountEditor({ widget: widget({ type: 'scatter', metrics: [], groupBy: undefined, x: 'amount', y: 'qty', size: 'qty' }) })
    await okModal()
    const out = saved(wrapper)!
    expect(out).toMatchObject({ type: 'scatter', x: 'amount', y: 'qty', size: 'qty', metrics: [] })
    expect(Object.keys(out)).not.toContain('groupBy')
  })

  it('a scatter without both axes cannot be saved', async () => {
    const wrapper = await mountEditor({ widget: widget({ type: 'scatter', metrics: [], groupBy: undefined, x: 'amount' }) })
    await flushPromises()
    expect(inDialog().get('[data-testid="editor-problem"]').text()).toContain('散点图需要同时选择 X 与 Y')
    // 校验不通过时「保存」是禁用的，用户点不了；这里断言禁用态而不是去点它
    expect(modalOkDisabled()).toBe(true)
    expect(wrapper.emitted('save')).toBeUndefined()
  })

  it('switching to scatter clears aggregate settings; switching back restores a count metric', async () => {
    const wrapper = await mountEditor({ widget: widget({ type: 'auto', metrics: ['sum:amount'], groupBy: 'status' }) })
    await typeBtn(wrapper, 'scatter').trigger('click')
    await flushPromises()
    expect(inDialog().find('[data-field="x"]').exists()).toBe(true)
    expect(inDialog().find('[data-action="add-metric"]').exists()).toBe(false)
    await typeBtn(wrapper, 'table').trigger('click')
    await flushPromises()
    expect(inDialog().findAll('[data-testid="metric-row"]')).toHaveLength(1)
    expect(inDialog().find('[data-field="x"]').exists()).toBe(false)
  })
})

describe('BiWidgetEditor grouping', () => {
  it('shows the time bucket only for date fields and the bin step only for number fields', async () => {
    const date = await mountEditor({ widget: widget({ type: 'line', groupBy: 'placed:week' }) })
    expect(inDialog().find('[data-field="unit"]').exists()).toBe(true)
    expect(inDialog().find('[data-field="bin"]').exists()).toBe(false)
    date.unmount()
    const num = await mountEditor({ widget: widget({ type: 'column', groupBy: 'amount:bin=25' }) })
    expect(inDialog().find('[data-field="bin"]').exists()).toBe(true)
    expect((inDialog().get('[data-field="bin"]').element as HTMLInputElement).value).toBe('25')
    expect(inDialog().find('[data-field="unit"]').exists()).toBe(false)
    num.unmount()
    const cat = await mountEditor({ widget: widget({ type: 'bar', groupBy: 'title' }) })
    expect(inDialog().find('[data-field="unit"]').exists()).toBe(false)
    expect(inDialog().find('[data-field="bin"]').exists()).toBe(false)
  })

  it('editing the bin step rewrites group_by; an invalid step is kept visible and blocks saving', async () => {
    const wrapper = await mountEditor({ widget: widget({ type: 'column', groupBy: 'amount:bin=10' }) })
    await inDialog().get('[data-field="bin"]').setValue('2.5')
    await okModal()
    expect(saved(wrapper)).toMatchObject({ groupBy: 'amount:bin=2.5' })

    wrapper.unmount()
    const bad = await mountEditor({ widget: widget({ type: 'column', groupBy: 'amount:bin=10' }) })
    await inDialog().get('[data-field="bin"]').setValue('0')
    await flushPromises()
    expect(inDialog().get('[data-testid="editor-problem"]').text()).toContain('分组维度格式不正确')
    // 校验不通过时「保存」是禁用的，用户点不了；这里断言禁用态而不是去点它
    expect(modalOkDisabled()).toBe(true)
    expect(bad.emitted('save')).toBeUndefined()
  })

  it('has no series choice until a group-by field is picked', async () => {
    const wrapper = await mountEditor({ widget: widget({ type: 'table', groupBy: undefined }) })
    expect(inDialog().get('[data-field="series"]').attributes('disabled')).toBeDefined()
  })

  it('offers no json fields for grouping', async () => {
    const wrapper = await mountEditor({ widget: widget() })
    expect(inDialog().html()).not.toContain('payload')
  })
})

describe('BiWidgetEditor reopening', () => {
  it('reloads the draft each time it opens, so a cancelled edit does not leak', async () => {
    const wrapper = await mountEditor({ widget: widget(), open: false })
    await wrapper.setProps({ open: true })
    await root(wrapper).get('[data-field="title"]').setValue('half edited')
    await wrapper.setProps({ open: false })
    await wrapper.setProps({ open: true })
    expect((root(wrapper).get('[data-field="title"]').element as HTMLInputElement).value).toBe('状态占比')
  })

  it('opening for a new widget after editing starts blank', async () => {
    const wrapper = await mountEditor({ widget: widget(), open: true })
    await wrapper.setProps({ open: false })
    await wrapper.setProps({ widget: null })
    await wrapper.setProps({ open: true })
    expect((root(wrapper).get('[data-field="title"]').element as HTMLInputElement).value).toBe('')
    expect(dialogTitle()).toBe('添加图表')
  })
})

describe('BiWidgetEditor sort', () => {
  it('shows a sort control only when the chart aggregates, and a direction once a column is chosen', async () => {
    const grouped = await mountEditor({ widget: { id: 'w1', title: 'T', type: 'column', metrics: ['count'], groupBy: 'status', span: 6 } })
    expect(inDialog().find('[data-field="sort"]').exists()).toBe(true)
    // 没有选择排序列时不显示方向选择器。
    expect(inDialog().find('[data-field="sort-dir"]').exists()).toBe(false)
    grouped.unmount()
    const sorted = await mountEditor({ widget: { id: 'w2', title: 'T', type: 'bar', metrics: ['count'], groupBy: 'status', sort: '-count', span: 6 } })
    expect(inDialog().find('[data-field="sort-dir"]').exists()).toBe(true)

    // 散点不聚合，没有排序概念。
    sorted.unmount()
    const scatter = await mountEditor({ widget: { id: 'w3', title: 'S', type: 'scatter', metrics: [], x: 'amount', y: 'qty', span: 6 } })
    expect(inDialog().find('[data-field="sort"]').exists()).toBe(false)
  })

  it('drops a sort that points at a column the response no longer contains', async () => {
    // 度量被删掉后留下的陈旧 sort：打开时可保存，但保存结果里不能再带着它（否则请求必然 400）。
    const wrapper = await mountEditor({ widget: { id: 'w1', title: 'T', type: 'column', metrics: ['count'], groupBy: 'status', sort: '-sum_amount', span: 6 } })
    expect(modalOkDisabled()).toBe(false)
    await okModal()
    const saved = wrapper.emitted('save')![0][0] as BiWidget
    expect(saved.sort).toBeUndefined()
  })
})
