// bi-filter.ts — 看板全局条件的合成（planv6.7 §4.2、F5 / F6 / F7）。
//
// 页面级条件 = 时间范围 + 全局 filter 表达式 + 联动 chip。每个 widget 再追加自己的 filter，
// 全部用 `&&` 合并后交给后端 compileFilter。**前端不解析表达式**：只负责转义与拼接，
// 语法与字段校验仍在后端（含 `;` / 长度 / 未知字段）。

export type RangeKey = 'h1' | 'h24' | 'd7' | 'd30' | 'all'

export const RANGE_KEYS: RangeKey[] = ['h1', 'h24', 'd7', 'd30', 'all']

const RANGE_MS: Record<Exclude<RangeKey, 'all'>, number> = {
  h1: 60 * 60 * 1000,
  h24: 24 * 60 * 60 * 1000,
  d7: 7 * 24 * 60 * 60 * 1000,
  d30: 30 * 24 * 60 * 60 * 1000
}

/** 联动筛选 chip：点击图元产生，可单独移除。 */
export interface FilterChip {
  id: string
  /** 展示文案，如 status=paid。 */
  label: string
  /** 合并进 filter 的表达式片段。 */
  expr: string
}

/** 把字符串转成 filter 语法里的字符串字面量（JSON 风格，转义引号与反斜杠）。 */
export function quote(value: string): string {
  return JSON.stringify(value)
}

/**
 * 时间范围 → 表达式。field 为 created_at / updated_at / 已登记 date 字段。
 * `now` 可注入以便测试；全部时间使用 UTC RFC3339，与后端存储格式一致。
 */
export function rangeExpr(range: RangeKey, field: string, now: Date = new Date()): string {
  if (range === 'all') return ''
  const since = new Date(now.getTime() - RANGE_MS[range]).toISOString()
  return `${field}>=${quote(since)}`
}

/** 合并若干条件片段；空片段忽略，多个片段用 && 连接并各自加括号，避免 || 优先级串味。 */
export function combine(parts: Array<string | undefined>): string {
  const clean = parts.map((p) => (p ?? '').trim()).filter(Boolean)
  if (clean.length === 0) return ''
  if (clean.length === 1) return clean[0]
  return clean.map((p) => `(${p})`).join(' && ')
}

/** 联动 chip 的表达式：数值与布尔直接比较，其余用字符串字面量。 */
export function chipFor(field: string, value: unknown, fieldType?: string): FilterChip | null {
  if (value === null || value === undefined || value === '') return null
  if (!/^[A-Za-z][A-Za-z0-9_]{0,62}$/.test(field)) return null
  let literal: string
  if (fieldType === 'number' && Number.isFinite(Number(value))) literal = String(Number(value))
  else if (fieldType === 'bool' && (value === true || value === 'true' || value === false || value === 'false')) literal = String(value === true || value === 'true')
  else literal = quote(String(value))
  return { id: `${field}=${literal}`, label: `${field}=${String(value)}`, expr: `${field}=${literal}` }
}

/**
 * 时间桶 chip：点击折线 / 柱的某个时间桶，筛选到该桶对应的时间区间。
 * bucket 是后端返回的文本桶（day: 2026-11-16；month: 2026-11；year: 2026；hour: 2026-11-16 09:00）。
 * 无法解析时返回 null（不产生 chip，而不是产生一个错的区间）。
 */
export function bucketChip(field: string, unit: string, bucket: string): FilterChip | null {
  if (!/^[A-Za-z][A-Za-z0-9_]{0,62}$/.test(field)) return null
  const range = bucketRange(unit, bucket)
  if (!range) return null
  const [start, end] = range
  return {
    id: `${field}@${unit}:${bucket}`,
    label: `${field}=${bucket}`,
    expr: `${field}>=${quote(start)} && ${field}<${quote(end)}`
  }
}

/** 桶 → [起, 止) UTC RFC3339。week 以桶标签为周一，向后 7 天。 */
export function bucketRange(unit: string, bucket: string): [string, string] | null {
  const iso = (d: Date) => d.toISOString().replace('.000Z', 'Z')
  if (unit === 'hour') {
    const m = /^(\d{4})-(\d{2})-(\d{2}) (\d{2}):00$/.exec(bucket)
    if (!m) return null
    const start = new Date(Date.UTC(+m[1], +m[2] - 1, +m[3], +m[4]))
    return [iso(start), iso(new Date(start.getTime() + 3600_000))]
  }
  if (unit === 'day' || unit === 'week') {
    const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(bucket)
    if (!m) return null
    const start = new Date(Date.UTC(+m[1], +m[2] - 1, +m[3]))
    if (Number.isNaN(start.getTime())) return null
    return [iso(start), iso(new Date(start.getTime() + (unit === 'week' ? 7 : 1) * 86400_000))]
  }
  if (unit === 'month') {
    const m = /^(\d{4})-(\d{2})$/.exec(bucket)
    if (!m) return null
    return [iso(new Date(Date.UTC(+m[1], +m[2] - 1, 1))), iso(new Date(Date.UTC(+m[1], +m[2], 1)))]
  }
  if (unit === 'year') {
    const m = /^(\d{4})$/.exec(bucket)
    if (!m) return null
    return [iso(new Date(Date.UTC(+m[1], 0, 1))), iso(new Date(Date.UTC(+m[1] + 1, 0, 1)))]
  }
  return null
}

/** 最近使用的 filter：去重、最新在前、最多 5 条。 */
export function pushHistory(history: string[], filter: string, max = 5): string[] {
  const value = filter.trim()
  if (!value) return history
  return [value, ...history.filter((h) => h !== value)].slice(0, max)
}

/** 数值分箱 chip：点击某个分箱，筛选到 [起, 起+步长)。 */
export function binChip(field: string, start: unknown, step: number): FilterChip | null {
  const n = Number(start)
  if (!/^[A-Za-z][A-Za-z0-9_]{0,62}$/.test(field) || !Number.isFinite(n) || !(step > 0)) return null
  return {
    id: `${field}@bin:${n}`,
    label: `${field}∈[${n}, ${n + step})`,
    expr: `${field}>=${n} && ${field}<${n + step}`
  }
}
