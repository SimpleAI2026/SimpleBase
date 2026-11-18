// bi-auto.ts — 「自动」图表类型推断（planv6.7 §4.3.1）。
//
// 纯函数。按从上到下第一条命中的规则返回具体类型；返回 'kpi' 表示应渲染为 KPI 指标卡
// （KPI 不在类型选择器里，见计划 §4.3）。类目数来自上一次聚合结果，首次请求前传 undefined，
// 此时按字段类型预判，结果回来后再用真实类目数重新解析（不重复请求）。

import type { BiChartType, BiWidget, CollectionField } from '@/services/types'
import { dimensionKind, hasDateMetric, isAdditiveMetrics, parseGroupBy } from './bi-compat'

export type ResolvedType = Exclude<BiChartType, 'auto'> | 'kpi'

/** 规则 7 / 8 的边界：饼图最多 6 类，柱状图最多 12 类且标签平均 ≤ 8 个字符。 */
export const PIE_MAX_CATEGORIES = 6
export const COLUMN_MAX_CATEGORIES = 12
export const COLUMN_MAX_LABEL = 8

export interface AutoHints {
  /** 上一次聚合结果的类目数；首次请求前为 undefined。 */
  categoryCount?: number
  /** 上一次聚合结果里 x 标签的平均长度。 */
  avgLabelLength?: number
}

function isPiePreferred(field: CollectionField | undefined): boolean {
  return field?.type === 'select' || field?.type === 'bool'
}

/** 解析「自动」。fields 为 name → field（含内置时间列）。 */
export function resolveAutoType(
  w: BiWidget,
  fields: Map<string, CollectionField>,
  hints: AutoHints = {}
): ResolvedType {
  // 1. 两个 number 字段做 X / Y，且没选度量。
  if (w.x && w.y && w.metrics.length === 0) return 'scatter'

  const kind = dimensionKind(w.groupBy)
  // 2. 无维度、仅 1 个度量。
  if (kind === 'none' && w.metrics.length === 1) return 'kpi'
  // 10（前置）：度量数 > 3，或无维度却有多个度量，都装不进图。
  if (w.metrics.length > 3 || (kind === 'none' && w.metrics.length !== 1)) return 'table'

  // 日期的 min / max 是日期值，画不成数值图：有维度时只能用表格。
  if (hasDateMetric(w.metrics, fields)) return 'table'
  // 3. 时间桶。系列 × 多度量折线画不全，退到表格。
  if (kind === 'time') return w.series && w.metrics.length > 1 ? 'table' : 'line'
  // 4. 数值分箱。
  if (kind === 'bin') return w.metrics.length === 1 && !w.series ? 'column' : 'table'

  // 此后主维度必为分类。
  const hasSeries = Boolean(w.series)
  // 5. 分类 + series + 1 个可累加度量。
  if (hasSeries && w.metrics.length === 1 && isAdditiveMetrics(w.metrics)) return 'stacked_bar'
  // 6. 分类 + series + 1 个不可累加度量，或分类 + 2–3 个度量。
  if (hasSeries && w.metrics.length === 1) return 'grouped_bar'
  if (!hasSeries && w.metrics.length >= 2 && w.metrics.length <= 3) return 'grouped_bar'
  // series 与多度量同时出现：无法同时表达，退到表格。
  if (hasSeries) return 'table'

  // 7–9：分类 + 1 个度量。
  const group = parseGroupBy(w.groupBy)
  const dimension = group ? fields.get(group.field) : undefined
  const count = hints.categoryCount
  const label = hints.avgLabelLength
  if (isPiePreferred(dimension) && (count === undefined || count <= PIE_MAX_CATEGORIES)) return 'pie'
  if (count !== undefined && count <= COLUMN_MAX_CATEGORIES && (label ?? 0) <= COLUMN_MAX_LABEL) return 'column'
  return 'bar'
}

/** 从一次聚合结果推导 hints（x 列的类目数与平均标签长度）。 */
export function hintsFromRows(columns: string[], rows: unknown[][]): AutoHints {
  const idx = columns.indexOf('x')
  if (idx < 0 || rows.length === 0) return {}
  const labels = new Set<string>()
  let total = 0
  for (const row of rows) {
    const text = row[idx] === null || row[idx] === undefined ? '' : String(row[idx])
    labels.add(text)
    total += [...text].length
  }
  return { categoryCount: labels.size, avgLabelLength: total / rows.length }
}
