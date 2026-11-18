// bi-agent-block.ts — 助手输出的 `<!-- sb:chart {...} -->` 块解析与校验（planv6.7 §3.4 / §4.7）。
//
// 对标 starter-delivery.ts：**前端只信结构**。
//  - 块是 HTML 注释，Markdown 不渲染，不会出现在正文里；
//  - 字段名必须在 schema 内，类型必须兼容当前配置（与类型选择器共用 bi-compat）；
//  - title 一律当纯文本，调用方用 {{ }} 渲染，不用 v-html；
//  - filter 前端不解析，原样交后端 compileFilter 校验；
//  - id / span 由前端分配，块里给了也忽略；未知 key 丢弃；超长 / 非法 JSON 的块跳过。

import type { BiChartType, BiSpan, BiWidget, CollectionField } from '@/services/types'
import { ALL_TYPES, AGGREGATE_MAX_LIMIT, POINTS_MAX_LIMIT, checkType, fieldMap } from './bi-compat'

const BLOCK_RE = /<!--\s*sb:chart\s+([\s\S]*?)\s*-->/g
/** 单块 JSON 的上限，防止异常大的注释拖慢渲染。 */
export const MAX_BLOCK_CHARS = 4 * 1024
/** 单条消息最多取几块。 */
export const MAX_BLOCKS = 4
/** 整条消息过长时不解析（防御）。 */
const MAX_CONTENT_CHARS = 256 * 1024
export const MAX_TITLE_CHARS = 40
const MAX_FIELD_STRING = 2048

export interface AgentChartBlock {
  version: number
  title: string
  type: string
  metrics: string[]
  groupBy?: string
  series?: string
  x?: string
  y?: string
  size?: string
  filter?: string
  sort?: string
  limit?: number
  area?: boolean
  donut?: boolean
  /**
   * 解析阶段发现的、必须拒绝整块的问题（如 filter 超长）。
   * 绝不能把超长条件悄悄丢掉：那会让「带限制的图」变成「不带限制的图」，统计口径被静默放宽。
   */
  error?: string
}

/** 解析阶段就失败的块：没有可校验的内容，只留下原因，供界面逐项说明。 */
export interface BrokenBlock {
  reason: string
}

export interface ParsedChartBlocks {
  blocks: AgentChartBlock[]
  broken: BrokenBlock[]
}

export type ValidatedWidget =
  | { ok: true; widget: BiWidget }
  | { ok: false; reason: string; title: string }

function str(v: unknown, max = MAX_FIELD_STRING): string | undefined {
  if (typeof v !== 'string') return undefined
  const t = v.trim()
  return t === '' || t.length > max ? undefined : t
}

/** 解析并保留每个坏块的原因；最多取 MAX_BLOCKS 个有效块与 MAX_BLOCKS 条失败说明。 */
export function parseChartBlocksDetailed(content: string): ParsedChartBlocks {
  const out: AgentChartBlock[] = []
  const broken: BrokenBlock[] = []
  if (!content || content.length > MAX_CONTENT_CHARS) return { blocks: out, broken }
  const fail = (reason: string) => {
    if (broken.length < MAX_BLOCKS) broken.push({ reason })
  }
  for (const m of content.matchAll(BLOCK_RE)) {
    if (out.length >= MAX_BLOCKS) break
    const body = m[1]
    if (!body) continue
    if (body.length > MAX_BLOCK_CHARS) {
      fail('block_too_large')
      continue
    }
    let raw: unknown
    try {
      raw = JSON.parse(body)
    } catch {
      fail('bad_json')
      continue
    }
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) {
      fail('block_not_object')
      continue
    }
    const o = raw as Record<string, unknown>
    const metrics = Array.isArray(o.metrics)
      ? o.metrics.filter((x): x is string => typeof x === 'string').map((x) => x.trim()).filter(Boolean).slice(0, 16)
      : []
    const block: AgentChartBlock = {
      version: typeof o.version === 'number' ? o.version : 1,
      title: str(o.title, 400) ?? '',
      type: str(o.type, 32) ?? 'auto',
      metrics
    }
    // 只按白名单 key 取值：未知 key（含 id / span / 布局）天然被丢弃。
    const groupBy = str(o.group_by)
    if (groupBy) block.groupBy = groupBy
    for (const key of ['series', 'x', 'y', 'size', 'filter', 'sort'] as const) {
      const v = str(o[key])
      if (v) block[key] = v
      // 给了值却因超长被丢掉：拒绝整块，而不是当作「没写」。
      else if (typeof o[key] === 'string' && (o[key] as string).trim().length > MAX_FIELD_STRING) block.error = `${key}_too_long`
    }
    if (typeof o.limit === 'number') block.limit = o.limit
    if (o.area === true) block.area = true
    if (o.donut === true) block.donut = true
    out.push(block)
  }
  return { blocks: out, broken }
}

/** 从助手消息里取出至多 MAX_BLOCKS 个图表块；坏块（超长 / 非 JSON / 非对象）跳过。需要原因时用 parseChartBlocksDetailed。 */
export function parseChartBlocks(content: string): AgentChartBlock[] {
  return parseChartBlocksDetailed(content).blocks
}

/** 去掉图表块注释，剩余作为普通 Markdown 展示。 */
export function stripChartBlocks(content: string): string {
  return content.replace(BLOCK_RE, '').trimEnd()
}

/** 栅格宽度：整行放折线与表格，其余半行；饼图与指标卡占三分之一。 */
export function defaultSpan(type: BiChartType, hasSeriesOrManyMetrics: boolean): BiSpan {
  if (type === 'pie') return 4
  if (type === 'line' || type === 'table' || type === 'scatter') return 8
  if (type === 'auto') return 4
  return hasSeriesOrManyMetrics ? 8 : 6
}

/** 把块校验成可加入看板的 BiWidget；id 由调用方传入（前端分配）。 */
export function validateAgentWidget(
  block: AgentChartBlock,
  schemaFields: CollectionField[],
  id: string,
  /** SQL 表没有内置 created_at / updated_at：助手若引用它们必须被拒绝，而不是放行后查询 400。 */
  builtinTime: boolean = true
): ValidatedWidget {
  const title = block.title.replace(/[\u0000-\u001f\u007f]/g, ' ').trim().slice(0, MAX_TITLE_CHARS)
  const fail = (reason: string): ValidatedWidget => ({ ok: false, reason, title: title || '—' })
  if (block.version !== 1) return fail('version_unsupported')
  if (!title) return fail('title_required')
  if (block.error) return fail(block.error)
  if (!(ALL_TYPES as string[]).includes(block.type)) return fail('type_unknown')

  const widget: BiWidget = {
    id,
    title,
    type: block.type as BiChartType,
    metrics: block.metrics,
    span: 6
  }
  if (block.groupBy) widget.groupBy = block.groupBy
  if (block.series) widget.series = block.series
  if (block.x) widget.x = block.x
  if (block.y) widget.y = block.y
  if (block.size) widget.size = block.size
  if (block.filter) widget.filter = block.filter
  if (block.sort) widget.sort = block.sort
  const scatter = Boolean(widget.x && widget.y)
  if (block.limit !== undefined) {
    // 前端把 limit 夹到后端允许范围，而不是拒绝：助手常给出略大的数字。
    const max = scatter ? POINTS_MAX_LIMIT : AGGREGATE_MAX_LIMIT
    if (!Number.isFinite(block.limit)) return fail('limit_out_of_range')
    widget.limit = Math.min(Math.max(Math.trunc(block.limit), 1), max)
  }
  if (block.area) widget.area = true
  if (block.donut) widget.donut = true

  const fields = fieldMap(schemaFields, builtinTime)
  const checked = checkType(widget.type, widget, fields)
  if (!checked.ok) return fail(checked.reason)
  widget.span = defaultSpan(widget.type, Boolean(widget.series) || widget.metrics.length > 1)
  return { ok: true, widget }
}
