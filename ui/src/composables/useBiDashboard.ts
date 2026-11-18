// useBiDashboard.ts — 看板配置状态与持久化（planv6.7 §4.2 / F10）。
//
// 只管「有哪些图表」：widgets 的增删改、localStorage 读写、字段失效丢弃、恢复默认。
// 不发请求（请求由每个 widget 自己的 useBiQuery 负责），因此可以脱离组件直接测试。

import { ref, type Ref } from 'vue'
import type { BiDashboardConfig, BiWidget, CollectionField } from '@/services/types'
import { buildDefaultWidgets, type BiDefaultLabels } from '@/utils/bi-defaults'
import { checkType, fieldMap } from '@/utils/bi-compat'
import { sourceKeyPart, type BiSourceKind } from '@/utils/bi-source'

const STORAGE_PREFIX = 'sb_bi_v1'
/** 看板配置里 widget 数量的硬上限，防止 localStorage 被异常数据撑大。 */
export const MAX_WIDGETS = 24

export interface BiScope {
  projectId: string
  databaseId: string
  /** 数据源名：集合名或 SQL 表名（字段名沿用 collection，兼容已有调用与本地配置）。 */
  collection: string
  /** 缺省为集合。SQL 表的配置 key 带 `table:` 前缀，不会与同名集合串配置。 */
  kind?: BiSourceKind
}

export function storageKey(scope: BiScope): string {
  return `${STORAGE_PREFIX}:${scope.projectId}:${scope.databaseId}:${sourceKeyPart({ kind: scope.kind ?? 'collection', name: scope.collection })}`
}

/** 数据源的种类差异：看板生成与字段校验据此区分。缺省即集合，保持既有行为。 */
export interface BiSourceTraits {
  /** 是否有内置 created_at / updated_at（集合有，SQL 表没有）。 */
  builtinTime: boolean
  /** 默认看板「记录趋势」用的时间列；null 表示不生成趋势。 */
  trendField: string | null
}
export const COLLECTION_TRAITS: BiSourceTraits = { builtinTime: true, trendField: 'created_at' }

function isWidget(v: unknown): v is BiWidget {
  if (!v || typeof v !== 'object') return false
  const o = v as Record<string, unknown>
  return typeof o.id === 'string' && typeof o.title === 'string' && typeof o.type === 'string' &&
    Array.isArray(o.metrics) && o.metrics.every((m) => typeof m === 'string') && [4, 6, 8, 12].includes(o.span as number)
}

/** 读配置；损坏 / 版本不符 / 无存储一律返回 null（调用方回落到默认看板）。 */
export function loadConfig(scope: BiScope, storage: Storage | undefined = safeStorage()): BiDashboardConfig | null {
  if (!storage) return null
  try {
    const raw = storage.getItem(storageKey(scope))
    if (!raw) return null
    const parsed = JSON.parse(raw) as { version?: number; widgets?: unknown }
    if (parsed.version !== 1 || !Array.isArray(parsed.widgets)) return null
    return { version: 1, widgets: parsed.widgets.filter(isWidget).slice(0, MAX_WIDGETS) }
  } catch {
    return null
  }
}

export function saveConfig(scope: BiScope, widgets: BiWidget[], storage: Storage | undefined = safeStorage()): void {
  if (!storage) return
  try {
    storage.setItem(storageKey(scope), JSON.stringify({ version: 1, widgets: widgets.slice(0, MAX_WIDGETS) } satisfies BiDashboardConfig))
  } catch {
    /* 配额满或隐私模式：持久化只是体验层，失败不影响看板使用。 */
  }
}

export function clearConfig(scope: BiScope, storage: Storage | undefined = safeStorage()): void {
  try {
    storage?.removeItem(storageKey(scope))
  } catch {
    /* 同上 */
  }
}

function safeStorage(): Storage | undefined {
  try {
    return typeof window !== 'undefined' ? window.localStorage : undefined
  } catch {
    return undefined
  }
}

/** 丢弃引用了已不存在字段（或配置已不合法）的 widget，返回保留项与被丢弃数。 */
export function dropInvalidWidgets(
  widgets: BiWidget[],
  schemaFields: CollectionField[],
  traits: BiSourceTraits = COLLECTION_TRAITS
): { kept: BiWidget[]; dropped: number } {
  const fields = fieldMap(schemaFields, traits.builtinTime)
  const kept = widgets.filter((w) => checkType(w.type, w, fields).ok)
  return { kept, dropped: widgets.length - kept.length }
}

let seq = 0
/** 生成 widget id：同一页面内唯一即可，不进入后端。 */
export function newWidgetId(prefix = 'w'): string {
  seq += 1
  return `${prefix}-${Date.now().toString(36)}-${seq}`
}

export interface BiDashboard {
  widgets: Ref<BiWidget[]>
  needsSchema: Ref<boolean>
  /** 加载时有几个 widget 因字段失效被丢弃（>0 时页面 toast 提示）。 */
  dropped: Ref<number>
  /** 读配置，没有则生成默认看板。切集合时调用。 */
  load: (scope: BiScope, fields: CollectionField[], registered: boolean, traits?: BiSourceTraits) => void
  add: (widget: BiWidget) => void
  update: (widget: BiWidget) => void
  duplicate: (id: string) => BiWidget | null
  remove: (id: string) => void
  /** 恢复默认看板并清掉本地配置。 */
  reset: () => void
  /** 清空（切项目 / 库 / 集合时）。 */
  clear: () => void
}

export function useBiDashboard(labels: BiDefaultLabels, storage?: Storage): BiDashboard {
  const widgets = ref<BiWidget[]>([])
  const needsSchema = ref(false)
  const dropped = ref(0)
  let scope: BiScope | null = null
  let schemaFields: CollectionField[] = []
  let registered = false
  let traits: BiSourceTraits = COLLECTION_TRAITS

  const store = () => storage ?? safeStorage()

  function persist() {
    if (scope) saveConfig(scope, widgets.value, store())
  }

  function applyDefaults() {
    const board = buildDefaultWidgets(schemaFields, registered, labels, traits.trendField)
    widgets.value = board.widgets
    needsSchema.value = board.needsSchema
  }

  function load(next: BiScope, fields: CollectionField[], isRegistered: boolean, nextTraits: BiSourceTraits = COLLECTION_TRAITS) {
    scope = next
    schemaFields = fields
    registered = isRegistered
    traits = nextTraits
    dropped.value = 0
    const saved = loadConfig(next, store())
    if (!saved) {
      applyDefaults()
      return
    }
    const { kept, dropped: n } = dropInvalidWidgets(saved.widgets, fields, traits)
    dropped.value = n
    widgets.value = kept
    needsSchema.value = !isRegistered || fields.length === 0
    // 丢弃过字段时把清理后的结果写回，避免每次打开都重复提示。
    if (n > 0) persist()
  }

  /** 加入看板；已满（MAX_WIDGETS）时返回 false，调用方据此提示并保留草稿 / 预览，不能假装成功。 */
  function add(widget: BiWidget): boolean {
    if (widgets.value.length >= MAX_WIDGETS) return false
    widgets.value = [...widgets.value, widget]
    persist()
    return true
  }

  function update(widget: BiWidget) {
    widgets.value = widgets.value.map((w) => (w.id === widget.id ? widget : w))
    persist()
  }

  function duplicate(id: string): BiWidget | null {
    const src = widgets.value.find((w) => w.id === id)
    if (!src || widgets.value.length >= MAX_WIDGETS) return null
    const copy: BiWidget = { ...src, id: newWidgetId(), metrics: [...src.metrics] }
    const at = widgets.value.findIndex((w) => w.id === id)
    const next = [...widgets.value]
    next.splice(at + 1, 0, copy)
    widgets.value = next
    persist()
    return copy
  }

  function remove(id: string) {
    widgets.value = widgets.value.filter((w) => w.id !== id)
    persist()
  }

  function reset() {
    if (scope) clearConfig(scope, store())
    dropped.value = 0
    applyDefaults()
  }

  function clear() {
    scope = null
    schemaFields = []
    registered = false
    widgets.value = []
    needsSchema.value = false
    dropped.value = 0
  }

  return { widgets, needsSchema, dropped, load, add, update, duplicate, remove, reset, clear }
}
