<template>
  <div
    ref="el"
    class="w-full"
    :class="heightClass"
    role="img"
    :aria-label="ariaLabel"
  />
</template>

<script setup lang="ts">
// BiChart：数据看板里唯一直接使用 echarts 的组件（planv6.7 §4.5）。
// 只接收 option；主题色由 useBiTheme 提供，亮暗色切换时由调用方重算 option。
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts/core'
import { BarChart, LineChart, PieChart, ScatterChart } from 'echarts/charts'
import {
  DataZoomComponent,
  DatasetComponent,
  GridComponent,
  LegendComponent,
  ToolboxComponent,
  TooltipComponent
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { ECharts, EChartsOption } from 'echarts'

echarts.use([
  BarChart,
  LineChart,
  PieChart,
  ScatterChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DatasetComponent,
  DataZoomComponent,
  ToolboxComponent,
  CanvasRenderer
])

const props = withDefaults(
  defineProps<{
    option: EChartsOption
    ariaLabel?: string
    /** Tailwind 高度类；横向条形图类目多时由调用方传更高的值。 */
    heightClass?: string
  }>(),
  { ariaLabel: '', heightClass: 'h-64 sm:h-72' }
)

const emit = defineEmits<{
  /** 点击图元：name 为类目 / 扇区名，seriesName 为系列名。 */
  select: [payload: { name: string; seriesName: string; value: unknown }]
}>()

const el = ref<HTMLElement | null>(null)
let chart: ECharts | null = null
let ro: ResizeObserver | null = null

function render() {
  chart?.setOption(props.option, true)
}

onMounted(() => {
  if (!el.value) return
  chart = echarts.init(el.value, undefined, { renderer: 'canvas' })
  render()
  chart.on('click', (params: unknown) => {
    const p = params as { name?: unknown; seriesName?: unknown; value?: unknown }
    emit('select', { name: String(p.name ?? ''), seriesName: String(p.seriesName ?? ''), value: p.value })
  })
  ro = new ResizeObserver(() => chart?.resize())
  ro.observe(el.value)
})

watch(() => props.option, render, { deep: true })

onBeforeUnmount(() => {
  ro?.disconnect()
  ro = null
  chart?.dispose()
  chart = null
})
</script>
