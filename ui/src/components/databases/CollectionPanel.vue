<template>
  <div class="pr-4 pl-0 py-2">
    <div v-if="loading && !collections.length" class="flex flex-col gap-2">
      <Skeleton class="h-8 w-full" />
      <Skeleton class="h-8 w-2/3" />
      <Skeleton class="h-8 w-1/2" />
    </div>
    <Table v-else>
      <TableHeader>
        <TableRow>
          <TableHead>集合</TableHead>
          <TableHead class="w-44">操作</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableEmpty v-if="!rows.length" :colspan="2">
          <SbEmptyState description="暂无集合" action-text="新建集合" @action="emit('create-collection')" />
        </TableEmpty>
        <TableRow v-for="record in rows" :key="record.name">
          <TableCell class="sb-mono">{{ record.name }}</TableCell>
          <TableCell>
            <div class="flex gap-1">
              <Button variant="ghost" size="sm" @click="emit('view-data', record.name)">查看数据</Button>
              <Button variant="ghost" size="sm" @click="emit('add-document', record.name)">新增文档</Button>
            </div>
          </TableCell>
        </TableRow>
      </TableBody>
    </Table>
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
