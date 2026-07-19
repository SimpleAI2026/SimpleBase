<template>
  <PageContainer title="数据管理" subtitle="集合与文档的增删查改">
    <a-card class="sb-card">
      <div class="sb-toolbar">
        <a-select
          v-model:value="current"
          style="min-width:240px"
          placeholder="选择集合"
          :options="collections.map(c => ({ label: c, value: c }))"
          @change="loadRows"
        >
          <template #suffixIcon><DatabaseOutlined /></template>
        </a-select>
        <a-button type="primary" @click="showCreate = true" :disabled="!current">
          <template #icon><PlusOutlined /></template>
          新增
        </a-button>
        <a-button @click="loadRows" :disabled="!current">
          <template #icon><ReloadOutlined /></template>
          刷新
        </a-button>
      </div>
    </a-card>

    <a-card class="sb-card" title="文档列表">
      <a-alert v-if="err" type="error" :message="err" show-icon style="margin-bottom:12px" />
      <a-table
        :data-source="rows"
        :loading="loading"
        row-key="id"
        :pagination="{ pageSize: 10, size: 'small' }"
      >
        <a-table-column title="ID" dataIndex="id" :width="240" />
        <a-table-column title="数据" :customRender="renderJson" />
        <a-table-column title="操作" :width="100" :customRender="renderOps" />
      </a-table>
    </a-card>

    <a-modal v-model:open="showCreate" title="新增数据" @ok="createRow" @cancel="showCreate = false">
      <a-input-textarea v-model:value="payload" :rows="8" placeholder='{"k":"v"}' />
    </a-modal>
  </PageContainer>
</template>
<script setup lang="ts">
import { ref, h, onMounted } from 'vue'
import { DatabaseOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons-vue'
import { api } from '../services/api'
import PageContainer from '../components/PageContainer.vue'

const collections = ref<string[]>([])
const current = ref<string>()
const rows = ref<any[]>([])
const loading = ref(false)
const showCreate = ref(false)
const payload = ref('')
const err = ref('')

function renderJson({ record }: any) {
  return JSON.stringify(record)
}
function renderOps({ record }: any) {
  return h(
    'a-button',
    { type: 'link', danger: true, size: 'small', onClick: () => removeRow(record.id) },
    '删除'
  )
}
async function loadCollections() {
  err.value = ''
  try {
    collections.value = await api.db.collections()
  } catch (e: any) {
    err.value = e?.message || '加载失败'
  }
}
async function loadRows() {
  if (!current.value) return
  loading.value = true
  err.value = ''
  try {
    rows.value = await api.db.rows(current.value)
  } catch (e: any) {
    err.value = e?.message || '加载失败'
  }
  loading.value = false
}
async function createRow() {
  if (!current.value) return
  try {
    await api.db.insert(current.value, JSON.parse(payload.value || '{}'))
    showCreate.value = false
    payload.value = ''
    await loadRows()
  } catch (e: any) {
    err.value = e?.message || '创建失败'
  }
}
async function removeRow(id: string) {
  if (!current.value) return
  try {
    await api.db.remove(current.value, id)
    await loadRows()
  } catch (e: any) {
    err.value = e?.message || '删除失败'
  }
}
onMounted(() => {
  loadCollections()
})
</script>
