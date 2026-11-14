<template>
  <div class="trend-chart trend-canvas flex flex-col gap-2 overflow-hidden rounded-lg border border-border bg-card px-4 pt-3 pb-2">
    <!-- 图例与切换放进同一张卡片，与绘图区左右对齐；左轴（蓝）= 请求，右轴（红）= 错误 -->
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="trend-legend flex items-center gap-4 text-xs text-muted-foreground">
        <span class="inline-flex items-center gap-1.5">
          <span class="size-2 rounded-sm bg-chart-1" />
          {{ t('components.trendChart.requests') }}<span class="text-muted-foreground/80">{{ t('components.trendChart.leftAxis') }}</span>
        </span>
        <span class="inline-flex items-center gap-1.5">
          <span class="size-2 rounded-sm bg-chart-2" />
          {{ t('components.trendChart.errors') }}<span class="text-muted-foreground/80">{{ t('components.trendChart.rightAxis') }}</span>
        </span>
      </div>
      <ToggleGroup
        v-if="!hideControls"
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

    <div class="relative w-full">
      <div
        ref="el"
        class="h-72 w-full sm:h-[22rem]"
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
    /** 只读展示（如首页预览）时隐藏柱/线切换 */
    hideControls?: boolean
  }>(),
  { mode: 'line', hideControls: false }
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

/** #rrggbb → rgba()；非十六进制色值原样返回（渐变退化为纯色） */
function withAlpha(color: string, alpha: number) {
  const m = /^#([0-9a-f]{6})$/i.exec(color)
  if (!m) return color
  const n = parseInt(m[1], 16)
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`
}

/** 纵轴刻度：1200 → 1.2k，避免长数字挤压绘图区 */
function compact(v: number) {
  if (Math.abs(v) >= 1e6) return `${+(v / 1e6).toFixed(1)}M`
  if (Math.abs(v) >= 1e3) return `${+(v / 1e3).toFixed(1)}k`
  return String(v)
}

function themeColors() {
  return {
    primary: cssVar('--chart-1', '#3b82f6'),
    destructive: cssVar('--chart-2', '#dc2626'),
    muted: cssVar('--muted-foreground', '#64748b'),
    border: cssVar('--border', '#e2e8f0'),
    card: cssVar('--card', '#ffffff'),
    sans: cssVar('--font-sans', 'system-ui, sans-serif'),
  }
}

function buildOption(): EChartsOption {
  const c = themeColors()
  const dates = props.points.map((p) => p.date)
  const req = props.points.map((p) => p.requests)
  const err = props.points.map((p) => p.errors)
  const isBar = props.mode === 'bar'
  const dark = document.documentElement.classList.contains('dark')

  // 点多时柱子自动变细；点少时不超过 28px，避免一两天数据时出现「砖块」
  const barMaxWidth = props.points.length > 20 ? 14 : 28
  const series: SeriesOption[] = isBar
    ? [
        {
          name: t('components.trendChart.requests'),
          type: 'bar',
          data: req,
          barMaxWidth,
          barMinHeight: 0,
          barGap: '12%',
          barCategoryGap: '28%',
          itemStyle: { color: c.primary, borderRadius: [4, 4, 0, 0] },
          emphasis: { itemStyle: { color: withAlpha(c.primary, 0.85) } },
        },
        {
          name: t('components.trendChart.errors'),
          type: 'bar',
          data: err,
          yAxisIndex: 1,
          barMaxWidth,
          barMinHeight: 2,
          itemStyle: { color: c.destructive, borderRadius: [4, 4, 0, 0] },
          emphasis: { itemStyle: { color: withAlpha(c.destructive, 0.85) } },
        },
      ]
    : [
        {
          name: t('components.trendChart.requests'),
          type: 'line',
          data: req,
          smooth: 0.25,
          symbol: 'circle',
          symbolSize: 8,
          // 数据点只在悬停时出现，平时只看线形
          showSymbol: props.points.length <= 1,
          lineStyle: { color: c.primary, width: 2.5 },
          itemStyle: { color: c.primary, borderColor: c.card, borderWidth: 2 },
          areaStyle: {
            color: {
              type: 'linear',
              x: 0,
              y: 0,
              x2: 0,
              y2: 1,
              colorStops: [
                { offset: 0, color: withAlpha(c.primary, dark ? 0.32 : 0.2) },
                { offset: 1, color: withAlpha(c.primary, dark ? 0.04 : 0.02) },
              ],
            },
          },
          emphasis: { focus: 'series' },
          z: 3,
        },
        {
          name: t('components.trendChart.errors'),
          type: 'line',
          data: err,
          yAxisIndex: 1,
          smooth: 0.25,
          symbol: 'circle',
          symbolSize: 8,
          showSymbol: props.points.length <= 1,
          lineStyle: { color: c.destructive, width: 2 },
          itemStyle: { color: c.destructive, borderColor: c.card, borderWidth: 2 },
          emphasis: { focus: 'series' },
          z: 4,
        },
      ]

  return {
    animationDuration: 400,
    animationEasing: 'cubicOut',
    textStyle: { fontFamily: c.sans, color: c.muted, fontSize: 12 },
    tooltip: {
      trigger: 'axis',
      backgroundColor: c.card,
      borderColor: c.border,
      borderWidth: 1,
      padding: [8, 12],
      extraCssText: 'border-radius: 8px; box-shadow: 0 4px 16px rgba(0,0,0,0.08);',
      textStyle: { color: cssVar('--foreground', '#1f1e1d'), fontSize: 12 },
      valueFormatter: (v) => (typeof v === 'number' ? v.toLocaleString() : String(v ?? '')),
      axisPointer: {
        type: isBar ? 'shadow' : 'line',
        shadowStyle: { color: withAlpha(c.primary, 0.06) },
        lineStyle: { color: c.muted, width: 1, type: 'dashed', opacity: 0.6 },
      },
    },
    grid: {
      left: 0,
      right: 0,
      top: 16,
      bottom: 8,
      containLabel: true,
    },
    xAxis: {
      type: 'category',
      data: dates,
      boundaryGap: true,
      axisLine: { lineStyle: { color: c.border, width: 1 } },
      axisTick: { show: false },
      axisLabel: { color: c.muted, fontSize: 12, margin: 12, hideOverlap: true },
    },
    // 左轴：请求；右轴：错误（量级常差两个数量级，共用一根轴时错误会贴在 0 线上）
    yAxis: [
      {
        type: 'value',
        minInterval: 1,
        splitNumber: 4,
        splitLine: { lineStyle: { color: c.border, type: [4, 4], width: 1 } },
        axisLine: { show: false },
        axisTick: { show: false },
        axisLabel: { color: c.primary, fontSize: 12, margin: 12, formatter: compact },
      },
      {
        type: 'value',
        minInterval: 1,
        splitNumber: 4,
        splitLine: { show: false },
        axisLine: { show: false },
        axisTick: { show: false },
        axisLabel: { color: c.destructive, fontSize: 12, margin: 12, formatter: compact },
      },
    ],
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
