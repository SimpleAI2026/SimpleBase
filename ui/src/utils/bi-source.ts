// bi-source.ts — 数据看板的「数据源」抽象（集合 / SQL 表）。
//
// 看板、聚合请求、明细表、Agent 上下文、本地配置 key 只关心「一个有字段的数据源」，
// 不关心它是集合还是 SQL 表。种类相关的差异集中在这里，做成纯函数，不依赖 Vue / i18n：
//   - SQL 列类型 → 看板字段类型（与后端 sqlColumnKind 同一口径）；
//   - SQL 表没有内置 created_at / updated_at，也没有 schema 登记概念；
//   - 数据源的唯一标识（用于本地配置、Agent 会话 key），两种种类互不冲突。

import type { CollectionField, DocumentItem, SchemaColumn, SchemaTableRows } from '@/services/types'

export type BiSourceKind = 'collection' | 'table'

export interface BiSource {
  kind: BiSourceKind
  /** 集合名或 SQL 表名。 */
  name: string
}

/** 本地存储 key 的数据源段：集合保持原样（向后兼容已有配置），表加 `table:` 前缀避免与同名集合冲突。 */
export function sourceKeyPart(source: BiSource): string {
  return source.kind === 'table' ? `table:${source.name}` : source.name
}

/** 下拉选项的值：`c:<name>` / `t:<name>`；名字本身受标识符规则约束，不含冒号。 */
export function encodeSource(source: BiSource): string {
  return `${source.kind === 'table' ? 't' : 'c'}:${source.name}`
}

export function decodeSource(value: string): BiSource | null {
  const m = /^([ct]):(.+)$/.exec(value)
  if (!m) return null
  return { kind: m[1] === 't' ? 'table' : 'collection', name: m[2] }
}

export function sameSource(a: BiSource | null, b: BiSource | null): boolean {
  return a !== null && b !== null && a.kind === b.kind && a.name === b.name
}

/**
 * 引擎列类型 → 看板字段类型。必须与后端 `sqlColumnKind`（internal/api/aggregate_env.go）逐类一致：
 * 前端据此决定「哪些列能当度量 / 维度 / 时间字段」，后端据此编译 SQL，两边口径不同会出现
 * 「前端放行、后端 400」或「后端支持、前端不让选」。
 */
export function sqlColumnKind(dataType: string): CollectionField['type'] {
  let t = dataType.trim().toUpperCase()
  const paren = t.indexOf('(')
  if (paren >= 0) t = t.slice(0, paren)
  if (t === 'BOOLEAN' || t === 'BOOL') return 'bool'
  if (t.startsWith('TIMESTAMP') || t === 'DATETIME' || t === 'DATE') return 'date'
  if (NUMBER_TYPES.has(t)) return 'number'
  if (JSON_TYPES.has(t)) return 'json'
  return 'text'
}

const NUMBER_TYPES = new Set([
  'BIGINT', 'INTEGER', 'INT', 'SMALLINT', 'TINYINT', 'HUGEINT', 'UBIGINT', 'UINTEGER', 'USMALLINT', 'UTINYINT',
  'INT2', 'INT4', 'INT8', 'DOUBLE', 'FLOAT', 'REAL', 'DECIMAL', 'NUMERIC', 'FLOAT4', 'FLOAT8'
])
const JSON_TYPES = new Set(['JSON', 'BLOB', 'BYTEA', 'STRUCT', 'LIST', 'MAP', 'UNION', 'ARRAY'])

/** SQL 表的列 → 看板字段。敏感列（系统库凭证列）整体剔除：后端也视其为不存在。 */
export function columnsToFields(columns: SchemaColumn[]): CollectionField[] {
  return columns
    .filter((c) => !c.sensitive && /^[A-Za-z][A-Za-z0-9_]{0,62}$/.test(c.name))
    .map((c) => ({
      name: c.name,
      type: sqlColumnKind(c.type),
      required: c.nullable === false,
      unique: false,
      options: {},
      hidden: false,
      readonly: true
    }))
}

/**
 * 表里可作为「时间字段」的列：date 类型的列，按常见命名（created_at 等）优先。
 * 返回空数组表示该表没有时间维度：趋势图与时间范围筛选都不可用。
 */
export function tableTimeFields(fields: CollectionField[]): string[] {
  const dates = fields.filter((f) => f.type === 'date').map((f) => f.name)
  const preferred = ['occurred_at', 'created_at', 'updated_at', 'started_at']
  return [...dates].sort((a, b) => rank(a, preferred) - rank(b, preferred))
}

function rank(name: string, preferred: string[]): number {
  const i = preferred.indexOf(name.toLowerCase())
  return i === -1 ? preferred.length : i
}

/** 集合明细的内置列：id 与两个时间戳，永远排在字段前面。 */
export const DOCUMENT_SYSTEM_COLUMNS = ['id', 'created_at', 'updated_at']

/**
 * 集合文档 → 明细表的列与行。列 = 系统列 + 已登记的可见字段 + 数据里出现过的其余键
 * （未登记 schema 的集合靠最后一类看到字段）。
 *
 * - 已登记且标记 hidden 的字段不会因为「数据里出现过」又被补回来（hidden 是展示约定，不是鉴权边界）；
 * - 系统列（id / created_at / updated_at）以文档元数据为准：业务数据里恰好有同名键时，
 *   不能覆盖真实的 id 与时间戳，否则明细里看到的 id 与实际可编辑 / 删除的对象对不上。
 */
export function documentTable(items: DocumentItem[], fields: CollectionField[]): { columns: string[]; rows: Array<Record<string, unknown>> } {
  const hidden = new Set(fields.filter((f) => f.hidden).map((f) => f.name))
  const names = new Set<string>(DOCUMENT_SYSTEM_COLUMNS)
  for (const f of fields) if (!f.hidden) names.add(f.name)
  for (const item of items) for (const key of Object.keys(item.data)) if (!hidden.has(key)) names.add(key)
  const rows = items.map((item) => {
    const data = Object.fromEntries(Object.entries(item.data).filter(([key]) => !hidden.has(key)))
    return { ...data, id: item.id, created_at: item.createdAt, updated_at: item.updatedAt }
  })
  return { columns: [...names], rows }
}

/** SQL 表分页行（二维数组，与 columns 按下标对齐）→ 明细表的列与行。敏感列已在后端置空，这里同样不展示。 */
export function sqlTable(result: Pick<SchemaTableRows, 'columns' | 'rows'>): { columns: string[]; rows: Array<Record<string, unknown>> } {
  const visible = result.columns.map((c, i) => ({ name: c.name, i, hidden: Boolean(c.sensitive) })).filter((c) => !c.hidden)
  const rows = result.rows.map((row) => Object.fromEntries(visible.map((c) => [c.name, row[c.i]])))
  return { columns: visible.map((c) => c.name), rows }
}
