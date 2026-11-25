<template>
  <ProjectScope>
  <PageContainer :subtitle="t('pages.databases.subtitle')">
    <SbListCard
      :title="isAdmin ? t('pages.databases.cardTitleAdmin') : t('pages.databases.cardTitle')"
      :description="isAdmin ? t('pages.databases.cardDescAdmin') : t('pages.databases.cardDesc', { n: databases.length })"
      :pager="{ page, pageSize, total, pageCount }"
      @update:page="page = $event"
    >
      <template #actions>
        <SbRefreshButton :pending="pending" @click="doLoad" />
        <Button v-if="canWrite" size="sm" @click="openCreate">
          <PlusIcon data-icon="inline-start" />
          {{ t('pages.databases.create') }}
        </Button>
      </template>
      <div class="p-0">
        <SbAsyncRegion
          class="p-4 md:hidden"
          block="cards"
          :state="loadState"
          :empty-description="isAdmin ? t('pages.databases.emptySystem') : t('pages.databases.empty')"
          :empty-action-text="canWrite ? t('pages.databases.create') : undefined"
          @retry="doLoad"
          @empty-action="openCreate"
        >
        <div class="space-y-3" :aria-label="t('pages.databases.mobileListAria')">
          <article v-for="record in paged" :key="record.id" class="rounded-xl border border-border bg-card p-4 shadow-xs">
            <div class="flex min-w-0 items-start justify-between gap-2">
              <div class="min-w-0">
                <div class="flex min-w-0 items-center gap-2">
                  <DatabaseIcon aria-hidden="true" class="size-4 shrink-0 text-primary" />
                  <span class="sb-mono truncate font-semibold" :title="record.name">{{ record.name }}</span>
                  <Badge variant="outline">{{ showSqlPanel(record) ? t('pages.databases.badgeSql') : t('pages.databases.badgeCollection') }}</Badge>
                </div>
                <Badge class="mt-2" :variant="statusBadgeVariant(record.status)">{{ statusText(record.status) }}</Badge>
              </div>
              <Button variant="outline" size="sm" :disabled="!isReady(record)" :aria-expanded="expanded.has(record.id)" :aria-controls="`db-mobile-${record.id}`" @click="toggleExpand(record)">
                {{ expanded.has(record.id) ? t('pages.databases.collapseData') : t('pages.databases.viewData') }}
              </Button>
            </div>
            <div class="mt-4 flex flex-wrap items-center gap-2 border-t border-border/70 pt-3">
              <Button variant="outline" size="sm" :disabled="!isReady(record)" @click="openSql(record)">SQL</Button>
              <Button v-if="canWrite && !isSqlDatabase(record)" variant="outline" size="sm" :disabled="!isReady(record)" @click="openCreateCollection(record)">{{ t('pages.databases.createCollection') }}</Button>
              <ConfirmAction v-if="canWrite" :disabled="record.status === 'deleting'" :title="t('pages.databases.deleteTooltip')" @confirm="removeDb(record)">
                <Button variant="destructiveGhost" size="sm" :disabled="record.status === 'deleting'">{{ t('common.del') }}</Button>
              </ConfirmAction>
              <span v-else-if="isAdmin" class="inline-flex items-center gap-1 text-xs text-muted-foreground">{{ t('pages.databases.protected') }}</span>
            </div>
            <div class="mt-3 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 text-xs text-muted-foreground">
              <span>ID</span><span class="sb-mono min-w-0 break-all">{{ record.id }}</span>
              <span>{{ t('pages.databases.dataVolume') }}</span><span>{{ formatCount(record.documentCount) }}</span>
              <span>{{ t('pages.databases.createdAt') }}</span><span>{{ formatTime(record.createdAt) }}</span>
            </div>
            <div v-if="expanded.has(record.id)" :id="`db-mobile-${record.id}`" class="mt-4 border-t border-border pt-4">
              <SchemaPanel v-if="showSqlPanel(record)" :project-id="projectStore.id" :database="record" :readonly="isAdmin" />
              <DataTabs v-else :project-id="projectStore.id" :database="record" :reload-token="collectionReload[record.id] || 0" :readonly="isAdmin" @view-data="(c) => openDocList(record, c)" @add-document="(c) => openKv(record, c)" @create-collection="openCreateCollection(record)" />
            </div>
          </article>
        </div>
        </SbAsyncRegion>
        <div class="hidden md:block">
          <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="w-12 text-center" />
              <TableHead class="sb-col-name">{{ t('pages.databases.name') }}</TableHead>
              <TableHead class="sb-col-id">ID</TableHead>
              <TableHead class="sb-col-sm">{{ t('pages.databases.status') }}</TableHead>
              <TableHead class="w-24">{{ t('pages.databases.dataVolume') }}</TableHead>
              <TableHead class="sb-col-md">{{ t('pages.databases.createdAt') }}</TableHead>
              <TableHead class="sb-col-act min-w-36">{{ t('pages.databases.actions') }}</TableHead>
            </TableRow>
          </TableHeader>
          <SbAsyncRegion
            as="tbody"
            :columns="7"
            :state="loadState"
            :empty-description="isAdmin ? t('pages.databases.emptySystem') : t('pages.databases.empty')"
            :empty-action-text="canWrite ? t('pages.databases.create') : undefined"
            @retry="doLoad"
            @empty-action="openCreate"
          >
            <template v-for="record in paged" :key="record.id">
              <TableRow>
                <TableCell class="text-center">
                  <Tooltip>
                    <TooltipTrigger as-child>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        data-row-expand
                        :disabled="!isReady(record)"
                        :aria-label="t('pages.databases.viewRecordData', { action: expanded.has(record.id) ? t('pages.databases.collapse') : t('pages.databases.viewData'), name: record.name })"
                        :aria-expanded="expanded.has(record.id)"
                        :aria-controls="`db-desktop-${record.id}`"
                        @click="toggleExpand(record)"
                      >
                        <ChevronRightIcon class="transition-transform" :class="expanded.has(record.id) && 'rotate-90'" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>
                      {{ isReady(record) ? (expanded.has(record.id) ? t('pages.databases.collapse') : t('pages.databases.viewData')) : t('pages.databases.dbNotReady') }}
                    </TooltipContent>
                  </Tooltip>
                </TableCell>
                <TableCell class="sb-col-name">
                  <span class="sb-mono inline-flex items-center gap-1.5 font-medium text-foreground">
                    <DatabaseIcon class="size-4 shrink-0 text-primary" />
                    <span class="truncate">{{ record.name }}</span>
                    <Badge variant="outline">{{ showSqlPanel(record) ? t('pages.databases.badgeSql') : t('pages.databases.badgeCollection') }}</Badge>
                  </span>
                </TableCell>
                <TableCell class="sb-col-id">
                  <Tooltip>
                    <TooltipTrigger as-child>
                      <span class="sb-mono block max-w-full truncate text-xs text-muted-foreground">{{ record.id }}</span>
                    </TooltipTrigger>
                    <TooltipContent>{{ record.id }}</TooltipContent>
                  </Tooltip>
                </TableCell>
                <TableCell class="sb-col-sm">
                  <Badge :variant="statusBadgeVariant(record.status)">{{ statusText(record.status) }}</Badge>
                </TableCell>
                <TableCell class="w-24 tabular-nums text-sm">
                  <Tooltip>
                    <TooltipTrigger as-child>
                      <span class="cursor-default">{{ formatCount(record.documentCount) }}</span>
                    </TooltipTrigger>
                    <TooltipContent>
                      {{ record.documentCount === undefined ? t('pages.databases.countUnavailable') : t('pages.databases.rowsTotal', { n: record.documentCount }) }}
                    </TooltipContent>
                  </Tooltip>
                </TableCell>
                <TableCell class="sb-col-md text-xs text-muted-foreground">{{ formatTime(record.createdAt) }}</TableCell>
                <TableCell>
                  <div class="flex flex-wrap items-center justify-center gap-1">
                    <Tooltip>
                      <TooltipTrigger as-child>
                        <span>
                          <Button variant="ghost" size="sm" :disabled="!isReady(record)" @click="openSql(record)">
                            SQL
                          </Button>
                        </span>
                      </TooltipTrigger>
                      <TooltipContent>{{ isReady(record) ? (isAdmin ? t('pages.databases.sqlConsole') : t('pages.databases.sqlWorkbench')) : t('pages.databases.dbNotReady') }}</TooltipContent>
                    </Tooltip>
                    <template v-if="canWrite">
                      <Tooltip v-if="!isSqlDatabase(record)">
                        <TooltipTrigger as-child>
                          <span>
                            <Button variant="ghost" size="sm" :disabled="!isReady(record)" @click="openCreateCollection(record)">
                              {{ t('pages.databases.createCollection') }}
                            </Button>
                          </span>
                        </TooltipTrigger>
                        <TooltipContent>{{ isReady(record) ? t('pages.databases.createCollection') : t('pages.databases.dbNotReady') }}</TooltipContent>
                      </Tooltip>
                      <ConfirmAction
                        :disabled="record.status === 'deleting'"
                        :title="t('pages.databases.deleteTooltip')"
                        @confirm="removeDb(record)"
                      >
                        <Button variant="destructiveGhost" size="sm" :disabled="record.status === 'deleting'">
                          {{ t('common.del') }}
                        </Button>
                      </ConfirmAction>
                    </template>
                    <Tooltip v-else-if="isAdmin">
                      <TooltipTrigger as-child>
                        <span class="inline-flex items-center gap-1 text-xs text-muted-foreground">
                          {{ t('pages.databases.protected') }}
                        </span>
                      </TooltipTrigger>
                      <TooltipContent>{{ t('pages.databases.protectedTip') }}</TooltipContent>
                    </Tooltip>
                  </div>
                </TableCell>
              </TableRow>
              <TableRow v-if="expanded.has(record.id)" class="bg-muted/20 hover:bg-muted/20">
                <TableCell colspan="7" class="border-b-0 p-0 text-left">
                  <div :id="`db-desktop-${record.id}`" class="border-y border-border/70 bg-muted/25 px-6 py-4">
                    <SchemaPanel
                      v-if="showSqlPanel(record)"
                      :project-id="projectStore.id"
                      :database="record"
                      :readonly="isAdmin"
                    />
                    <DataTabs
                      v-else
                      :project-id="projectStore.id"
                      :database="record"
                      :reload-token="collectionReload[record.id] || 0"
                      :readonly="isAdmin"
                      @view-data="(c) => openDocList(record, c)"
                      @add-document="(c) => openKv(record, c)"
                      @create-collection="openCreateCollection(record)"
                    />
                  </div>
                </TableCell>
              </TableRow>
            </template>
          </SbAsyncRegion>
          </Table>
        </div>
        </div>
    </SbListCard>
    <SbModal
      v-model:open="createVisible"
      :title="t('pages.databases.createTitle')"
      :confirm-loading="creating"
      :ok-button-props="{ disabled: !!nameError && !!newName.trim() }"
      @ok="create"
    >
      <FieldGroup>
        <Field :data-invalid="nameError ? true : undefined">
          <FieldLabel for="db-name">{{ t('pages.databases.formName') }}</FieldLabel>
          <Input
            id="db-name"
            v-model="newName"
            :placeholder="t('pages.databases.namePlaceholder')"
            :aria-invalid="nameError ? true : undefined"
          />
          <FieldDescription v-if="nameError">{{ nameError }}</FieldDescription>
        </Field>
        <Field>
          <FieldLabel>{{ t('pages.databases.dataModel') }}</FieldLabel>
          <div class="flex flex-wrap gap-2">
            <Button type="button" size="sm" :variant="dataModel === 'collection' ? 'default' : 'outline'" @click="dataModel = 'collection'">{{ t('pages.databases.modelCollection') }}</Button>
            <Button type="button" size="sm" :variant="dataModel === 'sql' ? 'default' : 'outline'" @click="dataModel = 'sql'">{{ t('pages.databases.modelSql') }}</Button>
          </div>
          <FieldDescription>{{ t('pages.databases.modelHint') }}</FieldDescription>
        </Field>
        <Field v-if="dataModel === 'sql'">
          <FieldLabel for="db-init-sql">{{ t('pages.databases.initSql') }}</FieldLabel>
          <Textarea
            id="db-init-sql"
            v-model="initSql"
            :placeholder="t('pages.databases.initSqlPlaceholder')"
            class="min-h-28 font-mono text-xs"
          />
          <FieldDescription>{{ t('pages.databases.initSqlHint') }}</FieldDescription>
        </Field>
        <FieldError v-if="createError" class="whitespace-pre-wrap break-words">{{ createError }}</FieldError>
      </FieldGroup>
    </SbModal>

    <SqlWorkModal v-model:open="dialogOpen.sql" :project-id="projectStore.id" :database="activeDb" :readonly="isAdmin" />
    <CreateCollectionModal
      v-model:open="dialogOpen.createCollection"
      :project-id="projectStore.id"
      :database="activeDb"
      @created="onCollectionCreated"
    />
    <DocumentListModal
      v-model:open="dialogOpen.docList"
      :project-id="projectStore.id"
      :database-id="activeDb?.id || ''"
      :collection="activeCollection"
      :reload-token="docReload"
      :readonly="isAdmin"
      @add-document="onAddDocumentFromList"
    />
    <DocumentKvModal
      v-model:open="kvOpen"
      :project-id="projectStore.id"
      :database-id="activeDb?.id || ''"
      :collection="activeCollection"
      @created="onDocumentCreated"
    />
  </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { ChevronRightIcon, DatabaseIcon, PlusIcon } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import {
  Table,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { api } from '@/services/api'
import type { DatabaseItem } from '@/services/api'
import { useProjectStore } from '@/stores/project'
import { storeToRefs } from 'pinia'
import { useLoadState } from '@/composables/useLoadState'
import { useCanWrite } from '@/composables/useCanWrite'
import { useProjectReload } from '@/composables/useProjectReload'
import { usePagination } from '@/composables/usePagination'
import { statusBadgeVariant, statusText } from '@/utils/status'
import { formatCount, formatTime } from '@/utils/format'
import PageContainer from '@/components/PageContainer.vue'
import ProjectScope from '@/components/ProjectScope.vue'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import SbRefreshButton from '@/components/SbRefreshButton.vue'
import SbListCard from '@/components/SbListCard.vue'
import ConfirmAction from '@/components/ConfirmAction.vue'
import SbModal from '@/components/modal/SbModal.vue'
import SqlWorkModal from '@/components/modal/SqlWorkModal.vue'
import CreateCollectionModal from '@/components/modal/CreateCollectionModal.vue'
import DocumentListModal from '@/components/modal/DocumentListModal.vue'
import DocumentKvModal from '@/components/modal/DocumentKvModal.vue'
import DataTabs from '@/components/databases/DataTabs.vue'
import SchemaPanel from '@/components/databases/SchemaPanel.vue'

const projectStore = useProjectStore()
const { isAdmin } = storeToRefs(projectStore)
const canWrite = useCanWrite()
const { t } = useI18n()
const databases = ref<DatabaseItem[]>([])
const { page, pageSize, total, pageCount, items: paged } = usePagination(databases)

const createVisible = ref(false)
const creating = ref(false)
const createError = ref('')
const newName = ref('')
const dataModel = ref<'collection' | 'sql'>('collection')
const initSql = ref('')
/** 已展开的数据库 id（Set 的增删收敛在 toggleExpand / expand）。 */
const expanded = ref(new Set<string>())
const collectionReload = reactive<Record<string, number>>({})

/**
 * 互斥弹窗：SQL 工作台 / 新建集合 / 文档列表同一时间只开一个（一个 `dialog` 值 + 三个 v-model 视图）。
 * 文档列表与「新增文档」会叠开（列表在下、新增在上），所以 kvOpen 不在此列。
 */
type DialogKind = 'sql' | 'createCollection' | 'docList'
const dialog = ref<DialogKind | ''>('')
const dialogOpen = reactive(
  Object.fromEntries(
    (['sql', 'createCollection', 'docList'] as const).map((kind) => [
      kind,
      computed({ get: () => dialog.value === kind, set: (open: boolean) => { dialog.value = open ? kind : '' } })
    ])
  ) as Record<DialogKind, boolean>
)
const kvOpen = ref(false)
const activeDb = ref<DatabaseItem | null>(null)
const activeCollection = ref('')
const docReload = ref(0)

const loadState = useLoadState({
  fallback: 'pages.databases.loadFailed'
})
const { pending, run } = loadState

function doLoad() {
  return run(async () => {
    const list = await api.databases.list(projectStore.id)
    databases.value = list
    return list.length > 0
  })
}

const nameError = computed(() => {
  const n = newName.value.trim()
  if (!n) return ''
  if (n.length > 63) return t('pages.databases.nameTooLong')
  if (/[\/\\\x00-\x1f]/.test(n)) return t('pages.databases.nameInvalid')
  return ''
})

function isReady(db: DatabaseItem) {
  return db.status === 'ready'
}

function isSqlDatabase(db: DatabaseItem) {
  return db.dataModel === 'sql'
}

function showSqlPanel(db: DatabaseItem) {
  return isAdmin.value || isSqlDatabase(db)
}

function toggleExpand(db: DatabaseItem) {
  if (!isReady(db)) return
  const next = new Set(expanded.value)
  if (!next.delete(db.id)) next.add(db.id)
  expanded.value = next
}

function openCreate() {
  newName.value = ''
  dataModel.value = 'collection'
  initSql.value = ''
  createError.value = ''
  createVisible.value = true
}

/** 设置当前操作对象并打开对应弹窗（collection 缺省保持上次值）。 */
function openDialog(kind: DialogKind, db: DatabaseItem, collection?: string) {
  activeDb.value = db
  if (collection !== undefined) activeCollection.value = collection
  dialog.value = kind
}

const openSql = (db: DatabaseItem) => openDialog('sql', db)
const openCreateCollection = (db: DatabaseItem) => openDialog('createCollection', db)
const openDocList = (db: DatabaseItem, collection: string) => openDialog('docList', db, collection)

function openKv(db: DatabaseItem, collection: string) {
  activeDb.value = db
  activeCollection.value = collection
  kvOpen.value = true
}

function onCollectionCreated() {
  if (!activeDb.value) return
  const id = activeDb.value.id
  collectionReload[id] = (collectionReload[id] || 0) + 1
  if (!expanded.value.has(id)) expanded.value = new Set(expanded.value).add(id)
}

function onAddDocumentFromList() {
  if (activeDb.value && activeCollection.value) {
    kvOpen.value = true
  }
}

function onDocumentCreated() {
  docReload.value += 1
  if (dialog.value !== 'docList' && activeDb.value && activeCollection.value) {
    dialog.value = 'docList'
  }
}

async function create() {
  const name = newName.value.trim()
  if (!name) {
    toast.warning(t('pages.databases.nameRequired'))
    return
  }
  if (nameError.value) {
    toast.warning(nameError.value)
    return
  }
  const sql = initSql.value.trim()
  createError.value = ''
  creating.value = true
  try {
    const db = await api.databases.create(projectStore.id, {
      name,
      dataModel: dataModel.value,
      initSql: sql ? sql : undefined
    })
    toast.success(t('pages.databases.createSuccess', { name: db.name, status: statusText(db.status) }))
    createVisible.value = false
    await doLoad()
  } catch (e) {
    createError.value = errorMessage(e, t('pages.databases.createFailed'))
    toast.error(createError.value)
  } finally {
    creating.value = false
  }
}

async function removeDb(db: DatabaseItem) {
  try {
    await api.databases.remove(projectStore.id, db.id)
    toast.success(t('pages.databases.deleteSubmitted', { name: db.name }))
    await doLoad()
  } catch (e) {
    toast.error(errorMessage(e, t('pages.databases.deleteFailed')))
  }
}

watch(databases, (list) => {
  const ready = new Set(list.filter(isReady).map((d) => d.id))
  expanded.value = new Set([...expanded.value].filter((id) => ready.has(id)))
})

useProjectReload(() => projectStore.id, doLoad, { onSwitch: () => { expanded.value = new Set() } })
</script>
