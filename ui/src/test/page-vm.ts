import type { VueWrapper } from '@vue/test-utils'

/**
 * 页面内部状态的测试适配层。
 *
 * 数据源加载链抽进 useBiDataSource 后，页面通过 defineExpose 暴露 `ds`（composable 的返回值）。
 * 老用例用过的 `vm.databaseId / vm.collection / vm.loadDatabases ...` 在这里映射到新结构，
 * 这样断言本身（竞态、整链刷新、导出等）不用改写。
 */
export interface PageVm {
  databaseId: string
  /** 当前数据源名（集合名或 SQL 表名）；未选为空串。 */
  collection: string
  sourceKind: '' | 'collection' | 'table'
  globalFilter: string
  chips: Array<{ id: string; expr: string }>
  range: string
  refreshKey: string
  loadDatabases: (preserve?: boolean) => Promise<void>
  loadCollections: (preserve?: boolean) => Promise<void>
  reloadCollections: () => Promise<void>
  refresh: () => Promise<void>
  selectDatabase: (id: string) => void
  selectCollection: (name: string) => void
  selectTable: (name: string) => void
  exportData: (format: 'csv' | 'json') => Promise<void>
  onAction: (action: string) => Promise<void>
}

interface Exposed {
  ds: {
    databaseId: { value: string }
    source: { value: { kind: 'collection' | 'table'; name: string } | null }
    loadDatabases: (preserve?: boolean) => Promise<void>
    loadSources: (preserve?: boolean) => Promise<void>
    reloadSources: () => Promise<void>
    refresh: () => Promise<void>
    selectDatabase: (id: string) => void
    selectSource: (s: { kind: 'collection' | 'table'; name: string }) => void
  }
  // defineExpose 的顶层 ref 会被 Vue 自动解包，所以这里直接是值；`ds` 是普通对象，其内部的 ref 不解包。
  globalFilter: string
  chips: Array<{ id: string; expr: string }>
  range: string
  refreshKey: string
  exportData: (format: 'csv' | 'json') => Promise<void>
  onAction: (action: string) => Promise<void>
}

export function pageVm(wrapper: VueWrapper): PageVm {
  const e = wrapper.vm as unknown as Exposed
  return {
    get databaseId() { return e.ds.databaseId.value },
    get collection() { return e.ds.source.value?.name ?? '' },
    get sourceKind() { return e.ds.source.value?.kind ?? '' },
    get globalFilter() { return e.globalFilter },
    get chips() { return e.chips },
    get range() { return e.range },
    set range(v: string) { e.range = v },
    get refreshKey() { return e.refreshKey },
    set refreshKey(v: string) { e.refreshKey = v },
    loadDatabases: (preserve) => e.ds.loadDatabases(preserve),
    loadCollections: (preserve) => e.ds.loadSources(preserve),
    reloadCollections: () => e.ds.reloadSources(),
    refresh: () => e.ds.refresh(),
    selectDatabase: (id) => e.ds.selectDatabase(id),
    selectCollection: (name) => e.ds.selectSource({ kind: 'collection', name }),
    selectTable: (name) => e.ds.selectSource({ kind: 'table', name }),
    exportData: (format) => e.exportData(format),
    onAction: (action) => e.onAction(action)
  } as PageVm
}
