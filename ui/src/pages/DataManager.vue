<template>
  <PageContainer title="数据管理" subtitle="集合与文档的增删查改">
    <a-card class="sb-card">
      <div class="sb-toolbar">
        <a-select
          v-model:value="current"
          style="min-width: 240px"
          placeholder="选择集合"
          :options="collections.map((c) => ({ label: c, value: c }))"
          @change="loadRows"
        >
          <template #suffixIcon><DatabaseOutlined /></template>
        </a-select>
        <a-button @click="showCreateCollection = true">
          <template #icon><PlusOutlined /></template>
          新建集合
        </a-button>
        <a-button type="primary" :disabled="!current" @click="showCreate = true">
          <template #icon><PlusOutlined /></template>
          新增文档
        </a-button>
        <a-button :disabled="!current" @click="loadRows">
          <template #icon><ReloadOutlined /></template>
          刷新
        </a-button>
      </div>
    </a-card>

    <a-card class="sb-card" title="文档列表">
      <a-table
        :columns="columns"
        :data-source="rows"
        :loading="loading"
        row-key="id"
        :pagination="{ pageSize: 10, size: 'small', showTotal: (t: number) => `共 ${t} 条` }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'data'">
            <pre class="sb-json">{{ formatJson(record) }}</pre>
          </template>
          <template v-else-if="column.key === 'ops'">
            <a-button type="link" size="small" @click="openEditor(record)">
              <template #icon><EditOutlined /></template>
              编辑
            </a-button>
            <a-popconfirm title="确认删除该文档？" @confirm="removeRow(record.id)">
              <a-button type="link" danger size="small">
                <template #icon><DeleteOutlined /></template>
                删除
              </a-button>
            </a-popconfirm>
          </template>
        </template>
        <template #emptyText>
          <a-empty description="暂无数据" />
        </template>
      </a-table>
    </a-card>

    <a-modal v-model:open="showCreateCollection" title="新建集合" :confirm-loading="creatingCollection" @ok="createCollection">
      <a-input v-model:value="collectionName" placeholder="例如：users" />
    </a-modal>

    <a-modal
      v-model:open="showCreate"
      :title="editingId ? '编辑数据' : '新增数据'"
      :confirm-loading="creating"
      @ok="createRow"
      @cancel="resetEditor"
    >
      <a-textarea
        v-model:value="payload"
        :rows="8"
        placeholder='{"k":"v"}'
        :status="jsonError ? 'error' : ''"
      />
      <div v-if="jsonError" class="sb-json-error">{{ jsonError }}</div>
    </a-modal>
  </PageContainer>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { DatabaseOutlined, PlusOutlined, ReloadOutlined, DeleteOutlined, EditOutlined } from '@ant-design/icons-vue'
import { api } from '../services/api'
import type { DbRow } from '../services/api'
import { formatJson } from '../utils/format'
import PageContainer from '../components/PageContainer.vue'

const columns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 200, ellipsis: true },
  { title: '数据', key: 'data' },
  { title: '操作', key: 'ops', width: 110 }
]

const collections = ref<string[]>([])
const current = ref<string>()
const rows = ref<DbRow[]>([])
const loading = ref(false)
const showCreate = ref(false)
const showCreateCollection = ref(false)
const creating = ref(false)
const creatingCollection = ref(false)
const collectionName = ref('')
const editingId = ref('')
const payload = ref('')
const jsonError = ref('')

watch(payload, (v) => {
  if (!v.trim()) {
    jsonError.value = ''
    return
  }
  try {
    JSON.parse(v)
    jsonError.value = ''
  } catch (e: any) {
    jsonError.value = `JSON 格式错误：${e.message}`
  }
})

async function loadCollections() {
  try {
    collections.value = await api.db.collections()
    if (!current.value && collections.value.length) {
      current.value = collections.value[0]
      await loadRows()
    }
  } catch (e: any) {
    message.error(e?.message || '集合加载失败')
  }
}

async function createCollection() {
  const name = collectionName.value.trim()
  if (!name) {
    message.warning('请输入集合名称')
    return
  }
  creatingCollection.value = true
  try {
    await api.db.createCollection(name)
    current.value = name
    collectionName.value = ''
    showCreateCollection.value = false
    await loadCollections()
    await loadRows()
    message.success('集合创建成功')
  } catch (e: any) {
    message.error(e?.message || '集合创建失败')
  } finally {
    creatingCollection.value = false
  }
}

async function loadRows() {
  if (!current.value) return
  loading.value = true
  try {
    rows.value = await api.db.rows(current.value)
  } catch (e: any) {
    message.error(e?.message || '数据加载失败')
  } finally {
    loading.value = false
  }
}

function openEditor(row: DbRow) {
  editingId.value = row.id
  payload.value = JSON.stringify(row, null, 2)
  jsonError.value = ''
  showCreate.value = true
}

function resetEditor() {
  editingId.value = ''
  payload.value = ''
  jsonError.value = ''
}

async function createRow() {
  if (!current.value) return
  let doc: Record<string, unknown>
  try {
    doc = JSON.parse(payload.value || '{}')
  } catch {
    jsonError.value = 'JSON 格式错误，请检查输入'
    return
  }
  creating.value = true
  try {
    if (editingId.value) {
      await api.db.update(current.value, editingId.value, doc)
      message.success('保存成功')
    } else {
      await api.db.insert(current.value, doc)
      message.success('创建成功')
    }
    showCreate.value = false
    editingId.value = ''
    payload.value = ''
    await loadRows()
  } catch (e: any) {
    message.error(e?.message || '创建失败')
  } finally {
    creating.value = false
  }
}

async function removeRow(id: string) {
  if (!current.value) return
  try {
    await api.db.remove(current.value, id)
    message.success('删除成功')
    await loadRows()
  } catch (e: any) {
    message.error(e?.message || '删除失败')
  }
}

onMounted(loadCollections)
</script>

<style scoped>
.sb-json {
  margin: 0;
  padding: 8px 10px;
  background: var(--sb-bg-soft);
  border-radius: 6px;
  font-family: 'JetBrains Mono', Menlo, Consolas, monospace;
  font-size: 12px;
  line-height: 1.5;
  max-height: 180px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
.sb-json-error {
  color: var(--sb-danger);
  font-size: 12px;
  margin-top: 6px;
}
</style>
