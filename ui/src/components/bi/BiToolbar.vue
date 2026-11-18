<template>
  <div class="flex flex-wrap items-center gap-x-4 gap-y-2" data-testid="bi-toolbar">
    <div class="flex min-w-0 flex-wrap items-center gap-2">
    <span class="text-sm text-muted-foreground">{{ t('pages.dataBrowser.dataSource') }}</span>

    <!-- 只有一个 ready 库时隐藏库下拉（每个项目只有一个用户库） -->
    <Select v-if="databases.length > 1" :model-value="databaseId || undefined" @update:model-value="emit('select-database', String($event))">
      <SelectTrigger class="w-full sm:w-52" size="sm" :disabled="loading" data-field="database">
        <SelectValue :placeholder="t('pages.dataBrowser.database')" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem v-for="d in databases" :key="d.id" :value="d.id">{{ d.name }}</SelectItem>
      </SelectContent>
    </Select>

    <!-- 数据源：集合与 SQL 表放在同一个下拉里，值是 encodeSource(...)；没有数据源时禁用 -->
    <Select :model-value="sourceValue || undefined" @update:model-value="onSource(String($event))">
      <SelectTrigger class="w-full sm:w-60" size="sm" :disabled="loading || (collections.length === 0 && tables.length === 0)" data-field="collection">
        <SelectValue :placeholder="t('pages.dataBrowser.collection')" />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup v-if="collections.length">
          <span v-if="tables.length" class="block px-1.5 py-1 text-xs text-muted-foreground" data-group="collection">{{ t('pages.dataBrowser.groupCollections') }}</span>
          <SelectItem v-for="c in collections" :key="'c:' + c.name" :value="encodeSource({ kind: 'collection', name: c.name })">
            {{ c.name }}
            <span class="ml-2 text-xs text-muted-foreground">{{ c.schemaMode === 'view' ? t('pages.dataBrowser.viewBadge') : c.documentCount }}</span>
          </SelectItem>
        </SelectGroup>
        <SelectGroup v-if="tables.length">
          <span v-if="collections.length" class="block px-1.5 py-1 text-xs text-muted-foreground" data-group="table">{{ t('pages.dataBrowser.groupTables') }}</span>
          <SelectItem v-for="tb in tables" :key="'t:' + tb.name" :value="encodeSource({ kind: 'table', name: tb.name })">
            {{ tb.name }}
            <span class="ml-2 text-xs text-muted-foreground">{{ t('pages.dataBrowser.columnCount', { n: tb.columns.length }) }}</span>
          </SelectItem>
        </SelectGroup>
      </SelectContent>
    </Select>
    </div>

    <!-- 图表类型：Agent 生成时的偏好类型，与数据源同一排（planv6.7 ui-simplify-ux-plan） -->
    <div v-if="chartType" class="flex min-w-0 flex-wrap items-center gap-2">
      <span class="text-sm text-muted-foreground">{{ t('pages.dataBrowser.chartType') }}</span>
      <Select :model-value="chartType" @update:model-value="emit('update:chartType', String($event) as BiChartType)">
        <SelectTrigger class="w-full sm:w-40" size="sm" :aria-label="t('pages.dataBrowser.chartType')" data-field="chart-type">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem v-for="type in ALL_TYPES" :key="type" :value="type">{{ t(`components.bi.types.${type}`) }}</SelectItem>
        </SelectContent>
      </Select>
    </div>

    <div class="ml-auto flex flex-wrap items-center gap-2">
      <Select :model-value="refreshKey" @update:model-value="emit('update:refresh', String($event) as RefreshKey)">
        <SelectTrigger class="w-32" size="sm" :aria-label="t('components.bi.refresh.label')" data-field="refresh">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem v-for="key in REFRESH_KEYS" :key="key" :value="key">{{ refreshLabel(key) }}</SelectItem>
        </SelectContent>
      </Select>

      <Button variant="outline" size="sm" :disabled="loading" data-action="refresh" @click="emit('refresh')">
        <Spinner v-if="loading" data-icon="inline-start" />
        <RefreshCwIcon v-else data-icon="inline-start" />
        {{ t('common.refresh') }}
      </Button>

      <!-- 数据操作：Popover 内放按钮列表（不引入 dropdown-menu） -->
      <Popover v-if="sourceKind === 'collection'" v-model:open="menuOpen">
        <PopoverTrigger as-child>
          <Button variant="outline" size="sm" :disabled="!sourceValue" data-action="data-actions">
            <DatabaseIcon data-icon="inline-start" />{{ t('pages.dataBrowser.dataActions') }}
          </Button>
        </PopoverTrigger>
        <PopoverContent align="end" class="w-48 gap-1 p-1.5">
          <Button size="sm" variant="ghost" class="justify-start" :disabled="!canEditSchema" data-action="schema" @click="pick('schema')">
            <TableIcon data-icon="inline-start" />{{ t('pages.dataBrowser.schema') }}
          </Button>
          <Button size="sm" variant="ghost" class="justify-start" :disabled="!canCreateView" data-action="view" @click="pick('view')">
            <EyeIcon data-icon="inline-start" />{{ t('pages.dataBrowser.newView') }}
          </Button>
          <Button size="sm" variant="ghost" class="justify-start" :disabled="!canImport" data-action="import" @click="pick('import')">
            <UploadIcon data-icon="inline-start" />{{ t('pages.dataBrowser.import') }}
          </Button>
          <Button size="sm" variant="ghost" class="justify-start" data-action="export-csv" @click="pick('export-csv')">
            <DownloadIcon data-icon="inline-start" />{{ t('pages.dataBrowser.exportCSV') }}
          </Button>
          <Button size="sm" variant="ghost" class="justify-start" data-action="export-json" @click="pick('export-json')">
            <DownloadIcon data-icon="inline-start" />{{ t('pages.dataBrowser.exportJSON') }}
          </Button>
        </PopoverContent>
      </Popover>
    </div>
  </div>
</template>

<script setup lang="ts">
// BiToolbar：数据源、刷新、自动刷新、「数据操作」菜单（planv6.7 §4.1，F1 / F11 / F12）。
// Schema / 导入 / 新建视图 / 导出原先散在页面顶部，这里收进一个菜单；
// 权限判断由宿主算好以 can* 传入，工具栏只负责禁用。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { DatabaseIcon, DownloadIcon, EyeIcon, RefreshCwIcon, TableIcon, UploadIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import type { BiChartType, CollectionInfo, DatabaseItem, SchemaTable } from '@/services/types'
import { ALL_TYPES } from '@/utils/bi-compat'
import { decodeSource, encodeSource, type BiSource, type BiSourceKind } from '@/utils/bi-source'

export type RefreshKey = 'off' | 's30' | 'm1' | 'm5'
export type DataAction = 'schema' | 'view' | 'import' | 'export-csv' | 'export-json'

const props = defineProps<{
  databases: DatabaseItem[]
  databaseId: string
  /** 当前库的集合（集合库）。 */
  collections: CollectionInfo[]
  /** 当前库的 SQL 表（SQL 库）。 */
  tables: SchemaTable[]
  /** 当前选中的数据源；null 表示未选。 */
  source: BiSource | null
  loading: boolean
  refreshKey: RefreshKey
  canEditSchema: boolean
  canCreateView: boolean
  canImport: boolean
  /** 页面级「偏好图表类型」；不传则不显示该下拉（没有数据源时）。 */
  chartType?: BiChartType
}>()

const emit = defineEmits<{
  'select-database': [id: string]
  'select-source': [source: BiSource]
  'update:refresh': [key: RefreshKey]
  'update:chartType': [type: BiChartType]
  refresh: []
  action: [action: DataAction]
}>()

const { t } = useI18n()
const sourceValue = computed(() => (props.source ? encodeSource(props.source) : ''))
const sourceKind = computed<BiSourceKind | null>(() => props.source?.kind ?? null)

function onSource(value: string) {
  const next = decodeSource(value)
  if (next) emit('select-source', next)
}

const REFRESH_KEYS: RefreshKey[] = ['off', 's30', 'm1', 'm5']
const menuOpen = ref(false)

function refreshLabel(key: RefreshKey): string {
  return key === 'off' ? t('components.bi.refresh.label') + '：' + t('components.bi.refresh.off') : t(`components.bi.refresh.${key}`)
}

function pick(action: DataAction) {
  menuOpen.value = false
  emit('action', action)
}
</script>
