<template>
  <SbModal
    :open="open"
    :title="title"
    :width="720"
    @update:open="emit('update:open', $event)"
  >
    <div class="flex flex-col gap-3">
      <div class="flex gap-2">
        <Button v-if="!readonly" size="sm" @click="emit('add-document')">
          <PlusIcon data-icon="inline-start" />
          {{ t('components.documentList.addDocument') }}
        </Button>
        <Button size="sm" variant="outline" :disabled="pending" @click="load">
          <Spinner v-if="pending" data-icon="inline-start" />
          <RefreshCwIcon v-else data-icon="inline-start" />
          {{ t('common.refresh') }}
        </Button>
      </div>
      <div class="overflow-x-auto rounded-lg border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="sb-col-id">ID</TableHead>
              <TableHead class="max-w-md text-left">{{ t('components.documentList.colData') }}</TableHead>
              <TableHead class="w-28">{{ t('components.documentList.colActions') }}</TableHead>
            </TableRow>
          </TableHeader>
          <SbAsyncRegion
            as="tbody"
            :columns="3"
            :pending="pending"
            :show-skeleton="showSkeleton"
            :show-empty="showEmpty"
            :show-error="showError"
            :refreshing="refreshing"
            :error="error"
            @retry="load"
          >
            <template #empty>{{ t('components.documentList.empty') }}</template>
            <TableRow v-for="record in paged" :key="record.id">
              <TableCell class="sb-mono font-medium truncate text-xs">{{ record.id }}</TableCell>
              <TableCell class="text-left">
                <SbCodeBlock :value="docFields(record)" max-height="160px" />
              </TableCell>
              <TableCell>
                <ConfirmAction
                  v-if="!readonly"
                  :title="t('components.documentList.deleteConfirm')"
                  @confirm="removeRow(record.id)"
                >
                  <Button variant="destructiveGhost" size="sm">{{ t('common.del') }}</Button>
                </ConfirmAction>
                <span v-else class="text-xs text-muted-foreground">{{ t('common.readonly') }}</span>
              </TableCell>
            </TableRow>
          </SbAsyncRegion>
        </Table>
      </div>
      <TablePager
        :page="page"
        :page-size="pageSize"
        :total="total"
        :page-count="pageCount"
        @update:page="page = $event"
      />
    </div>
    <template #footer>
      <Button variant="outline" @click="emit('update:open', false)">{{ t('common.close') }}</Button>
    </template>
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
import {
  Table,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { api } from '../../services/api'
import type { DbRow } from '../../services/api'
import { useLoadState } from '../../composables/useLoadState'
import { usePagination } from '../../composables/usePagination'
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
  /** 只读模式（admin 系统库）：隐藏新增文档入口。 */
  readonly?: boolean
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  'add-document': []
}>()

const { t } = useI18n()

const rows = ref<DbRow[]>([])
const title = computed(() =>
  props.collection
    ? t('components.documentList.titleOf', { collection: props.collection })
    : t('components.documentList.title')
)
const { pending, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({
  fallback: 'components.documentList.loadFailed'
})
const { page, pageSize, total, pageCount, items: paged } = usePagination(rows)

function docFields(row: DbRow): Record<string, unknown> {
  const { id, ...rest } = row
  void id
  return rest
}

function load() {
  if (!props.databaseId || !props.collection) return Promise.resolve()
  return run(async () => {
    const data = await api.db.rows(props.projectId, props.databaseId, props.collection)
    rows.value = data
    return data.length > 0
  })
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

watch(
  () => [props.open, props.projectId, props.databaseId, props.collection, props.reloadToken],
  () => {
    if (props.open) void load()
  },
  { immediate: true }
)
</script>
