<template>
  <div class="py-1">
    <div class="overflow-x-auto rounded-lg border border-border/70 bg-card/60">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="min-w-44 max-w-80">集合名称</TableHead>
            <TableHead class="w-52">操作</TableHead>
          </TableRow>
        </TableHeader>
        <SbAsyncRegion
          as="tbody"
          :columns="2"
          :pending="pending"
          :show-skeleton="showSkeleton"
          :show-empty="showEmpty"
          :show-error="showError"
          :refreshing="refreshing"
          :error="error"
          :empty-description="readonly ? '暂无数据表' : '暂无集合'"
          :empty-action-text="readonly ? undefined : '新建集合'"
          @retry="load"
          @empty-action="emit('create-collection')"
        >
          <TableRow v-for="record in rows" :key="record.name">
            <TableCell class="sb-mono max-w-80 truncate font-medium" :title="record.name">{{ record.name }}</TableCell>
            <TableCell class="w-52">
              <div class="flex items-center justify-center gap-1">
                <Button variant="ghost" size="sm" @click="emit('view-data', record.name)">查看数据</Button>
                <Button v-if="!readonly" variant="ghost" size="sm" @click="emit('add-document', record.name)">新增文档</Button>
              </div>
            </TableCell>
          </TableRow>
        </SbAsyncRegion>
      </Table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Button } from '@/components/ui/button'
import { Table, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { api } from '../../services/api'
import type { DatabaseItem } from '../../services/api'
import SbAsyncRegion from '../SbAsyncRegion.vue'
import { useLoadState } from '../../composables/useLoadState'

const props = defineProps<{
  projectId: string
  database: DatabaseItem
  reloadToken?: number
  /** 只读模式（admin 系统库）：隐藏新增文档/新建集合等写入口。 */
  readonly?: boolean
}>()

const emit = defineEmits<{
  'view-data': [collection: string]
  'add-document': [collection: string]
  'create-collection': []
}>()

const collections = ref<string[]>([])
const rows = computed(() => collections.value.map((name) => ({ name })))
const { pending, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({
  fallback: '集合加载失败'
})

function load() {
  return run(async () => {
    const data = await api.db.collections(props.projectId, props.database.id)
    collections.value = data
    return data.length > 0
  })
}

watch(
  () => [props.projectId, props.database.id, props.reloadToken],
  () => {
    void load()
  },
  { immediate: true }
)
</script>
