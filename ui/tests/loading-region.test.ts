import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import SbBlockSkeleton from '@/components/SbBlockSkeleton.vue'
import SbTableSkeleton from '@/components/SbTableSkeleton.vue'

function mountRegion(props: Record<string, unknown>, slot = '') {
  const Host = defineComponent({
    components: { SbAsyncRegion },
    setup() {
      return { props }
    },
    template:
      props.as === 'tbody'
        ? `<table><SbAsyncRegion v-bind="props">${slot}</SbAsyncRegion></table>`
        : `<SbAsyncRegion v-bind="props">${slot}</SbAsyncRegion>`
  })
  return mount(Host)
}

describe('SbTableSkeleton', () => {
  it('renders one muted bar per cell and hides them from assistive tech', () => {
    const wrapper = mount(SbTableSkeleton, { props: { columns: 3, rows: 2 } })
    expect(wrapper.findAll('[data-slot="skeleton"]')).toHaveLength(6)
    expect(wrapper.find('[data-slot="skeleton"]').classes()).toContain('bg-muted')
    expect(wrapper.find('tr').attributes('aria-hidden')).toBe('true')
    expect(wrapper.text()).not.toContain('暂无数据')
  })

  it('reserves row height without painting a pulse while ghost', () => {
    const wrapper = mount(SbTableSkeleton, { props: { columns: 1, rows: 1, ghost: true } })
    expect(wrapper.find('tr').classes()).toContain('invisible')
    expect(wrapper.find('tr').classes()).toContain('h-10')
  })
})

describe('SbBlockSkeleton', () => {
  it('renders lines, cards, a chart, and a spinner', () => {
    const lines = mount(SbBlockSkeleton, { props: { variant: 'lines', count: 2 } })
    expect(lines.findAll('[data-slot="skeleton"]')).toHaveLength(2)

    const cards = mount(SbBlockSkeleton, { props: { variant: 'cards', count: 3 } })
    expect(cards.findAll('[data-slot="skeleton"]')).toHaveLength(3)
    expect(cards.find('[data-slot="skeleton"]').classes()).toContain('h-24')

    const chart = mount(SbBlockSkeleton, { props: { variant: 'chart' } })
    expect(chart.find('[data-slot="skeleton"]').classes()).toContain('h-44')

    const spinner = mount(SbBlockSkeleton, { props: { variant: 'spinner' } })
    expect(spinner.find('[role="status"]').attributes('aria-label')).toBe('正在加载')
    expect(spinner.find('[aria-hidden="true"]').classes()).toContain('min-h-28')
  })

  it('uses the default line count', () => {
    const wrapper = mount(SbBlockSkeleton)
    expect(wrapper.findAll('[data-slot="skeleton"]').length).toBeGreaterThanOrEqual(4)
  })
})

describe('SbAsyncRegion', () => {
  it('shows a skeleton and a busy label before any empty copy', () => {
    const wrapper = mountRegion({ as: 'tbody', showSkeleton: true, columns: 2, rows: 2 })
    const region = wrapper.find('[data-slot="table-body"]')
    expect(region.attributes('aria-busy')).toBe('true')
    expect(wrapper.findAll('[data-slot="skeleton"]')).toHaveLength(4)
    expect(wrapper.find('tr[aria-hidden="true"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('正在加载')
    expect(wrapper.text()).not.toContain('暂无数据')
  })

  it('shows the empty description only in the empty state', () => {
    const wrapper = mountRegion({
      as: 'tbody',
      showEmpty: true,
      columns: 2,
      emptyDescription: '没有行'
    })
    expect(wrapper.text()).toContain('没有行')
    expect(wrapper.findAll('[data-slot="skeleton"]')).toHaveLength(0)
    expect(wrapper.find('[data-slot="table-body"]').attributes('aria-busy')).toBe('false')
  })

  it('shows an error alert and emits retry', async () => {
    const wrapper = mountRegion({ as: 'tbody', showError: true, columns: 2, error: 'load fail' })
    expect(wrapper.text()).toContain('加载失败')
    expect(wrapper.text()).toContain('load fail')
    expect(wrapper.text()).toContain('重试')
    expect(wrapper.text()).not.toContain('暂无数据')
    await wrapper.find('button').trigger('click')
    expect(wrapper.findComponent(SbAsyncRegion).emitted('retry')).toHaveLength(1)
  })

  it('keeps existing rows and marks the region busy while refreshing', () => {
    const wrapper = mountRegion(
      { refreshing: true, pending: true },
      '<span class="row">已有行</span>'
    )
    const region = wrapper.find('[data-slot="async-region"]')
    expect(region.attributes('aria-busy')).toBe('true')
    expect(region.classes()).toContain('opacity-80')
    expect(wrapper.text()).toContain('已有行')
    expect(wrapper.text()).toContain('正在加载')
    expect(wrapper.findAll('[data-slot="skeleton"]')).toHaveLength(0)
  })

  it('reserves layout with an invisible placeholder during the quiet wait', () => {
    const wrapper = mountRegion({ pending: true, block: 'lines' })
    expect(wrapper.find('[data-slot="async-region"]').attributes('aria-busy')).toBe('true')
    expect(wrapper.find('[aria-hidden="true"]').classes()).toContain('invisible')
    expect(wrapper.findAll('[data-slot="skeleton"]').length).toBeGreaterThan(0)
    expect(wrapper.text()).not.toContain('暂无数据')
  })

  it('renders a block error outside a table and a custom empty slot', async () => {
    const errorHost = mountRegion({ showError: true, error: '' })
    expect(errorHost.text()).toContain('加载失败')
    await errorHost.find('button').trigger('click')
    expect(errorHost.findComponent(SbAsyncRegion).emitted('retry')).toHaveLength(1)

    const Host = defineComponent({
      components: { SbAsyncRegion },
      template: `
        <SbAsyncRegion :show-empty="true" empty-title="空标题" empty-action-text="去创建">
          <template #empty>
            <p class="custom-empty">自定义空</p>
          </template>
        </SbAsyncRegion>
      `
    })
    const empty = mount(Host)
    expect(empty.text()).toContain('自定义空')
    expect(empty.text()).not.toContain('空标题')
  })

  it('uses the default empty action and a custom table skeleton slot', async () => {
    const Host = defineComponent({
      components: { SbAsyncRegion },
      template: `
        <div>
          <SbAsyncRegion :show-empty="true" empty-description="没有内容" empty-action-text="新建" />
          <table>
            <SbAsyncRegion as="tbody" :show-skeleton="true" :columns="1">
              <template #skeleton>
                <tr class="custom-sk"><td>骨架槽</td></tr>
              </template>
            </SbAsyncRegion>
          </table>
        </div>
      `
    })
    const wrapper = mount(Host)
    expect(wrapper.text()).toContain('没有内容')
    expect(wrapper.text()).toContain('新建')
    await wrapper.find('button').trigger('click')
    expect(wrapper.findComponent(SbAsyncRegion).emitted('empty-action')).toHaveLength(1)
    expect(wrapper.find('.custom-sk').exists()).toBe(true)
  })
})
