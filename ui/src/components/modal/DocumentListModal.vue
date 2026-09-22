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
        <Button size="sm" variant="outline" :disabled="loading" @click="load">
          <Spinner v-if="loading" data-icon="inline-start" />
          <RefreshCwIcon v-else data-icon="inline-start" />
          刷新
        </Button>
      </div>
      <div class="overflow-x-auto rounded-lg border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="w-[200px]">ID</TableHead>
              <TableHead>数据</TableHead>
              <TableHead class="w-24">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-if="loading && !paged.length">
              <TableCell colspan="3">
                <div class="flex flex-col gap-2 py-2">
                  <Skeleton class="h-8 w-full" />
                  <Skeleton class="h-8 w-2/3" />
                </div>
              </TableCell>
            </TableRow>
            <TableEmpty v-else-if="!paged.length" :colspan="3">暂时未查询到数据</TableEmpty>
            <TableRow v-for="record in paged" :key="record.id">
              <TableCell class="sb-mono font-medium truncate text-xs">{{ record.id }}</TableCell>
              <TableCell>
                <SbCodeBlock :value="docFields(record)" max-height="160px" />
              </TableCell>
              <TableCell>
                <ConfirmAction v-if="!readonly" title="确认删除该文档？" @confirm="removeRow(record.id)">
                  <Button variant="destructiveGhost" size="sm">删除</Button>
                </ConfirmAction>
                <span v-else class="text-xs text-muted-foreground">只读</span>
              </TableCell>
            </TableRow>
          </TableBody>
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
import { ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { PlusIcon, RefreshCwIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableBody,
  TableCell,
  TableEmpty,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { api } from '../../services/api'
import type { DbRow } from '../../services/api'
import { usePagination } from '../../composables/usePagination'
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
const loading = ref(false)
const title = ref('文档')
const { page, pageSize, total, pageCount, items: paged } = usePagination(rows)

function docFields(row: DbRow): Record<string, unknown> {
  const { id, ...rest } = row
  void id
  return rest
}

async function load() {
  if (!props.databaseId || !props.collection) return
  loading.value = true
  try {
    rows.value = await api.db.rows(props.projectId, props.databaseId, props.collection)
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '数据加载失败')
  } finally {
    loading.value = false
  }
}

async function removeRow(id: string) {
  try {
    await api.db.remove(props.projectId, props.databaseId, props.collection, id)
    toast.success('删除成功')
    await load()
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '删除失败')
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
