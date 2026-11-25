// useBiTheme.ts — 读取现有 CSS 主题 token，并在亮暗色切换时更新。
//
// 与 TrendChart.vue 同一套做法：--chart-1..5 / --border / --muted-foreground 等，
// 监听 <html class> 变化重算，不引入新的色值字面量。

import { onBeforeUnmount, onMounted, ref } from 'vue'
import type { BiTheme } from '@/utils/bi-chart'

const FALLBACK_PALETTE = ['#2563eb', '#dc2626', '#386582', '#8d5b15', '#15803d']

function cssVar(name: string, fallback: string): string {
  if (typeof window === 'undefined') return fallback
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return v || fallback
}

/** 读当前主题。导出供测试直接断言。 */
export function readBiTheme(): BiTheme {
  const palette = [1, 2, 3, 4, 5].map((n, i) => cssVar(`--chart-${n}`, FALLBACK_PALETTE[i]))
  return {
    palette,
    muted: cssVar('--muted-foreground', '#64748b'),
    border: cssVar('--border', '#e2e8f0'),
    card: cssVar('--card', '#ffffff'),
    foreground: cssVar('--foreground', '#1f1e1d'),
    sans: cssVar('--font-sans', 'system-ui, sans-serif'),
    dark: document.documentElement.classList.contains('dark')
  }
}

/** 响应式主题：<html class> 变化（亮暗切换）时刷新。 */
export function useBiTheme() {
  const theme = ref<BiTheme>(readBiTheme())
  let observer: MutationObserver | null = null

  onMounted(() => {
    theme.value = readBiTheme()
    observer = new MutationObserver(() => { theme.value = readBiTheme() })
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
  })
  onBeforeUnmount(() => {
    observer?.disconnect()
    observer = null
  })

  return { theme }
}
