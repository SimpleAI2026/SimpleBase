<template>
  <ProjectScope>
  <PageContainer :subtitle="t('pages.databases.subtitle')">
    <Card>
      <CardHeader class="flex-wrap gap-3 border-b max-sm:[&_[data-slot=card-action]]:w-full max-sm:[&_[data-slot=card-action]]:justify-start">
        <CardTitle>{{ isAdmin ? t('pages.databases.cardTitleAdmin') : t('pages.databases.cardTitle') }}</CardTitle>
        <CardDescription>
          {{ isAdmin ? t('pages.databases.cardDescAdmin') : t('pages.databases.cardDesc', { n: databases.length }) }}
        </CardDescription>
        <CardAction v-if="!isAdmin">
          <div class="flex items-center gap-2">
            <Button variant="outline" size="sm" :disabled="pending" @click="doLoad">
              <Spinner v-if="pending" data-icon="inline-start" />
              <RefreshCwIcon v-else data-icon="inline-start" />
              {{ t('common.refresh') }}
            </Button>
            <Button size="sm" @click="openCreate">
              <PlusIcon data-icon="inline-start" />
              {{ t('pages.databases.create') }}
            </Button>
          </div>
        </CardAction>
        <CardAction v-else>
          <Button variant="outline" size="sm" :disabled="pending" @click="doLoad">
            <Spinner v-if="pending" data-icon="inline-start" />
            <RefreshCwIcon v-else data-icon="inline-start" />
            {{ t('common.refresh') }}
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent class="p-0">
        <SbAsyncRegion
          class="p-4 md:hidden"
          block="cards"
          :pending="pending"
          :show-skeleton="showSkeleton"
          :show-empty="showEmpty"
          :show-error="showError"
          :refreshing="refreshing"
          :error="error"
          :empty-description="isAdmin ? t('pages.databases.emptySystem') : t('pages.databases.empty')"
          :empty-action-text="isAdmin ? undefined : t('pages.databases.create')"
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
              <Button variant="outline" size="sm" :disabled="!isReady(record)" :aria-expanded="expandedRowKeys.includes(record.id)" :aria-controls="`db-mobile-${record.id}`" @click="toggleExpand(record)">
                {{ expandedRowKeys.includes(record.id) ? t('pages.databases.expand') : t('pages.databases.viewData') }}
              </Button>
            </div>
            <div class="mt-4 flex flex-wrap items-center gap-2 border-t border-border/70 pt-3">
              <Button variant="outline" size="sm" :disabled="!isReady(record)" @click="openSql(record)">SQL</Button>
              <Button v-if="!isAdmin && !isSqlDatabase(record)" variant="outline" size="sm" :disabled="!isReady(record)" @click="openCreateCollection(record)">{{ t('pages.databases.createCollection') }}</Button>
              <ConfirmAction v-if="!isAdmin" :disabled="record.status === 'deleting'" :title="t('pages.databases.deleteTooltip')" @confirm="removeDb(record)">
                <Button variant="destructiveGhost" size="sm" :disabled="record.status === 'deleting'">{{ t('common.del') }}</Button>
              </ConfirmAction>
              <span v-else class="inline-flex items-center gap-1 text-xs text-muted-foreground">{{ t('pages.databases.protected') }}</span>
            </div>
            <div class="mt-3 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 text-xs text-muted-foreground">
              <span>ID</span><span class="sb-mono min-w-0 break-all">{{ record.id }}</span>
              <span>{{ t('pages.databases.dataVolume') }}</span><span>{{ formatCount(record.documentCount) }}</span>
              <span>{{ t('pages.databases.createdAt') }}</span><span>{{ formatTime(record.createdAt) }}</span>
            </div>
            <div v-if="expandedRowKeys.includes(record.id)" :id="`db-mobile-${record.id}`" class="mt-4 border-t border-border pt-4">
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
            :pending="pending"
            :show-skeleton="showSkeleton"
            :show-empty="showEmpty"
            :show-error="showError"
            :refreshing="refreshing"
            :error="error"
            :empty-description="isAdmin ? t('pages.databases.emptySystem') : t('pages.databases.empty')"
            :empty-action-text="isAdmin ? undefined : t('pages.databases.create')"
            @retry="doLoad"
            @empty-action="openCreate"
          >
            <template v-for="record in paged" :key="record.id">
              <TableRow>
                <TableCell class="text-center">
                  <Tooltip>
                    <TooltipTrigger as-child>
                      <Button
                        variant="outline"
                        size="icon-sm"
                        class="rounded-full"
                        :disabled="!isReady(record)"
                        :aria-label="t('pages.databases.viewRecordData', { action: expandedRowKeys.includes(record.id) ? t('pages.databases.collapse') : t('pages.databases.viewData'), name: record.name })"
                        :aria-expanded="expandedRowKeys.includes(record.id)"
                        :aria-controls="`db-desktop-${record.id}`"
                        @click="toggleExpand(record)"
                      >
                        <MinusIcon v-if="expandedRowKeys.includes(record.id)" class="size-3" />
                        <PlusIcon v-else class="size-3" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>
                      {{ isReady(record) ? (expandedRowKeys.includes(record.id) ? t('pages.databases.collapse') : t('pages.databases.viewData')) : t('pages.databases.dbNotReady') }}
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
                    <template v-if="!isAdmin">
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
                    <Tooltip v-else>
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
              <TableRow v-if="expandedRowKeys.includes(record.id)" class="bg-muted/20 hover:bg-muted/20">
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
        <TablePager
          variant="footer"
          :page="page"
          :page-size="pageSize"
          :total="total"
          :page-count="pageCount"
          @update:page="page = $event"
        />
      </CardContent>
    </Card>

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

    <SqlWorkModal v-model:open="sqlOpen" :project-id="projectStore.id" :database="activeDb" :readonly="isAdmin" />
    <CreateCollectionModal
      v-model:open="createCollectionOpen"
      :project-id="projectStore.id"
      :database="activeDb"
      @created="onCollectionCreated"
    />
    <DocumentListModal
      v-model:open="docListOpen"
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
import {
  DatabaseIcon,
  MinusIcon,
  PlusIcon,
  RefreshCwIcon,
} from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { api } from '../services/api'
import type { DatabaseItem } from '../services/api'
import { useProjectStore } from '../stores/project'
import { storeToRefs } from 'pinia'
import { useLoadState } from '../composables/useLoadState'
import { usePagination } from '../composables/usePagination'
import { statusBadgeVariant, statusText } from '@/lib/status'
import { formatCount, formatTime } from '../utils/format'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import SbAsyncRegion from '../components/SbAsyncRegion.vue'
import ConfirmAction from '../components/ConfirmAction.vue'
import TablePager from '../components/TablePager.vue'
import SbModal from '../components/modal/SbModal.vue'
import SqlWorkModal from '../components/modal/SqlWorkModal.vue'
import CreateCollectionModal from '../components/modal/CreateCollectionModal.vue'
import DocumentListModal from '../components/modal/DocumentListModal.vue'
import DocumentKvModal from '../components/modal/DocumentKvModal.vue'
import DataTabs from '../components/databases/DataTabs.vue'
import SchemaPanel from '../components/databases/SchemaPanel.vue'

const projectStore = useProjectStore()
const { isAdmin } = storeToRefs(projectStore)
const { t } = useI18n()
const databases = ref<DatabaseItem[]>([])
const { page, pageSize, total, pageCount, items: paged } = usePagination(databases)

const createVisible = ref(false)
const creating = ref(false)
const createError = ref('')
const newName = ref('')
const dataModel = ref<'collection' | 'sql'>('collection')
const initSql = ref('')
const expandedRowKeys = ref<string[]>([])
const collectionReload = reactive<Record<string, number>>({})

const sqlOpen = ref(false)
const createCollectionOpen = ref(false)
const docListOpen = ref(false)
const kvOpen = ref(false)
const activeDb = ref<DatabaseItem | null>(null)
const activeCollection = ref('')
const docReload = ref(0)

const { pending, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({
  fallback: 'pages.databases.loadFailed'
})

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
  if (expandedRowKeys.value.includes(db.id)) {
    expandedRowKeys.value = expandedRowKeys.value.filter((id) => id !== db.id)
  } else {
    expandedRowKeys.value = [...expandedRowKeys.value, db.id]
  }
}

function openCreate() {
  newName.value = ''
  dataModel.value = 'collection'
  initSql.value = ''
  createError.value = ''
  createVisible.value = true
}

function openSql(db: DatabaseItem) {
  activeDb.value = db
  sqlOpen.value = true
}

function openCreateCollection(db: DatabaseItem) {
  activeDb.value = db
  createCollectionOpen.value = true
}

function openDocList(db: DatabaseItem, collection: string) {
  activeDb.value = db
  activeCollection.value = collection
  docListOpen.value = true
}

function openKv(db: DatabaseItem, collection: string) {
  activeDb.value = db
  activeCollection.value = collection
  kvOpen.value = true
}

function onCollectionCreated() {
  if (!activeDb.value) return
  const id = activeDb.value.id
  collectionReload[id] = (collectionReload[id] || 0) + 1
  if (!expandedRowKeys.value.includes(id)) {
    expandedRowKeys.value = [...expandedRowKeys.value, id]
  }
}

function onAddDocumentFromList() {
  if (activeDb.value && activeCollection.value) {
    kvOpen.value = true
  }
}

function onDocumentCreated() {
  docReload.value += 1
  if (!docListOpen.value && activeDb.value && activeCollection.value) {
    docListOpen.value = true
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
  expandedRowKeys.value = expandedRowKeys.value.filter((id) => ready.has(id))
})

void doLoad()
watch(
  () => projectStore.id,
  () => {
    expandedRowKeys.value = []
    void doLoad()
  }
)
</script>
