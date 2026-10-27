<template>
  <ProjectScope>
    <PageContainer :subtitle="t('pages.dataBrowser.subtitle')">
      <Card>
        <CardContent class="p-4 sm:p-5">
          <div class="flex flex-wrap items-center gap-3">
            <Select :model-value="databaseId || undefined" class="w-full sm:w-64" @update:model-value="selectDatabase">
              <SelectTrigger class="w-full" :disabled="databasesLoading || databases.length === 0">
                <SelectValue :placeholder="t('pages.dataBrowser.database')" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem v-for="database in databases" :key="database.id" :value="database.id">
                  {{ database.name }}
                </SelectItem>
              </SelectContent>
            </Select>
            <Button variant="outline" :disabled="!projectId || databasesLoading" @click="refresh">
              <Spinner v-if="databasesLoading" data-icon="inline-start" />
              <RefreshCwIcon v-else data-icon="inline-start" />
              {{ t('common.refresh') }}
            </Button>
            <Button variant="outline" :disabled="!collection || readonly" @click="schemaOpen = true">{{ t('pages.dataBrowser.schema') }}</Button>
            <Button variant="outline" :disabled="!databaseId || readonly || isSqlDatabase" @click="viewOpen = true">{{ t('pages.dataBrowser.newView') }}</Button>
            <Button variant="outline" :disabled="!collection || readonly || isSqlDatabase" @click="importOpen = true">{{ t('pages.dataBrowser.import') }}</Button>
            <Button variant="outline" :disabled="!collection || isSqlDatabase" @click="exportData('csv')">{{ t('pages.dataBrowser.exportCSV') }}</Button>
            <Button variant="outline" :disabled="!collection || isSqlDatabase" @click="exportData('json')">{{ t('pages.dataBrowser.exportJSON') }}</Button>
          </div>
          <SbAsyncRegion
            v-if="databasesError"
            class="mt-4"
            :show-error="true"
            :error="databasesError"
            @retry="refresh"
          />
        </CardContent>
      </Card>

      <Card v-if="isSqlDatabase && selectedDatabase">
        <CardContent class="p-4 sm:p-5">
          <SchemaPanel :key="selectedDatabase.id" :project-id="projectId" :database="selectedDatabase" :readonly="readonly" />
        </CardContent>
      </Card>

      <div v-else class="grid gap-4 sm:gap-5 xl:grid-cols-[15rem_minmax(0,1fr)]">
        <aside>
          <Card>
            <CardContent class="p-4 sm:p-5">
              <p class="mb-3 text-xs font-medium text-muted-foreground">{{ t('pages.dataBrowser.collections') }}</p>
              <SbAsyncRegion
                :pending="collectionsLoading"
                :show-empty="!collectionsLoading && collections.length === 0"
                :show-error="Boolean(collectionsError)"
                :error="collectionsError"
                :empty-title="t('pages.dataBrowser.collections')"
                :empty-description="databaseId ? t('pages.dataBrowser.noCollections') : t('pages.dataBrowser.noDatabase')"
                @retry="reloadCollections"
              >
                <div class="flex max-h-48 flex-col gap-1 overflow-y-auto xl:max-h-none">
                  <button
                    v-for="item in collections"
                    :key="item.name"
                    type="button"
                    class="flex min-w-0 items-center justify-between rounded-md px-2 py-2 text-left text-sm hover:bg-accent"
                    :class="collection === item.name ? 'bg-accent' : ''"
                    @click="selectCollection(item.name)"
                  >
                    <span class="truncate">{{ item.name }}</span>
                    <span class="ml-2 shrink-0 text-xs text-muted-foreground">{{ item.schemaMode === 'view' ? 'VIEW' : item.documentCount }}</span>
                  </button>
                </div>
              </SbAsyncRegion>
            </CardContent>
          </Card>
        </aside>

        <main class="flex min-w-0 flex-col gap-4 sm:gap-5">
          <Card>
            <CardContent class="p-4 sm:p-5">
              <div class="grid gap-3 md:grid-cols-[minmax(0,1fr)_14rem_auto]">
                <Input v-model="filter" :placeholder="t('pages.dataBrowser.filter')" @keydown.enter="applyQuery" />
                <Input v-model="sort" :placeholder="t('pages.dataBrowser.sort')" @keydown.enter="applyQuery" />
                <Button :disabled="!collection" @click="applyQuery">{{ t('pages.dataBrowser.run') }}</Button>
              </div>
              <div v-if="schema" class="mt-3 flex flex-wrap gap-1">
                <Button v-for="field in schema.fields" :key="field.name" size="sm" variant="ghost" @click="appendField(field.name)">{{ field.name }}</Button>
              </div>
              <div v-if="history.length" class="mt-3 flex flex-wrap gap-1">
                <Button v-for="item in history" :key="item" size="sm" variant="outline" @click="filter = item; applyQuery()">{{ item }}</Button>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardContent class="flex flex-wrap gap-3 p-4 sm:p-5">
              <label v-for="column in columns" :key="column" class="flex items-center gap-1.5 text-xs">
                <Switch :model-value="visibleColumns.includes(column)" @update:model-value="toggleColumn(column, $event)" />{{ column }}
              </label>
            </CardContent>
          </Card>

          <Card>
            <Table>
              <TableHeader><TableRow>
                <TableHead v-for="column in shownColumns" :key="column">{{ column }}</TableHead>
                <TableHead v-if="!readonly">{{ t('common.actions') }}</TableHead>
              </TableRow></TableHeader>
              <SbAsyncRegion
                as="tbody"
                :columns="shownColumns.length + (readonly ? 0 : 1)"
                :pending="loading"
                :show-empty="!loading && rows.length === 0"
                :show-error="Boolean(loadError)"
                :error="loadError"
                :empty-title="t('pages.dataBrowser.empty')"
                @retry="load"
              >
                <TableRow v-for="row in rows" :key="row.id">
                  <TableCell v-for="column in shownColumns" :key="column" class="min-w-36 align-top">
                    <span v-if="isSystemColumn(column)" class="font-mono text-xs">{{ systemValue(row, column) }}</span>
                    <span v-else-if="readonly || fieldFor(column)?.readonly" class="text-sm">{{ displayValue(drafts[row.id]?.[column]) }}</span>
                    <Switch v-else-if="fieldFor(column)?.type === 'bool'" :model-value="boolValue(drafts[row.id]?.[column])" @update:model-value="setCell(row.id, column, $event)" />
                    <Select v-else-if="fieldFor(column)?.type === 'select'" :model-value="stringValue(drafts[row.id]?.[column])" @update:model-value="setCell(row.id, column, $event)">
                      <SelectTrigger><SelectValue /></SelectTrigger>
                      <SelectContent><SelectItem v-for="option in fieldFor(column)?.options.values || []" :key="option" :value="option">{{ option }}</SelectItem></SelectContent>
                    </Select>
                    <Textarea v-else-if="fieldFor(column)?.type === 'json'" :model-value="stringValue(drafts[row.id]?.[column])" rows="3" @update:model-value="setCell(row.id, column, $event)" />
                    <Input
                      v-else
                      :type="inputType(column)"
                      :model-value="stringValue(drafts[row.id]?.[column])"
                      @update:model-value="setCell(row.id, column, $event)"
                    />
                  </TableCell>
                  <TableCell v-if="!readonly"><Button size="sm" variant="outline" :disabled="savingId === row.id" @click="saveRow(row)">{{ t('common.save') }}</Button></TableCell>
                </TableRow>
              </SbAsyncRegion>
            </Table>
            <TablePager variant="footer" :page="page" :page-size="pageSize" :total="total" :page-count="pageCount" @update:page="goToPage" />
          </Card>
        </main>
      </div>

      <CollectionSchemaModal v-model:open="schemaOpen" :project-id="projectId" :database-id="databaseId" :collection="collection" @saved="load" />
      <ViewCollectionModal v-model:open="viewOpen" :project-id="projectId" :database-id="databaseId" @saved="reloadCollections" />
      <CollectionImportModal v-model:open="importOpen" :project-id="projectId" :database-id="databaseId" :collection="collection" @saved="load" />
    </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { toast } from 'vue-sonner'
import { api } from '@/services/api'
import type { ApiError, CollectionField, CollectionInfo, CollectionSchema, DatabaseItem, DocumentItem } from '@/services/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { errorMessage } from '@/utils/format'
import PageContainer from '@/components/PageContainer.vue'
import ProjectScope from '@/components/ProjectScope.vue'
import CollectionSchemaModal from '@/components/databases/CollectionSchemaModal.vue'
import ViewCollectionModal from '@/components/databases/ViewCollectionModal.vue'
import CollectionImportModal from '@/components/databases/CollectionImportModal.vue'
import SchemaPanel from '@/components/databases/SchemaPanel.vue'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import TablePager from '@/components/TablePager.vue'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { RefreshCwIcon } from '@lucide/vue'

const { t } = useI18n()
const project = useProjectStore()
const auth = useAuthStore()
const { projectId, isAdmin } = storeToRefs(project)
const databases = ref<DatabaseItem[]>([])
const databaseId = ref('')
const databasesLoading = ref(false)
const databasesError = ref('')
const collections = ref<CollectionInfo[]>([])
const collectionsLoading = ref(false)
const collectionsError = ref('')
const collection = ref('')
const schema = ref<CollectionSchema | null>(null)
const rows = ref<DocumentItem[]>([])
const drafts = ref<Record<string, Record<string, unknown>>>({})
const visibleColumns = ref<string[]>([])
const filter = ref('')
const sort = ref('')
const history = ref<string[]>([])
const page = ref(1)
const pageSize = 20
const totalItems = ref(0)
const loading = ref(false)
const loadError = ref('')
const savingId = ref('')
const schemaOpen = ref(false)
const viewOpen = ref(false)
const importOpen = ref(false)
let databasesRequest = 0
let collectionsRequest = 0
let documentsRequest = 0

const selectedDatabase = computed(() => databases.value.find((item) => item.id === databaseId.value))
const isSqlDatabase = computed(() => selectedDatabase.value?.dataModel === 'sql')
const selectedCollection = computed(() => collections.value.find((item) => item.name === collection.value))
const readonly = computed(() => isAdmin.value || !auth.canWrite || selectedCollection.value?.schemaMode === 'view')
const columns = computed(() => {
  const names = new Set<string>(['id', 'created_at', 'updated_at'])
  for (const field of schema.value?.fields || []) if (!field.hidden) names.add(field.name)
  for (const row of rows.value) for (const name of Object.keys(row.data)) names.add(name)
  return [...names]
})
const shownColumns = computed(() => visibleColumns.value.filter((column) => columns.value.includes(column)))
const total = computed(() => totalItems.value)
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

watch(columns, (value) => {
  visibleColumns.value = visibleColumns.value.filter((column) => value.includes(column))
  for (const column of value) if (!visibleColumns.value.includes(column)) visibleColumns.value.push(column)
}, { immediate: true })
watch(projectId, () => { void loadDatabases() }, { immediate: true })
watch(databaseId, () => { void loadCollections() })
watch(collection, () => {
  page.value = 1
  visibleColumns.value = []
  void load()
})

function clearDocuments() {
  schema.value = null
  rows.value = []
  drafts.value = {}
  visibleColumns.value = []
  totalItems.value = 0
  page.value = 1
  loading.value = false
  loadError.value = ''
}
function clearCollections() {
  collections.value = []
  collection.value = ''
  collectionsLoading.value = false
  collectionsError.value = ''
  clearDocuments()
}
async function loadDatabases(preserveSelection = false) {
  const currentProject = projectId.value
  const request = ++databasesRequest
  collectionsRequest++
  documentsRequest++
  databasesLoading.value = true
  databasesError.value = ''
  if (!preserveSelection) {
    databaseId.value = ''
    clearCollections()
  }
  if (!currentProject) {
    databases.value = []
    databaseId.value = ''
    clearCollections()
    databasesLoading.value = false
    return
  }
  try {
    const list = await api.databases.list(currentProject)
    if (request !== databasesRequest || currentProject !== projectId.value) return
    databases.value = list.filter((item) => item.status === 'ready')
    const nextId = preserveSelection && databases.value.some((item) => item.id === databaseId.value)
      ? databaseId.value
      : databases.value[0]?.id || ''
    if (databaseId.value !== nextId) databaseId.value = nextId
    else await loadCollections(preserveSelection)
  } catch (error) {
    if (request !== databasesRequest || currentProject !== projectId.value) return
    databases.value = []
    databaseId.value = ''
    clearCollections()
    databasesError.value = errorMessage(error, t('pages.dataBrowser.databaseLoadFailed'))
  } finally {
    if (request === databasesRequest) databasesLoading.value = false
  }
}
async function loadCollections(preserveSelection = false) {
  const currentProject = projectId.value
  const currentDatabase = databaseId.value
  const request = ++collectionsRequest
  documentsRequest++
  if (!currentProject || !currentDatabase || isSqlDatabase.value) {
    clearCollections()
    return
  }
  if (!preserveSelection) {
    collection.value = ''
    clearDocuments()
  }
  collectionsLoading.value = true
  collectionsError.value = ''
  try {
    const items = await api.db.collectionItems(currentProject, currentDatabase)
    if (request !== collectionsRequest || currentProject !== projectId.value || currentDatabase !== databaseId.value) return
    collections.value = items
    const nextCollection = preserveSelection && items.some((item) => item.name === collection.value)
      ? collection.value
      : items[0]?.name || ''
    if (collection.value !== nextCollection) collection.value = nextCollection
    else await load()
  } catch (error) {
    if (request !== collectionsRequest || currentProject !== projectId.value || currentDatabase !== databaseId.value) return
    collections.value = []
    collection.value = ''
    clearDocuments()
    collectionsError.value = errorMessage(error, t('pages.dataBrowser.collectionLoadFailed'))
  } finally {
    if (request === collectionsRequest) collectionsLoading.value = false
  }
}
function reloadCollections() {
  return loadCollections(true)
}
function refresh() {
  return loadDatabases(true)
}
function selectDatabase(value: unknown) {
  const id = typeof value === 'string' ? value : ''
  if (!id || id === databaseId.value || !databases.value.some((item) => item.id === id)) return
  databaseId.value = id
}
function selectCollection(name: string) {
  collection.value = name
}
function fieldFor(column: string): CollectionField | undefined {
  return schema.value?.fields.find((field) => field.name === column)
}
function isSystemColumn(column: string): boolean {
  return column === 'id' || column === 'created_at' || column === 'updated_at'
}
function systemValue(row: DocumentItem, column: string): string {
  return column === 'id' ? row.id : column === 'created_at' ? row.createdAt : row.updatedAt
}
function displayValue(value: unknown): string {
  if (value === undefined || value === null) return ''
  return typeof value === 'string' ? value : JSON.stringify(value)
}
function stringValue(value: unknown): string {
  return displayValue(value)
}
function boolValue(value: unknown): boolean {
  return value === true
}
function inputType(column: string): string {
  const type = fieldFor(column)?.type
  if (type === 'number') return 'number'
  if (type === 'date') return 'datetime-local'
  if (type === 'email') return 'email'
  if (type === 'url') return 'url'
  return 'text'
}
function setCell(id: string, column: string, value: string | number | boolean) {
  const field = fieldFor(column)
  let next: unknown = value
  if (field?.type === 'number') next = value === '' ? null : Number(value)
  if (field?.type === 'date' && value) next = new Date(value).toISOString()
  if (field?.type === 'json') {
    try { next = JSON.parse(String(value)) } catch { next = value }
  }
  if (!drafts.value[id]) drafts.value[id] = {}
  drafts.value[id][column] = next
}
function toggleColumn(column: string, visible: boolean) {
  if (visible && !visibleColumns.value.includes(column)) visibleColumns.value.push(column)
  if (!visible) visibleColumns.value = visibleColumns.value.filter((item) => item !== column)
}
function appendField(name: string) {
  filter.value += (filter.value && !filter.value.endsWith(' ') ? ' && ' : '') + name
}
function applyQuery() {
  page.value = 1
  if (filter.value && !history.value.includes(filter.value)) history.value = [filter.value, ...history.value].slice(0, 5)
  void load()
}
function goToPage(value: number) {
  if (value < 1 || value > pageCount.value) return
  page.value = value
  void load()
}
async function loadSchema() {
  const currentProject = projectId.value
  const currentDatabase = databaseId.value
  const currentCollection = collection.value
  if (!currentProject || !currentDatabase || !currentCollection) {
    schema.value = null
    return
  }
  try {
    const result = await api.db.getCollectionSchema(currentProject, currentDatabase, currentCollection)
    if (currentProject === projectId.value && currentDatabase === databaseId.value && currentCollection === collection.value) {
      schema.value = result
    }
  } catch (error) {
    if ((error as ApiError).status === 404) {
      if (currentProject === projectId.value && currentDatabase === databaseId.value && currentCollection === collection.value) {
        schema.value = null
      }
    } else {
      throw error
    }
  }
}
async function load() {
  const request = ++documentsRequest
  const currentProject = projectId.value
  const currentDatabase = databaseId.value
  const currentCollection = collection.value
  const currentPage = page.value
  if (!currentProject || !currentDatabase || !currentCollection) {
    clearDocuments()
    return
  }
  loading.value = true
  loadError.value = ''
  try {
    await loadSchema()
    if (request !== documentsRequest) return
    const result = await api.db.documents(currentProject, currentDatabase, currentCollection, {
      limit: pageSize,
      offset: (currentPage - 1) * pageSize,
      filter: filter.value || undefined,
      sort: sort.value || undefined,
      count: true
    })
    if (request !== documentsRequest || currentProject !== projectId.value || currentDatabase !== databaseId.value || currentCollection !== collection.value) return
    rows.value = result.items
    totalItems.value = result.total ?? ((currentPage - 1) * pageSize + result.items.length)
    drafts.value = Object.fromEntries(result.items.map((row) => [row.id, { ...row.data }]))
  } catch (error) {
    if (request === documentsRequest) loadError.value = errorMessage(error, t('pages.dataBrowser.loadFailed'))
  } finally {
    if (request === documentsRequest) loading.value = false
  }
}
async function saveRow(row: DocumentItem) {
  const payload: Record<string, unknown> = {}
  for (const [name, value] of Object.entries(drafts.value[row.id] || {})) {
    if (JSON.stringify(value) !== JSON.stringify(row.data[name])) payload[name] = value
  }
  if (!Object.keys(payload).length) return
  savingId.value = row.id
  try {
    await api.db.patch(projectId.value, databaseId.value, collection.value, row.id, payload)
    toast.success(t('pages.dataBrowser.saved'))
    await load()
  } catch (error) {
    toast.error(errorMessage(error, t('pages.dataBrowser.saveFailed')))
  } finally {
    savingId.value = ''
  }
}
async function exportData(format: 'csv' | 'json') {
  const currentCollection = collection.value
  if (!projectId.value || !databaseId.value || !currentCollection) return
  try {
    const blob = await api.db.exportCollection(projectId.value, databaseId.value, currentCollection, format, { filter: filter.value || undefined, sort: sort.value || undefined })
    const url = URL.createObjectURL(blob)
    const link = window.document.createElement('a')
    link.href = url
    link.download = `${currentCollection}.${format}`
    link.click()
    URL.revokeObjectURL(url)
  } catch (error) {
    toast.error(errorMessage(error, t('pages.dataBrowser.exportFailed')))
  }
}
</script>
