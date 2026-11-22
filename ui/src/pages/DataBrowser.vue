<template>
  <ProjectScope>
    <PageContainer :subtitle="t('pages.dataBrowser.subtitle')">
      <Card>
        <CardContent class="flex flex-col gap-4 p-4 sm:p-5">
          <BiToolbar
            :databases="ds.databases.value"
            :database-id="ds.databaseId.value"
            :collections="ds.collections.value"
            :tables="ds.tables.value"
            :source="ds.source.value"
            :loading="ds.databasesLoading.value || ds.sourcesLoading.value"
            :refresh-key="refreshKey"
            :can-edit-schema="canEditSchema"
            :can-create-view="canCreateView"
            :can-import="canImport"
            @select-database="ds.selectDatabase"
            @select-source="ds.selectSource"
            @update:refresh="refreshKey = $event"
            @refresh="manualRefresh"
            @action="onAction"
          />

          <SbAsyncRegion
            v-if="ds.databasesError.value || ds.sourcesError.value"
            :show-error="true"
            :error="ds.databasesError.value || ds.sourcesError.value"
            @retry="ds.databasesError.value ? ds.refresh() : ds.reloadSources()"
          />
          <!-- 字段（Schema）加载失败：重试要重取字段并重建看板，而不是只重取明细 -->
          <SbAsyncRegion
            v-if="ds.fieldsError.value"
            :show-error="true"
            :error="ds.fieldsError.value"
            data-testid="fields-error"
            @retry="ds.retryFields()"
          />

          <template v-if="showBoard">
            <p v-if="isTable" class="text-xs text-muted-foreground" data-testid="table-hint">{{ t('pages.dataBrowser.tableReadonlyHint') }}</p>
            <BiFilterBar
              :range="range"
              :time-field="timeField"
              :date-fields="dateFields"
              :filter="filterDraft"
              :history="history"
              :chips="chips"
              :field-names="fieldNames"
              :dirty="Boolean(appliedFilter || chips.length || (timeField && range !== 'all'))"
              :pending="filterDraft.trim() !== appliedFilter"
              :error="filterError"
              @update:range="range = $event"
              @update:time-field="timeField = $event"
              @update:filter="filterDraft = $event"
              @apply="applyFilter"
              @clear="clearFilters"
              @append-field="appendField"
              @use-history="useHistory"
              @remove-chip="removeChip"
            />
          </template>
        </CardContent>
      </Card>

      <!-- 库里没有任何数据源（空集合库 / 空 SQL 库）：给出下一步，而不是旧的表结构面板 -->
      <SbAsyncRegion
        v-if="!showBoard"
        :pending="ds.databasesLoading.value || ds.sourcesLoading.value"
        :show-empty="!ds.databasesLoading.value && !ds.sourcesLoading.value && !ds.databasesError.value && !ds.sourcesError.value"
        :empty-title="t('pages.dataBrowser.dataSource')"
        :empty-description="emptyDescription"
      />

      <template v-else>
        <div v-if="needsSchema" class="flex flex-wrap items-center gap-3 rounded-lg border border-dashed border-border px-4 py-3 text-sm text-muted-foreground" data-testid="needs-schema">
          <span class="min-w-0 flex-1">{{ isTable ? t('components.bi.defaults.noTimeColumn') : t('components.bi.defaults.needSchema') }}</span>
          <Button v-if="!isTable && canEditSchema" size="sm" variant="outline" @click="schemaOpen = true">
            <TableIcon data-icon="inline-start" />{{ t('components.bi.defaults.registerSchema') }}
          </Button>
        </div>

        <BiDetailTable
          :storage-key="detailColsKey"
          :columns="detail.columns.value"
          :rows="detail.rows.value"
          :mono="detail.mono.value"
          :loading="detail.loading.value"
          :error="detail.error.value"
          :page="detail.page.value"
          :page-size="detail.pageSize"
          :total="detail.total.value"
          @update:page="detail.goTo"
          @retry="detail.load"
        />

        <div class="flex flex-wrap items-center justify-between gap-2">
          <Button size="sm" variant="outline" data-action="add-widget" @click="openEditor(null)">
            <PlusIcon data-icon="inline-start" />{{ t('components.bi.widget.add') }}
          </Button>
          <ConfirmAction :title="t('components.bi.widget.resetConfirm')" @confirm="resetBoard">
            <Button size="sm" variant="ghost" data-action="reset-board">
              <RotateCcwIcon data-icon="inline-start" />{{ t('components.bi.widget.resetDefault') }}
            </Button>
          </ConfirmAction>
        </div>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-12 sm:gap-5" data-testid="bi-board">
          <div v-for="w in dashboard.widgets.value" :key="w.id" :class="spanClass(w.span)">
            <BiWidgetRunner
              :widget="w"
              :target="target"
              :fields="ds.fields.value"
              :builtin-time="!isTable"
              :global-filter="globalFilter"
              :refresh-tick="refreshTick"
              :readonly="false"
              @edit="openEditor"
              @duplicate="duplicateWidget"
              @remove="askRemove"
              @chip="addChip"
              @outcome="recordOutcome(w.id, $event)"
            />
          </div>
        </div>

      </template>

      <BiWidgetEditor v-model:open="editorOpen" :widget="editing" :fields="ds.fields.value" :builtin-time="!isTable" @save="saveWidget" />
      <ConfirmAction
        controlled
        :open="removeTarget !== null"
        :title="t('components.bi.widget.removeConfirm', { title: removeTarget?.title ?? '' })"
        @update:open="(v) => { if (!v) removeTarget = null }"
        @confirm="confirmRemove"
      />

      <CollectionSchemaModal v-model:open="schemaOpen" :project-id="projectId" :database-id="ds.databaseId.value" :collection="collectionName" @saved="onSchemaSaved" />
      <ViewCollectionModal v-model:open="viewOpen" :project-id="projectId" :database-id="ds.databaseId.value" @saved="ds.reloadSources" />
      <CollectionImportModal v-model:open="importOpen" :project-id="projectId" :database-id="ds.databaseId.value" :collection="collectionName" @saved="refreshBoard" />
    </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
// DataBrowser：数据看板（planv6.7）。页面只做编排：
//   - 库 → 数据源（集合 / SQL 表）→ 字段的加载链在 useBiDataSource；
//   - 明细表的取数在 useBiDetail；
//   - 看板配置与全局条件在这里接线；图表、请求、校验都在 components/bi 与 composables。
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { toast } from 'vue-sonner'
import { PlusIcon, RotateCcwIcon, TableIcon } from '@lucide/vue'
import { api } from '@/services/api'
import type { BiWidget } from '@/services/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { errorMessage } from '@/utils/format'
import { downloadBlob } from '@/utils/download'
import { COLLECTION_TRAITS, MAX_WIDGETS, useBiDashboard } from '@/composables/useBiDashboard'
import { useBiDataSource } from '@/composables/useBiDataSource'
import { useBiDetail } from '@/composables/useBiDetail'
import { combine, pushHistory, rangeExpr, type FilterChip, type RangeKey } from '@/utils/bi-filter'
import { sourceKeyPart } from '@/utils/bi-source'
import type { BiDefaultLabels } from '@/utils/bi-defaults'
import PageContainer from '@/components/PageContainer.vue'
import ProjectScope from '@/components/ProjectScope.vue'
import ConfirmAction from '@/components/ConfirmAction.vue'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import CollectionSchemaModal from '@/components/databases/CollectionSchemaModal.vue'
import ViewCollectionModal from '@/components/databases/ViewCollectionModal.vue'
import CollectionImportModal from '@/components/databases/CollectionImportModal.vue'
import BiToolbar, { type DataAction, type RefreshKey } from '@/components/bi/BiToolbar.vue'
import BiFilterBar from '@/components/bi/BiFilterBar.vue'
import BiWidgetRunner, { type BiOutcome } from '@/components/bi/BiWidgetRunner.vue'
import BiWidgetEditor from '@/components/bi/BiWidgetEditor.vue'
import BiDetailTable from '@/components/bi/BiDetailTable.vue'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'

const { t } = useI18n()
const project = useProjectStore()
const auth = useAuthStore()
const { projectId, isAdmin } = storeToRefs(project)

// —— 全局条件（先声明：数据源清空时要重置它们）——
const range = ref<RangeKey>('all')
const timeField = ref('created_at')
const filterDraft = ref('')
const appliedFilter = ref('')
const history = ref<string[]>([])
const chips = ref<FilterChip[]>([])
const refreshKey = ref<RefreshKey>('off')
const refreshTick = ref(0)

// —— 看板配置 ——
const defaultLabels: BiDefaultLabels = {
  total: () => t('components.bi.kpi.total'),
  sum: (field) => t('components.bi.kpi.sum', { field }),
  avg: (field) => t('components.bi.kpi.avg', { field }),
  trend: () => t('components.bi.defaults.trend'),
  histogram: (field) => t('components.bi.defaults.histogram', { field }),
  share: (field) => t('components.bi.defaults.share', { field }),
  top: (field, n) => t('components.bi.defaults.top', { field, n }),
  dateTrend: (field) => t('components.bi.defaults.dateTrend', { field })
}
const dashboard = useBiDashboard(defaultLabels)
const needsSchema = dashboard.needsSchema

// —— 数据源 ——
const ds = useBiDataSource({
  projectId: () => projectId.value,
  onClear: clearBelowSource,
  onLoaded: onSourceLoaded
})

const isTable = computed(() => ds.source.value?.kind === 'table')
const collectionName = computed(() => (ds.source.value?.kind === 'collection' ? ds.source.value.name : ''))
const showBoard = computed(() => Boolean(ds.source.value) && !ds.sourcesError.value)
const target = computed(() => ({
  projectId: projectId.value,
  databaseId: ds.databaseId.value,
  collection: ds.source.value?.name ?? '',
  kind: ds.source.value?.kind ?? ('collection' as const)
}))
const emptyDescription = computed(() => {
  if (!ds.databaseId.value) return t('pages.dataBrowser.noDatabase')
  return ds.isSqlDatabase.value ? t('pages.dataBrowser.noTables') : t('pages.dataBrowser.noCollections')
})

// —— 权限：看板是只读视图，写入口（Schema / 视图 / 导入）按角色与数据源种类禁用 ——
const canWriteData = computed(() => !isAdmin.value && auth.canWrite)
const canEditSchema = computed(() => canWriteData.value && Boolean(collectionName.value) && !ds.isView.value)
const canCreateView = computed(() => canWriteData.value && Boolean(ds.databaseId.value))
const canImport = computed(() => canWriteData.value && Boolean(collectionName.value) && !ds.isView.value)

// 集合恒有 created_at / updated_at；SQL 表只有自己的时间列（可能没有）。
const dateFields = computed(() =>
  isTable.value
    ? ds.timeFields.value
    : ['created_at', 'updated_at', ...ds.fields.value.filter((f) => f.type === 'date' && !f.hidden).map((f) => f.name)]
)
const fieldNames = computed(() => ds.fields.value.filter((f) => !f.hidden).map((f) => f.name))
// 相对时间窗口（近 1 小时 / 24 小时…）的「现在」：每一轮刷新统一取一次，同一轮里所有卡片与明细共用，
// 刷新时跟着前进。直接在 computed 里 new Date() 会把窗口下界冻结在第一次求值的时刻，运行越久窗口越长。
const clock = ref(Date.now())
// 切换范围时立即对齐到当前时间（sync：先于任何依赖它的 computed 被读取，避免同一次切换发出两轮请求）。
watch(range, () => { clock.value = Date.now() }, { flush: 'sync' })
// 没有时间列时不套时间范围（时间范围控件也会随之隐藏），否则会拼出引用不存在列的条件。
const globalFilter = computed(() =>
  combine([timeField.value ? rangeExpr(range.value, timeField.value, new Date(clock.value)) : '', appliedFilter.value, ...chips.value.map((c) => c.expr)])
)
/** 条件的「身份」：只有用户改动条件才变，时钟前进不算——否则每次自动刷新都会把明细表翻回第一页。 */
const conditionKey = computed(() => JSON.stringify([timeField.value, range.value, appliedFilter.value, chips.value.map((c) => c.expr)]))

/**
 * 卡片请求结果（按 widget id）：条件写错时所有卡片会一起失败，逐张重复报错没有意义。
 * 只要「有卡片因条件失败」且「没有任何卡片成功」，就判定是条件本身的问题，在筛选区集中提示一次。
 */
const outcomes = ref<Record<string, BiOutcome>>({})
/** 这些错误码来自 filter 编译 / 扫描上限：换掉条件就能恢复，属于「条件问题」而不是图表配置问题。 */
const CONDITION_CODES = new Set(['invalid_filter', 'invalid_request', 'filter_scan_limit_exceeded'])
const filterError = computed(() => {
  const list = Object.values(outcomes.value)
  if (list.length === 0 || list.some((o) => o.ok)) return ''
  const failed = list.find((o) => CONDITION_CODES.has(o.code))
  return failed?.message ?? ''
})
function recordOutcome(id: string, payload: BiOutcome) {
  outcomes.value = { ...outcomes.value, [id]: payload }
}
// 条件 / 数据源变化后重新判定：上一轮的错误不能继续挂在新条件上。
watch([conditionKey, () => ds.source.value], () => { outcomes.value = {} })

/** 明细列显隐的持久化键：按「项目 + 库 + 数据源」区分，集合与同名 SQL 表不会共用。 */
const detailColsKey = computed(() =>
  ds.source.value
    ? `sb_bi_cols_v1:${projectId.value}:${ds.databaseId.value}:${sourceKeyPart(ds.source.value)}`
    : ''
)

// —— 明细表 ——
const detail = useBiDetail({
  scope: () => ({ projectId: projectId.value, databaseId: ds.databaseId.value, source: ds.source.value }),
  fields: () => ds.fields.value,
  filter: () => globalFilter.value
})

// —— 看板编辑 ——
const editorOpen = ref(false)
const editing = ref<BiWidget | null>(null)
const removeTarget = ref<BiWidget | null>(null)

// —— 弹窗 ——
const schemaOpen = ref(false)
const viewOpen = ref(false)
const importOpen = ref(false)

watch(projectId, () => { void ds.loadDatabases() }, { immediate: true })

// 自动刷新：页面隐藏时暂停（planv6.7 F12）。
let timer: ReturnType<typeof setInterval> | null = null
const REFRESH_MS: Record<Exclude<RefreshKey, 'off'>, number> = { s30: 30_000, m1: 60_000, m5: 300_000 }
watch(refreshKey, (key) => {
  if (timer) clearInterval(timer)
  timer = null
  if (key === 'off') return
  timer = setInterval(() => { if (typeof document === 'undefined' || document.visibilityState !== 'hidden') refreshBoard() }, REFRESH_MS[key])
})
onBeforeUnmount(() => { if (timer) clearInterval(timer) })

// 条件变化 → 明细表回到第一页重取（widget 自己监听 globalFilter 重取）。时钟前进不触发这里。
watch(conditionKey, () => { detail.reset(); void detail.load() })

/** 数据源切换（含清空）：看板、明细与全局条件一并重置。 */
function clearBelowSource() {
  detail.clear()
  dashboard.clear()
  outcomes.value = {}
  chips.value = []
  appliedFilter.value = ''
  filterDraft.value = ''
  // 历史里的表达式引用的是上一个数据源的字段，带到新来源只会产生必然失败的条件。
  history.value = []
  range.value = 'all'
}

/** 字段就绪：读看板配置或生成默认看板，并按数据源决定时间字段，再拉明细。 */
function onSourceLoaded(info: { projectId: string; databaseId: string; source: { kind: 'collection' | 'table'; name: string }; fields: typeof ds.fields.value; registered: boolean; timeFields: string[] }) {
  const table = info.source.kind === 'table'
  timeField.value = table ? (info.timeFields[0] ?? '') : 'created_at'
  const traits = table ? { builtinTime: false, trendField: info.timeFields[0] ?? null } : COLLECTION_TRAITS
  // 表没有「登记」概念：有时间列就不提示；没有时间列时提示「没有时间趋势」。
  const needs = table ? info.timeFields.length === 0 : !info.registered
  dashboard.load(
    { projectId: info.projectId, databaseId: info.databaseId, collection: info.source.name, kind: info.source.kind },
    info.fields,
    table ? true : info.registered,
    traits
  )
  if (table) dashboard.needsSchema.value = needs
  if (dashboard.dropped.value > 0) toast.warning(t('components.bi.widget.droppedFields', { n: dashboard.dropped.value }))
  void detail.load()
}

async function onSchemaSaved() {
  await ds.onSourceChanged()
  await ds.reloadSources()
}

/**
 * 刷新数据：推进时间锚点、通知所有卡片重取、明细保持当前页。
 * 时间锚点与刷新信号在同一个 tick 内变化，卡片只会重取一次。
 */
function refreshBoard() {
  clock.value = Date.now()
  refreshTick.value++
  void detail.load()
}

/**
 * 手动刷新：只刷新数据。不能走「重载数据源」那条链——它会清掉筛选、时间范围、联动 chip 与明细页码，
 * 用户刚设好的分析上下文就丢了。来源列表（文档数 / 列数）仍会更新；来源消失时才回退并清理。
 */
async function manualRefresh() {
  await ds.refresh()
  if (ds.fieldsLoaded.value) refreshBoard()
}

// —— 全局条件操作 ——
function applyFilter() {
  appliedFilter.value = filterDraft.value.trim()
  history.value = pushHistory(history.value, appliedFilter.value)
}
/** 清除全部条件：时间范围、表达式、联动 chip 一起恢复（只有时间范围时也能清）。 */
function clearFilters() {
  filterDraft.value = ''
  appliedFilter.value = ''
  chips.value = []
  range.value = 'all'
}
function appendField(name: string) {
  filterDraft.value += (filterDraft.value && !filterDraft.value.endsWith(' ') ? ' && ' : '') + name
}
function useHistory(filter: string) {
  filterDraft.value = filter
  applyFilter()
}
function addChip(chip: FilterChip) {
  if (!chips.value.some((c) => c.id === chip.id)) chips.value = [...chips.value, chip]
}
function removeChip(id: string) {
  chips.value = chips.value.filter((c) => c.id !== id)
}

// —— 看板操作 ——
function spanClass(span: number): string {
  return { 4: 'md:col-span-4', 6: 'md:col-span-6', 8: 'md:col-span-8', 12: 'md:col-span-12' }[span] ?? 'md:col-span-6'
}
function openEditor(widget: BiWidget | null) {
  editing.value = widget
  editorOpen.value = true
}
function saveWidget(widget: BiWidget) {
  if (dashboard.widgets.value.some((w) => w.id === widget.id)) {
    dashboard.update(widget)
    return
  }
  if (!dashboard.add(widget)) toast.error(t('components.bi.widget.limitReached', { n: MAX_WIDGETS }))
}
function duplicateWidget(id: string) {
  if (!dashboard.duplicate(id)) toast.error(t('components.bi.widget.limitReached', { n: MAX_WIDGETS }))
}
function askRemove(widget: BiWidget) {
  removeTarget.value = widget
}
function confirmRemove() {
  if (removeTarget.value) dashboard.remove(removeTarget.value.id)
  removeTarget.value = null
}
function resetBoard() {
  dashboard.reset()
}
// —— 数据操作菜单（仅集合有）——
async function onAction(action: DataAction) {
  if (action === 'schema') schemaOpen.value = true
  else if (action === 'view') viewOpen.value = true
  else if (action === 'import') importOpen.value = true
  else await exportData(action === 'export-csv' ? 'csv' : 'json')
}

async function exportData(format: 'csv' | 'json') {
  const c = collectionName.value
  if (!projectId.value || !ds.databaseId.value || !c) return
  try {
    const blob = await api.db.exportCollection(projectId.value, ds.databaseId.value, c, format, { filter: globalFilter.value || undefined })
    downloadBlob(blob, `${c}.${format}`)
  } catch (error) {
    toast.error(errorMessage(error, t('pages.dataBrowser.exportFailed')))
  }
}
// 测试专用的只读观察点：页面状态本身不对外，但集成测试需要读到「当前数据源 / 全局条件」并驱动加载链。
defineExpose({ ds, globalFilter, chips, range, refreshKey, exportData, onAction })
</script>
