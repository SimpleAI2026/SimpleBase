// useBiDetail.ts — 看板明细表的取数：集合走文档列表，SQL 表走表分页，二者归一成「列 + 行」。
//
// 竞态：每次请求带令牌，且校验项目 / 库 / 数据源没变；旧响应一律丢弃（同 useBiQuery）。

import { computed, ref } from 'vue'
import { api } from '@/services/api'
import { t } from '@/locales'
import { errorMessage } from '@/utils/format'
import { DOCUMENT_SYSTEM_COLUMNS, documentTable, sameSource, sqlTable, type BiSource } from '@/utils/bi-source'
import type { CollectionField } from '@/services/types'

export const DETAIL_PAGE_SIZE = 20

export interface BiDetailOptions {
  scope: () => { projectId: string; databaseId: string; source: BiSource | null }
  fields: () => CollectionField[]
  /** 全局条件（时间范围 + filter + 联动 chip 合成后的表达式）。 */
  filter: () => string
}

export function useBiDetail(opts: BiDetailOptions) {
  const columns = ref<string[]>([])
  const rows = ref<Array<Record<string, unknown>>>([])
  const total = ref(0)
  const loading = ref(false)
  const error = ref('')
  const page = ref(1)
  let request = 0

  /** 集合的 id / 时间戳用等宽字体；SQL 表没有这层约定。 */
  const mono = computed(() => (opts.scope().source?.kind === 'collection' ? DOCUMENT_SYSTEM_COLUMNS : []))

  /** 切数据源：清空并作废在途请求。 */
  function clear() {
    request++
    columns.value = []
    rows.value = []
    total.value = 0
    error.value = ''
    loading.value = false
    page.value = 1
  }

  /** 回到第一页（条件变化时）。 */
  function reset() {
    page.value = 1
  }

  async function load() {
    const mine = ++request
    const { projectId, databaseId, source } = opts.scope()
    if (!projectId || !databaseId || !source) return
    loading.value = true
    error.value = ''
    const filter = opts.filter() || undefined
    const offset = (page.value - 1) * DETAIL_PAGE_SIZE
    const current = () => {
      const now = opts.scope()
      return mine === request && now.projectId === projectId && now.databaseId === databaseId && sameSource(now.source, source)
    }
    try {
      if (source.kind === 'table') {
        const result = await api.databases.tableRows(projectId, databaseId, source.name, { limit: DETAIL_PAGE_SIZE, offset, filter })
        if (!current()) return
        const table = sqlTable(result)
        columns.value = table.columns
        rows.value = table.rows
        total.value = result.total
      } else {
        const result = await api.db.documents(projectId, databaseId, source.name, { limit: DETAIL_PAGE_SIZE, offset, filter, count: true })
        if (!current()) return
        const table = documentTable(result.items, opts.fields())
        columns.value = table.columns
        rows.value = table.rows
        total.value = result.total ?? offset + result.items.length
      }
    } catch (e) {
      if (mine === request) error.value = errorMessage(e, t('pages.dataBrowser.loadFailed')) || t('pages.dataBrowser.loadFailed')
    } finally {
      if (mine === request) loading.value = false
    }
  }

  function goTo(next: number) {
    page.value = next
    void load()
  }

  return { columns, rows, mono, total, loading, error, page, pageSize: DETAIL_PAGE_SIZE, clear, reset, load, goTo }
}
