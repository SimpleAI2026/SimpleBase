<template>
  <div class="py-1">
    <div v-if="loading && !collections.length" class="flex flex-col gap-2 py-2">
      <Skeleton class="h-8 w-full" />
      <Skeleton class="h-8 w-2/3" />
      <Skeleton class="h-8 w-1/2" />
    </div>
    <div v-else class="overflow-x-auto rounded-lg border border-border/70 bg-card/60">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="min-w-44 max-w-80">集合名称</TableHead>
            <TableHead class="w-52 text-right">操作</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableEmpty v-if="!rows.length" :colspan="2">
            <SbEmptyState :description="readonly ? '暂无数据表' : '暂无集合'" :action-text="readonly ? undefined : '新建集合'" @action="!readonly && emit('create-collection')" />
          </TableEmpty>
          <TableRow v-for="record in rows" :key="record.name">
            <TableCell class="sb-mono max-w-80 truncate font-medium" :title="record.name">{{ record.name }}</TableCell>
            <TableCell class="w-52 text-right">
              <div class="flex items-center justify-end gap-1">
                <Button variant="ghost" size="sm" @click="emit('view-data', record.name)">查看数据</Button>
                <Button v-if="!readonly" variant="ghost" size="sm" @click="emit('add-document', record.name)">新增文档</Button>
              </div>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableEmpty, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { api } from '../../services/api'
import type { DatabaseItem } from '../../services/api'
import SbEmptyState from '../SbEmptyState.vue'

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
const loading = ref(false)
const rows = computed(() => collections.value.map((name) => ({ name })))

async function load() {
  loading.value = true
  try {
    collections.value = await api.db.collections(props.projectId, props.database.id)
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '集合加载失败')
  } finally {
    loading.value = false
  }
}

watch(
  () => [props.projectId, props.database.id, props.reloadToken],
  () => {
    void load()
  },
  { immediate: true }
)
</script>
