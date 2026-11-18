// useBiDataSource.ts — 数据看板的数据源加载链：库 → 数据源（集合 / SQL 表）→ 字段。
//
// 从 DataBrowser.vue 抽出，页面只负责编排。保留原页面的三个竞态计数器：
// 快速切库 / 切数据源时，旧响应一律丢弃，不会覆盖新选择。
//
// 数据源有两种：
//   - 集合库（dataModel !== 'sql'）：列出集合；字段来自登记的 schema，没登记则 404（降级只看总量与趋势）；
//   - SQL 库（dataModel === 'sql'）：列出表；字段是表的真实列，没有「登记」概念。

import { computed, ref } from 'vue'
import { api } from '@/services/api'
import { t } from '@/locales'
import { errorMessage } from '@/utils/format'
import { columnsToFields, sameSource, tableTimeFields, type BiSource } from '@/utils/bi-source'
import type { ApiError, CollectionField, CollectionInfo, CollectionSchema, DatabaseItem, SchemaTable } from '@/services/types'

export interface BiDataSourceOptions {
  projectId: () => string
  /** 数据源切换（含清空）时调用：页面据此清掉看板、Agent、明细与全局条件。 */
  onClear: () => void
  /** 字段加载完成：页面据此读看板配置 / 生成默认看板并拉明细。 */
  onLoaded: (info: LoadedSource) => void
  /** 字段加载失败（非「集合未登记」的 404）。错误文案同时落在 fieldsError，页面可直接渲染带重试的错误区。 */
  onFieldsError?: (message: string) => void
}

export interface LoadedSource {
  projectId: string
  databaseId: string
  source: BiSource
  fields: CollectionField[]
  /** 集合：已登记且有字段；SQL 表：恒为 true（真实列就是字段）。 */
  registered: boolean
  /** 表的时间列（按偏好排序）；集合恒为 null（用内置 created_at）。 */
  timeFields: string[]
}

export function useBiDataSource(opts: BiDataSourceOptions) {
  const databases = ref<DatabaseItem[]>([])
  const databaseId = ref('')
  const databasesLoading = ref(false)
  const databasesError = ref('')

  const collections = ref<CollectionInfo[]>([])
  const tables = ref<SchemaTable[]>([])
  const sourcesLoading = ref(false)
  const sourcesError = ref('')

  const source = ref<BiSource | null>(null)
  const schema = ref<CollectionSchema | null>(null)
  const fields = ref<CollectionField[]>([])
  const registered = ref(false)
  const timeFields = ref<string[]>([])
  /** 当前来源的字段是否已成功加载：失败后「重试」据此决定要不要重取字段。 */
  const fieldsLoaded = ref(false)
  /** 字段加载失败的提示（与明细区错误分开，重试要重取字段而不是重取明细）。 */
  const fieldsError = ref('')

  let databasesRequest = 0
  let sourcesRequest = 0
  let fieldsRequest = 0

  const selectedDatabase = computed(() => databases.value.find((d) => d.id === databaseId.value))
  const isSqlDatabase = computed(() => selectedDatabase.value?.dataModel === 'sql')
  const selectedCollection = computed(() =>
    source.value?.kind === 'collection' ? collections.value.find((c) => c.name === source.value!.name) : undefined
  )
  const isView = computed(() => selectedCollection.value?.schemaMode === 'view')
  const hasSources = computed(() => collections.value.length > 0 || tables.value.length > 0)

  /** 只清「数据源之下」的状态（字段、看板、明细等），不动数据源列表本身。 */
  function clearFields() {
    fieldsRequest++
    schema.value = null
    fields.value = []
    registered.value = false
    timeFields.value = []
    fieldsLoaded.value = false
    fieldsError.value = ''
    opts.onClear()
  }

  function clearSources() {
    collections.value = []
    tables.value = []
    source.value = null
    sourcesLoading.value = false
    sourcesError.value = ''
    clearFields()
  }

  async function loadDatabases(preserveSelection = false) {
    const currentProject = opts.projectId()
    const request = ++databasesRequest
    sourcesRequest++
    databasesLoading.value = true
    databasesError.value = ''
    if (!preserveSelection) {
      databaseId.value = ''
      clearSources()
    }
    if (!currentProject) {
      databases.value = []
      databaseId.value = ''
      clearSources()
      databasesLoading.value = false
      return
    }
    try {
      const list = await api.databases.list(currentProject)
      if (request !== databasesRequest || currentProject !== opts.projectId()) return
      databases.value = list.filter((d) => d.status === 'ready')
      const next = preserveSelection && databases.value.some((d) => d.id === databaseId.value) ? databaseId.value : databases.value[0]?.id || ''
      // 无论库 id 是否变化都显式加载数据源：不依赖外部 watcher，composable 可以脱离组件独立使用与测试。
      databaseId.value = next
      await loadSources(preserveSelection)
    } catch (error) {
      if (request !== databasesRequest || currentProject !== opts.projectId()) return
      databases.value = []
      databaseId.value = ''
      clearSources()
      databasesError.value = errorMessage(error, t('pages.dataBrowser.databaseLoadFailed'))
    } finally {
      if (request === databasesRequest) databasesLoading.value = false
    }
  }

  /** 列出当前库的数据源：集合库取集合，SQL 库取表。 */
  async function loadSources(preserveSelection = false) {
    const currentProject = opts.projectId()
    const currentDatabase = databaseId.value
    const sql = isSqlDatabase.value
    const request = ++sourcesRequest
    if (!currentProject || !currentDatabase) {
      clearSources()
      return
    }
    if (!preserveSelection) {
      source.value = null
      clearFields()
    }
    sourcesLoading.value = true
    sourcesError.value = ''
    const stale = () => request !== sourcesRequest || currentProject !== opts.projectId() || currentDatabase !== databaseId.value
    try {
      let list: BiSource[]
      if (sql) {
        const result = await api.databases.schema(currentProject, currentDatabase)
        if (stale()) return
        tables.value = result.tables
        collections.value = []
        list = result.tables.map((tb) => ({ kind: 'table' as const, name: tb.name }))
      } else {
        const items = await api.db.collectionItems(currentProject, currentDatabase)
        if (stale()) return
        collections.value = items
        tables.value = []
        list = items.map((c) => ({ kind: 'collection' as const, name: c.name }))
      }
      const keep = preserveSelection && source.value && list.some((s) => sameSource(s, source.value)) ? source.value : null
      const next = keep ?? list[0] ?? null
      if (!sameSource(source.value, next)) {
        source.value = next
        await onSourceChanged()
      } else if (!fieldsLoaded.value) {
        // 来源没变但字段还没成功加载（上次失败 / 尚未加载）：这时才需要重取字段。
        await onSourceChanged()
      }
      // 来源没变且字段已就绪：只刷新了来源列表（文档数 / 列数），保留筛选、看板、明细页等分析上下文。
    } catch (error) {
      if (stale()) return
      collections.value = []
      tables.value = []
      source.value = null
      clearFields()
      sourcesError.value = errorMessage(error, t('pages.dataBrowser.collectionLoadFailed'))
    } finally {
      if (request === sourcesRequest) sourcesLoading.value = false
    }
  }

  /** 数据源变化：取字段 → 交给页面读看板配置并拉明细。 */
  async function onSourceChanged() {
    clearFields()
    const current = source.value
    const projectId = opts.projectId()
    const dbId = databaseId.value
    if (!projectId || !dbId || !current) return
    const request = ++fieldsRequest
    const stale = () =>
      request !== fieldsRequest || projectId !== opts.projectId() || dbId !== databaseId.value || !sameSource(current, source.value)

    if (current.kind === 'table') {
      // SQL 表的字段是已经列出的真实列，不需要再请求。
      const table = tables.value.find((tb) => tb.name === current.name)
      if (!table) return
      const cols = columnsToFields(table.columns)
      fields.value = cols
      registered.value = true
      timeFields.value = tableTimeFields(cols)
      fieldsLoaded.value = true
      opts.onLoaded({ projectId, databaseId: dbId, source: current, fields: cols, registered: true, timeFields: timeFields.value })
      return
    }

    let loaded: CollectionSchema | null = null
    try {
      loaded = await api.db.getCollectionSchema(projectId, dbId, current.name)
    } catch (error) {
      // 404 = 集合未登记 schema：只能看到总量与时间趋势；其他错误当作加载失败。
      if ((error as ApiError).status !== 404) {
        if (!stale()) {
          const message = errorMessage(error, t('pages.dataBrowser.loadFailed'))
          fieldsError.value = message
          opts.onFieldsError?.(message)
        }
        return
      }
    }
    if (stale()) return
    schema.value = loaded
    fields.value = loaded?.fields ?? []
    registered.value = loaded !== null && loaded.fields.length > 0
    timeFields.value = []
    fieldsLoaded.value = true
    opts.onLoaded({ projectId, databaseId: dbId, source: current, fields: fields.value, registered: registered.value, timeFields: [] })
  }

  /** 字段加载失败后的重试：重取字段并重建看板（而不是只重取明细）。 */
  function retryFields() {
    return onSourceChanged()
  }

  function reloadSources() {
    return loadSources(true)
  }

  function refresh() {
    return loadDatabases(true)
  }

  function selectDatabase(id: string) {
    if (!id || id === databaseId.value || !databases.value.some((d) => d.id === id)) return
    databaseId.value = id
    void loadSources()
  }

  function selectSource(next: BiSource) {
    if (sameSource(next, source.value)) return
    source.value = next
    void onSourceChanged()
  }

  return {
    databases, databaseId, databasesLoading, databasesError,
    collections, tables, sourcesLoading, sourcesError,
    source, schema, fields, registered, timeFields, fieldsLoaded, fieldsError,
    selectedDatabase, isSqlDatabase, isView, hasSources,
    loadDatabases, loadSources, reloadSources, refresh, onSourceChanged, retryFields, selectDatabase, selectSource
  }
}
