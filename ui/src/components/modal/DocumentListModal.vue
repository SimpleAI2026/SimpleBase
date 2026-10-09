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
          新增文档
        </Button>
        <Button size="sm" variant="outline" :disabled="pending" @click="load">
          <Spinner v-if="pending" data-icon="inline-start" />
          <RefreshCwIcon v-else data-icon="inline-start" />
          刷新
        </Button>
      </div>
      <div class="overflow-x-auto rounded-lg border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="sb-col-id">ID</TableHead>
              <TableHead class="max-w-md text-left">数据</TableHead>
              <TableHead class="w-28">操作</TableHead>
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
            <template #empty>暂时未查询到数据</template>
            <TableRow v-for="record in paged" :key="record.id">
              <TableCell class="sb-mono font-medium truncate text-xs">{{ record.id }}</TableCell>
              <TableCell class="text-left">
                <SbCodeBlock :value="docFields(record)" max-height="160px" />
              </TableCell>
              <TableCell>
                <ConfirmAction v-if="!readonly" title="确认删除该文档？" @confirm="removeRow(record.id)">
                  <Button variant="destructiveGhost" size="sm">删除</Button>
                </ConfirmAction>
                <span v-else class="text-xs text-muted-foreground">只读</span>
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
      <Button variant="outline" @click="emit('update:open', false)">关闭</Button>
    </template>
  </SbModal>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { ref, watch } from 'vue'
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

const rows = ref<DbRow[]>([])
const title = ref('文档')
const { pending, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({
  fallback: '数据加载失败'
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
    toast.success('删除成功')
    await load()
  } catch (e) {
    toast.error(errorMessage(e, '删除失败'))
  }
}

watch(
  () => [props.open, props.projectId, props.databaseId, props.collection, props.reloadToken],
  () => {
    title.value = props.collection ? `${props.collection} 的文档` : '文档'
    if (props.open) void load()
  },
  { immediate: true }
)
</script>
