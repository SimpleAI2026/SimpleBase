<template>
  <SbModal
    :open="open"
    :title="title"
    :width="720"
    @update:open="emit('update:open', $event)"
  >
    <div class="sb-doc-toolbar">
      <a-button type="primary" size="small" @click="emit('add-document')">
        <template #icon><PlusOutlined /></template>
        新增文档
      </a-button>
      <a-button size="small" :loading="loading" @click="load">
        <template #icon><ReloadOutlined /></template>
        刷新
      </a-button>
    </div>
    <a-table
      :columns="columns"
      :data-source="rows"
      :loading="loading"
      row-key="id"
      :pagination="pagination"
      size="small"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'data'">
          <SbCodeBlock :value="docFields(record)" max-height="160px" />
        </template>
        <template v-else-if="column.key === 'ops'">
          <a-popconfirm title="确认删除该文档？" @confirm="removeRow(record.id)">
            <a-button type="link" danger size="small">删除</a-button>
          </a-popconfirm>
        </template>
      </template>
      <template #emptyText>
        <a-empty description="暂时未查询到数据" />
      </template>
    </a-table>
    <template #footer>
      <a-button @click="emit('update:open', false)">关闭</a-button>
    </template>
  </SbModal>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons-vue'
import { api } from '../../services/api'
import type { DbRow } from '../../services/api'
import { usePagination } from '../../composables/usePagination'
import SbCodeBlock from '../SbCodeBlock.vue'
import SbModal from './SbModal.vue'

const props = defineProps<{
  open: boolean
  projectId: string
  databaseId: string
  collection: string
  reloadToken?: number
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  'add-document': []
}>()

const columns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 200, ellipsis: true },
  { title: '数据', key: 'data' },
  { title: '操作', key: 'ops', width: 80 }
]

const pagination = usePagination()
const rows = ref<DbRow[]>([])
const loading = ref(false)
const title = ref('文档')

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
    message.error(e instanceof Error ? e.message : '数据加载失败')
  } finally {
    loading.value = false
  }
}

async function removeRow(id: string) {
  try {
    await api.db.remove(props.projectId, props.databaseId, props.collection, id)
    message.success('删除成功')
    await load()
  } catch (e) {
    message.error(e instanceof Error ? e.message : '删除失败')
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

<style scoped>
.sb-doc-toolbar {
  display: flex;
  gap: var(--sb-space-2);
  margin-bottom: var(--sb-space-3);
}
</style>
