import { i18n, t } from '../locales'

/**
 * 当前界面语言对应的 BCP 47 标签，供 toLocaleString 使用。
 * zh-CN → 'zh-CN'；en-US → 'en-US'。
 */
export function currentBcp47(): string {
  return i18n.global.locale.value === 'zh-CN' ? 'zh-CN' : 'en-US'
}

export function formatBytes(bytes: number): string {
  if (!bytes && bytes !== 0) return '-'
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(Math.floor(Math.log2(bytes) / 10), units.length - 1)
  return `${(bytes / 1024 ** i).toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

/** 数据量展示：undefined → —；数字千分位 + 计数单位（随语言变化） */
export function formatCount(n?: number): string {
  if (n === undefined || n === null || Number.isNaN(n)) return '—'
  return t('common.itemCount', { n: n.toLocaleString(currentBcp47()) })
}

export function formatTime(iso?: string, empty = '-'): string {
  if (!iso) return empty
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString(currentBcp47(), { hour12: false })
}

export function shortTime(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** 错误率展示：后端为 0~1 比值，转为百分数并固定 4 位小数（不含 % 符号）。 */
export function formatErrorRate(ratio: number | null | undefined): string {
  if (ratio == null || !Number.isFinite(ratio)) return (0).toFixed(4)
  return (ratio * 100).toFixed(4)
}

export function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback
}

export function formatJson(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}
