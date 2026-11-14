import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import * as echartsCore from 'echarts/core'

import TrendChart from '@/components/TrendChart.vue'

const points = [
  { date: '01-01', requests: 10, errors: 1 },
  { date: '01-02', requests: 0, errors: 0 },
  { date: '01-03', requests: 25, errors: 3 }
]

const stubs = {
  ToggleGroup: { template: '<div><slot /></div>' },
  ToggleGroupItem: { template: '<button type="button"><slot /></button>' }
}

function lastChart() {
  const init = vi.mocked(echartsCore.init)
  return init.mock.results[init.mock.results.length - 1]?.value as {
    setOption: ReturnType<typeof vi.fn>
    dispose: ReturnType<typeof vi.fn>
  }
}

describe('TrendChart (echarts)', () => {
  beforeEach(() => {
    vi.mocked(echartsCore.init).mockClear()
  })

  it('initializes echarts and sets bar option', async () => {
    const w = mount(TrendChart, {
      props: { points, mode: 'bar' },
      global: { stubs }
    })
    await flushPromises()
    expect(echartsCore.init).toHaveBeenCalled()
    expect(w.find('.trend-canvas').exists()).toBe(true)
    expect(w.find('[role="img"]').exists()).toBe(true)
    const inst = lastChart()
    expect(inst.setOption).toHaveBeenCalled()
    const opt = inst.setOption.mock.calls[0][0] as {
      series: { type: string; name: string }[]
      xAxis: { data: string[] }
    }
    expect(opt.series[0].type).toBe('bar')
    expect(opt.series[0].name).toBe('请求')
    expect(opt.series[1].name).toBe('错误')
    expect(opt.xAxis.data).toEqual(['01-01', '01-02', '01-03'])
  })

  it('uses a taller canvas, capped bar width and a fading area under the request line', async () => {
    const w = mount(TrendChart, { props: { points, mode: 'bar' }, global: { stubs } })
    await flushPromises()
    expect(w.get('[role="img"]').classes()).toContain('h-72')
    const bar = lastChart().setOption.mock.calls[0][0] as { series: { barMaxWidth: number }[] }
    expect(bar.series.map((s) => s.barMaxWidth)).toEqual([28, 28])
    await w.setProps({ mode: 'line' })
    const calls = lastChart().setOption.mock.calls
    const line = calls[calls.length - 1][0] as { series: { yAxisIndex?: number; areaStyle?: { color: { type: string } } }[]; yAxis: { axisLabel: { formatter: (v: number) => string } }[] }
    expect(line.series[1].yAxisIndex).toBe(1)
    expect(line.series[0].areaStyle?.color.type).toBe('linear')
    expect(line.series[1].areaStyle).toBeUndefined()
    expect(line.yAxis[0].axisLabel.formatter(1200)).toBe('1.2k')
    expect(line.yAxis[0].axisLabel.formatter(2500000)).toBe('2.5M')
    expect(line.yAxis[0].axisLabel.formatter(42)).toBe('42')
  })

  it('hides the bar/line switch for read-only previews but keeps the legend', async () => {
    const w = mount(TrendChart, {
      props: { points, hideControls: true },
      global: { stubs: { ToggleGroup: { template: '<div class="trend-mode-switch" />' } } }
    })
    await flushPromises()
    expect(w.find('.trend-mode-switch').exists()).toBe(false)
    expect(w.find('.trend-legend').exists()).toBe(true)
  })

  it('draws both series as solid lines with the same marker and no area noise', async () => {
    mount(TrendChart, { props: { points, mode: 'line' }, global: { stubs } })
    await flushPromises()
    const opt = lastChart().setOption.mock.calls[0][0] as {
      series: { symbol: string; lineStyle: { type?: string } }[]
    }
    expect(opt.series.map((s) => s.symbol)).toEqual(['circle', 'circle'])
    expect(opt.series.every((s) => s.lineStyle.type === undefined)).toBe(true)
  })

  it('defaults to line mode when mode is not provided', async () => {
    mount(TrendChart, { props: { points }, global: { stubs } })
    await flushPromises()
    const opt = lastChart().setOption.mock.calls[0][0] as { series: { type: string }[] }
    expect(opt.series[0].type).toBe('line')
    expect(opt.series[1].type).toBe('line')
  })

  it('switches to line mode and emits update:mode', async () => {
    const w = mount(TrendChart, {
      props: { points, mode: 'bar' },
      global: {
        stubs: {
          ToggleGroup: {
            template: `<div><slot /><button type="button" class="to-line" @click="$emit('update:model-value', 'line')">line</button></div>`
          },
          ToggleGroupItem: { template: '<button type="button"><slot /></button>' }
        }
      }
    })
    await flushPromises()
    await w.get('.to-line').trigger('click')
    await flushPromises()
    expect(w.emitted('update:mode')?.[0]).toEqual(['line'])
  })

  it('updates theme colors when the root theme class changes and exposes a trend summary', async () => {
    document.documentElement.classList.remove('dark')
    const w = mount(TrendChart, { props: { points, mode: 'bar' }, global: { stubs } })
    await flushPromises()
    const inst = lastChart()
    const before = inst.setOption.mock.calls.length
    expect(w.get('[role="img"]').attributes('aria-label')).toContain('请求总计 35，错误总计 4')
    document.documentElement.classList.add('dark')
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(inst.setOption.mock.calls.length).toBeGreaterThan(before)
    w.unmount()
    document.documentElement.classList.remove('dark')
  })

  it('re-renders when points change and disposes on unmount', async () => {
    const w = mount(TrendChart, {
      props: { points, mode: 'line' },
      global: { stubs }
    })
    await flushPromises()
    const inst = lastChart()
    const before = inst.setOption.mock.calls.length
    await w.setProps({
      points: [{ date: '02-01', requests: 5, errors: 0 }]
    })
    expect(inst.setOption.mock.calls.length).toBeGreaterThan(before)
    w.unmount()
    expect(inst.dispose).toHaveBeenCalled()
  })
})
