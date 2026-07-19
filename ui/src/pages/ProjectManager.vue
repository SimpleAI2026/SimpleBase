<template>
  <PageContainer title="项目管理" subtitle="创建与维护应用项目">
    <a-card class="sb-card" title="项目列表">
      <template #extra>
        <a-button type="primary" @click="openCreate = true">
          <template #icon><PlusOutlined /></template>
          新建项目
        </a-button>
      </template>
      <a-alert v-if="err" type="error" :message="err" show-icon style="margin-bottom:12px" />
      <a-table :data-source="projects" row-key="id" :pagination="{ pageSize: 10, size: 'small' }">
        <a-table-column title="ID" dataIndex="id" :width="240" />
        <a-table-column title="名称" dataIndex="name" />
        <a-table-column title="操作" :width="120" :customRender="renderOps" />
      </a-table>
    </a-card>

    <a-modal v-model:open="openCreate" title="新建项目" @ok="create">
      <a-form :model="form" layout="vertical">
        <a-form-item label="名称"><a-input v-model:value="form.name" placeholder="输入项目名称" /></a-form-item>
        <a-form-item label="描述"><a-input v-model:value="form.desc" placeholder="可选" /></a-form-item>
      </a-form>
    </a-modal>
  </PageContainer>
</template>
<script setup lang="ts">
import { ref, h, onMounted } from 'vue'
import { PlusOutlined } from '@ant-design/icons-vue'
import { api } from '../services/api'
import PageContainer from '../components/PageContainer.vue'

const projects = ref<any[]>([])
const err = ref('')
const openCreate = ref(false)
const form = ref({ name: '', desc: '' })

function renderOps({ record }: any) {
  return h(
    'a-button',
    { type: 'link', size: 'small', onClick: () => update(record.id) },
    '更新'
  )
}
async function load() {
  try {
    projects.value = await api.project.list()
  } catch (e: any) {
    err.value = e?.message || '加载失败'
  }
}
async function create() {
  try {
    await api.project.create(form.value)
    openCreate.value = false
    form.value = { name: '', desc: '' }
    await load()
  } catch (e: any) {
    err.value = e?.message || '创建失败'
  }
}
async function update(id: string) {
  try {
    await api.project.update(id, { name: 'updated' })
    await load()
  } catch (e: any) {
    err.value = e?.message || '更新失败'
  }
}
onMounted(load)
</script>
