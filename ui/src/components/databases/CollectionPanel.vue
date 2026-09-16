<template>
  <div class="sb-collection-panel">
    <a-skeleton :loading="loading && !collections.length" active :paragraph="{ rows: 3 }">
      <a-table
        :columns="columns"
        :data-source="rows"
        :loading="loading"
        :pagination="false"
        row-key="name"
        size="small"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'name'">
            <span class="sb-mono">{{ record.name }}</span>
          </template>
          <template v-else-if="column.key === 'ops'">
            <a-button type="link" size="small" @click="emit('view-data', record.name)">查看数据</a-button>
            <a-button type="link" size="small" @click="emit('add-document', record.name)">新增文档</a-button>
          </template>
        </template>
        <template #emptyText>
          <SbEmptyState description="暂无集合" action-text="新建集合" @action="emit('create-collection')" />
        </template>
      </a-table>
    </a-skeleton>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
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

const columns = [
  { title: '集合', dataIndex: 'name', key: 'name' },
  { title: '操作', key: 'ops', width: 180 }
]

const collections = ref<string[]>([])
const loading = ref(false)
const rows = computed(() => collections.value.map((name) => ({ name })))

async function load() {
  loading.value = true
  try {
    collections.value = await api.db.collections(props.projectId, props.database.id)
  } catch (e) {
    message.error(e instanceof Error ? e.message : '集合加载失败')
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

<style scoped>
.sb-collection-panel {
  padding: var(--sb-space-2) var(--sb-space-4) var(--sb-space-2) 0;
}
</style>
