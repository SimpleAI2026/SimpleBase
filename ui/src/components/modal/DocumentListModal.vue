<template>
  <SbModal :open="open" :title="title" :max-width="960" hide-footer @update:open="emit('update:open', $event)">
    <div class="flex flex-col gap-3">
      <div class="flex flex-wrap gap-2">
        <Button v-if="!readonly" size="sm" @click="emit('add-document')"><PlusIcon data-icon="inline-start" />{{ t('components.documentList.addDocument') }}</Button>
        <Button v-if="!readonly" size="sm" variant="outline" @click="openBatch">{{ t('components.documentList.batch') }}</Button>
        <Button size="sm" variant="outline" :disabled="pending" @click="load"><Spinner v-if="pending" data-icon="inline-start" /><RefreshCwIcon v-else data-icon="inline-start" />{{ t('common.refresh') }}</Button>
        <ConfirmAction v-if="!readonly" :title="t('components.documentList.truncateConfirm')" @confirm="truncate">
          <Button size="sm" variant="destructiveGhost">{{ t('components.documentList.truncate') }}</Button>
        </ConfirmAction>
      </div>
      <div class="grid gap-2 md:grid-cols-4">
        <Input v-model="filter" :placeholder="t('components.documentList.filterPlaceholder')" @keydown.enter="applyQuery" />
        <Input v-model="sort" :placeholder="t('components.documentList.sortPlaceholder')" @keydown.enter="applyQuery" />
        <Input v-model="fields" :placeholder="t('components.documentList.fieldsPlaceholder')" @keydown.enter="applyQuery" />
        <Button variant="secondary" @click="applyQuery">{{ t('components.documentList.apply') }}</Button>
      </div>
      <div class="overflow-x-auto rounded-lg border border-border">
        <Table>
          <TableHeader><TableRow>
            <TableHead class="sb-col-id">ID</TableHead>
            <TableHead class="max-w-md text-left">{{ t('components.documentList.colData') }}</TableHead>
            <TableHead class="w-40">{{ t('components.documentList.colActions') }}</TableHead>
          </TableRow></TableHeader>
          <SbAsyncRegion
            as="tbody" :columns="3" :pending="pending" :show-skeleton="showSkeleton"
            :show-empty="showEmpty" :show-error="showError" :refreshing="refreshing" :error="error" @retry="load"
          >
            <template #empty>{{ t('components.documentList.empty') }}</template>
            <TableRow v-for="record in rows" :key="record.id">
              <TableCell class="sb-mono truncate font-medium text-xs">{{ record.id }}</TableCell>
              <TableCell class="text-left"><SbCodeBlock :value="docFields(record)" max-height="160px" /></TableCell>
              <TableCell>
                <div class="flex justify-center gap-1">
                  <Button v-if="!readonly" variant="ghost" size="sm" @click="openPatch(record)">{{ t('common.edit') }}</Button>
                  <ConfirmAction v-if="!readonly" :title="t('components.documentList.deleteConfirm')" @confirm="removeRow(record.id)">
                    <Button variant="destructiveGhost" size="sm">{{ t('common.del') }}</Button>
                  </ConfirmAction>
                  <span v-else class="text-xs text-muted-foreground">{{ t('common.readonly') }}</span>
                </div>
              </TableCell>
            </TableRow>
          </SbAsyncRegion>
        </Table>
      </div>
      <TablePager :page="page" :page-size="pageSize" :total="total" :page-count="pageCount" @update:page="goToPage" />
      <div class="flex justify-end"><Button variant="outline" @click="emit('update:open', false)">{{ t('common.close') }}</Button></div>
    </div>
  </SbModal>

  <SbModal
    v-model:open="editorOpen"
    :title="editorMode === 'batch' ? t('components.documentList.batchTitle') : t('components.documentList.patchTitle')"
    :max-width="720"
    :confirm-loading="saving"
    :ok-text="t('common.save')"
    @ok="saveEditor"
  >
    <Field>
      <FieldLabel>{{ t('components.documentList.jsonPayload') }}</FieldLabel>
      <Textarea v-model="editorText" :rows="16" class="font-mono" />
    </Field>
  </SbModal>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { PlusIcon, RefreshCwIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Field, FieldLabel } from '@/components/ui/field'
import { Table, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { api } from '../../services/api'
import type { DbRow } from '../../services/api'
import { useLoadState } from '../../composables/useLoadState'
import SbAsyncRegion from '../SbAsyncRegion.vue'
import ConfirmAction from '../ConfirmAction.vue'
import TablePager from '../TablePager.vue'
import SbCodeBlock from '../SbCodeBlock.vue'
import SbModal from './SbModal.vue'

const props = defineProps<{
  open: boolean
  projectId: string
  databaseId: string
  collection: string
  reloadToken?: number
  readonly?: boolean
}>()
const emit = defineEmits<{
  'update:open': [value: boolean]
  'add-document': []
}>()
const { t } = useI18n()
const rows = ref<DbRow[]>([])
const page = ref(1)
const pageSize = 50
const totalItems = ref<number>()
const nextCursor = ref('')
const cursors = ref<string[]>([''])
const filter = ref('')
const sort = ref('')
const fields = ref('')
const editorOpen = ref(false)
const editorMode = ref<'patch' | 'batch'>('patch')
const editorId = ref('')
const editorText = ref('{}')
const saving = ref(false)
const title = computed(() => props.collection ? t('components.documentList.titleOf', { collection: props.collection }) : t('components.documentList.title'))
const total = computed(() => totalItems.value ?? (page.value - 1) * pageSize + rows.value.length + (nextCursor.value ? 1 : 0))
const pageCount = computed(() => nextCursor.value ? page.value + 1 : page.value)
const { pending, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({ fallback: 'components.documentList.loadFailed' })
function docFields(row: DbRow): Record<string, unknown> {
  const { id, created_at, updated_at, ...rest } = row
  void id
  void created_at
  void updated_at
  return rest
}
function load() {
  if (!props.databaseId || !props.collection) return Promise.resolve()
  return run(async () => {
    const data = await api.db.documents(props.projectId, props.databaseId, props.collection, {
      limit: pageSize,
      cursor: sort.value ? undefined : cursors.value[page.value - 1] || undefined,
      offset: sort.value ? (page.value - 1) * pageSize : undefined,
      filter: filter.value || undefined,
      sort: sort.value || undefined,
      fields: fields.value || undefined,
      count: true
    })
    rows.value = data.rows
    nextCursor.value = data.nextCursor ?? ''
    totalItems.value = data.total
    if (nextCursor.value) cursors.value[page.value] = nextCursor.value
    return data.rows.length > 0
  })
}
function applyQuery() {
  page.value = 1
  cursors.value = ['']
  totalItems.value = undefined
  void load()
}
function goToPage(value: number) {
  if (value < 1 || value > pageCount.value || value === page.value) return
  page.value = value
  void load()
}
async function removeRow(id: string) {
  try {
    await api.db.remove(props.projectId, props.databaseId, props.collection, id)
    toast.success(t('components.documentList.deleted'))
    await load()
  } catch (e) {
    toast.error(errorMessage(e, t('components.documentList.deleteFailed')))
  }
}
async function truncate() {
  try {
    await api.db.truncate(props.projectId, props.databaseId, props.collection)
    toast.success(t('components.documentList.truncated'))
    applyQuery()
  } catch (e) {
    toast.error(errorMessage(e, t('components.documentList.truncateFailed')))
  }
}
function openPatch(record: DbRow) {
  editorMode.value = 'patch'
  editorId.value = record.id
  editorText.value = JSON.stringify(docFields(record), null, 2)
  editorOpen.value = true
}
function openBatch() {
  editorMode.value = 'batch'
  editorText.value = '[\n  {}\n]'
  editorOpen.value = true
}
async function saveEditor() {
  saving.value = true
  try {
    const payload: unknown = JSON.parse(editorText.value)
    if (editorMode.value === 'batch') {
      if (!Array.isArray(payload)) throw new Error(t('components.documentList.batchArrayRequired'))
      await api.db.batch(props.projectId, props.databaseId, props.collection, payload as Record<string, unknown>[])
    } else {
      if (!payload || Array.isArray(payload) || typeof payload !== 'object') throw new Error(t('components.documentList.objectRequired'))
      await api.db.patch(props.projectId, props.databaseId, props.collection, editorId.value, payload as Record<string, unknown>)
    }
    toast.success(t('components.documentList.saved'))
    editorOpen.value = false
    await load()
  } catch (e) {
    toast.error(errorMessage(e, t('components.documentList.saveFailed')))
  } finally {
    saving.value = false
  }
}
watch(
  () => [props.open, props.projectId, props.databaseId, props.collection, props.reloadToken],
  () => {
    if (props.open) {
      page.value = 1
      cursors.value = ['']
      void load()
    }
  },
  { immediate: true }
)
</script>
