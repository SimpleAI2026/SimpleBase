import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import * as echartsCore from 'echarts/core'

import TrendChart from './TrendChart.vue'

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
