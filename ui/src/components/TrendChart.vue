<template>
  <div class="trend-chart flex flex-col gap-3">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="trend-legend flex items-center gap-4 text-xs text-muted-foreground">
        <span class="inline-flex items-center gap-1.5">
          <span class="size-2.5 rounded-sm bg-chart-1" />
          {{ t('components.trendChart.requests') }}
        </span>
        <span class="inline-flex items-center gap-1.5">
          <span class="inline-flex size-2.5 rotate-45 items-center justify-center border border-destructive bg-destructive" />
          {{ t('components.trendChart.errors') }}
        </span>
      </div>
      <ToggleGroup
        type="single"
        :model-value="mode"
        variant="outline"
        size="sm"
        class="trend-mode-switch h-8"
        @update:model-value="(v: string | undefined) => v && emit('update:mode', v as ChartMode)"
      >
        <ToggleGroupItem value="bar" class="h-8 gap-1 px-2.5 text-xs">
          <BarChart3Icon class="size-3.5" />
          {{ t('components.trendChart.bar') }}
        </ToggleGroupItem>
        <ToggleGroupItem value="line" class="h-8 gap-1 px-2.5 text-xs">
          <TrendingUpIcon class="size-3.5" />
          {{ t('components.trendChart.line') }}
        </ToggleGroupItem>
      </ToggleGroup>
    </div>

    <div class="trend-canvas relative w-full overflow-hidden rounded-lg border border-border/60 bg-card/40">
      <div
        ref="el"
        class="h-[240px] w-full"
        role="img"
        :aria-label="t('components.trendChart.ariaLabel', {
          mode: mode === 'bar' ? t('components.trendChart.bar') : t('components.trendChart.line'),
          requests: totals.requests,
          errors: totals.errors
        })"
      />
    </div>
  </div>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { BarChart3Icon, TrendingUpIcon } from '@lucide/vue'
import * as echarts from 'echarts/core'
import { BarChart, LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { ECharts, EChartsOption, SeriesOption } from 'echarts'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import type { TrendPoint } from '../services/types'

echarts.use([BarChart, LineChart, GridComponent, TooltipComponent, CanvasRenderer])

export type ChartMode = 'bar' | 'line'

const props = withDefaults(
  defineProps<{
    points: TrendPoint[]
    mode?: ChartMode
  }>(),
  { mode: 'line' }
)

const emit = defineEmits<{ 'update:mode': [mode: ChartMode] }>()

const { t } = useI18n()

const totals = computed(() => props.points.reduce((sum, point) => ({
  requests: sum.requests + point.requests,
  errors: sum.errors + point.errors
}), { requests: 0, errors: 0 }))

const el = ref<HTMLElement | null>(null)
let chart: ECharts | null = null
let ro: ResizeObserver | null = null
let themeObserver: MutationObserver | null = null

/** 读 CSS 变量，保持与现有 UI 同一套 token */
function cssVar(name: string, fallback: string) {
  /* v8 ignore next 4 -- jsdom/SSR 无 getComputedStyle 变量时走 fallback */
  if (typeof window === 'undefined') return fallback
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return v || fallback
}

function themeColors() {
  return {
    primary: cssVar('--chart-1', '#3b82f6'),
    destructive: cssVar('--chart-2', '#dc2626'),
    muted: cssVar('--muted-foreground', '#64748b'),
    border: cssVar('--border', '#e2e8f0'),
    card: cssVar('--card', '#ffffff'),
    font: cssVar('--font-sans', 'system-ui, sans-serif'),
  }
}

function buildOption(): EChartsOption {
  const c = themeColors()
  const dates = props.points.map((p) => p.date)
  const req = props.points.map((p) => p.requests)
  const err = props.points.map((p) => p.errors)
  const isBar = props.mode === 'bar'

  const series: SeriesOption[] = isBar
    ? [
        {
          name: t('components.trendChart.requests'),
          type: 'bar',
          data: req,
          itemStyle: { color: c.primary, borderRadius: [3, 3, 0, 0] },
          barMaxWidth: 16,
          barGap: '20%',
        },
        {
          name: t('components.trendChart.errors'),
          type: 'bar',
          data: err,
          itemStyle: { color: c.destructive, borderRadius: [3, 3, 0, 0] },
          barMaxWidth: 16,
        },
      ]
    : [
        {
          name: t('components.trendChart.requests'),
          type: 'line',
          data: req,
          smooth: true,
          symbol: 'circle',
          symbolSize: 7,
          lineStyle: { color: c.primary, width: 2 },
          itemStyle: { color: c.primary, borderColor: c.card, borderWidth: 1.5 },
          areaStyle: { color: c.primary, opacity: 0.08 },
        },
        {
          name: t('components.trendChart.errors'),
          type: 'line',
          data: err,
          smooth: true,
          symbol: 'diamond',
          symbolSize: 8,
          lineStyle: { color: c.destructive, width: 2, type: 'dashed' },
          itemStyle: { color: c.destructive, borderColor: c.card, borderWidth: 1.5 },
        },
      ]

  return {
    animationDuration: 280,
    textStyle: { fontFamily: c.font, color: c.muted, fontSize: 11 },
    tooltip: {
      trigger: 'axis',
      backgroundColor: c.card,
      borderColor: c.border,
      borderWidth: 1,
      textStyle: { color: cssVar('--foreground', '#1f1e1d'), fontSize: 12 },
      axisPointer: {
        type: isBar ? 'shadow' : 'line',
        shadowStyle: { color: c.primary, opacity: 0.06 },
        lineStyle: { color: c.border, width: 1 },
      },
    },
    grid: {
      left: 44,
      right: 16,
      top: 14,
      bottom: 32,
      containLabel: false,
    },
    xAxis: {
      type: 'category',
      data: dates,
      axisLine: { lineStyle: { color: c.border, width: 1.2 } },
      axisTick: { lineStyle: { color: c.border }, length: 4 },
      axisLabel: { color: c.muted, fontSize: 10, margin: 10 },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      splitLine: { lineStyle: { color: c.border, type: 'dashed', opacity: 0.7 } },
      axisLine: { show: true, lineStyle: { color: c.border, width: 1.2 } },
      axisTick: { show: false },
      axisLabel: { color: c.muted, fontSize: 10 },
    },
    series,
  }
}

function render() {
  if (!chart) return
  chart.setOption(buildOption(), true)
}

function init() {
  if (!el.value) return
  chart = echarts.init(el.value, undefined, { renderer: 'canvas' })
  render()
  ro = new ResizeObserver(() => chart?.resize())
  ro.observe(el.value)
  themeObserver = new MutationObserver(render)
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
}

watch(() => props.points, render, { deep: true })
watch(() => props.mode, render)

onMounted(init)
onBeforeUnmount(() => {
  themeObserver?.disconnect()
  themeObserver = null
  ro?.disconnect()
  ro = null
  chart?.dispose()
  chart = null
})
</script>
