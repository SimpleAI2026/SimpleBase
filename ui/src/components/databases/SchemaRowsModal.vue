<template>
  <SbModal
    :open="open"
    :title="table || '表数据'"
    :description="readonly ? '只读' : undefined"
    :width="960"
    hide-footer
    @update:open="emit('update:open', $event)"
  >
    <div class="overflow-x-auto rounded-lg border border-border/70">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead v-for="column in displayColumns" :key="column.name" :class="alignClass(column)">
              {{ column.name }}
            </TableHead>
          </TableRow>
        </TableHeader>
        <SbAsyncRegion
          as="tbody"
          :columns="skeletonColumns"
          :rows="4"
          :pending="pending"
          :show-skeleton="showSkeleton"
          :show-empty="showEmpty"
          :show-error="showError"
          :refreshing="refreshing"
          :error="error"
          empty-description="这张表没有行"
          @retry="load(false)"
        >
          <TableRow v-for="(row, rowIndex) in rows" :key="rowIndex">
            <TableCell v-for="(column, index) in displayColumns" :key="column.name" :class="alignClass(column)">
              <span v-if="column.sensitive">已隐藏</span>
              <span v-else-if="row[index] == null" class="text-muted-foreground">NULL</span>
              <span v-else class="block max-w-80 truncate" :title="cellText(row[index])">{{ cellText(row[index]) }}</span>
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
      @update:page="onPage"
    />
  </SbModal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Table, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { api } from '@/services/api'
import type { SchemaColumn } from '@/services/api'
import { useLoadState } from '@/composables/useLoadState'
import SbAsyncRegion from '../SbAsyncRegion.vue'
import SbModal from '../modal/SbModal.vue'
import TablePager from '../TablePager.vue'

const pageSize = 50

const props = defineProps<{
  open: boolean
  projectId: string
  databaseId: string
  table: string
  readonly?: boolean
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

const page = ref(1)
const total = ref(0)
const columns = ref<SchemaColumn[]>([])
const rows = ref<unknown[][]>([])
const { pending, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({
  fallback: '加载表数据失败'
})

const displayColumns = computed((): SchemaColumn[] =>
  columns.value.length > 0 ? columns.value : [{ name: '列', type: '' }]
)
const skeletonColumns = computed(() => Math.max(columns.value.length, 4))
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

function alignClass(column: SchemaColumn): string {
  const type = (column.type ?? '').toUpperCase()
  if (type.includes('CHAR') || type.includes('JSON') || type.includes('BLOB') || type.includes('TEXT')) {
    return 'max-w-80 text-left'
  }
  return ''
}

function cellText(value: unknown): string {
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  return JSON.stringify(value)
}

function load(replace: boolean) {
  if (!props.open || !props.table) return Promise.resolve()
  return run(async () => {
    const result = await api.databases.tableRows(props.projectId, props.databaseId, props.table, {
      limit: pageSize,
      offset: (page.value - 1) * pageSize
    })
    columns.value = result.columns
    rows.value = result.rows
    total.value = result.total
    return result.rows.length > 0
  }, { replace })
}

function onPage(next: number) {
  if (next === page.value) return
  page.value = next
  void load(false)
}

watch(
  () => [props.open, props.table] as const,
  () => {
    if (!props.open || !props.table) return
    page.value = 1
    void load(true)
  },
  { immediate: true }
)
</script>
