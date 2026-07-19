<template>
  <PageContainer title="S3 对象存储" subtitle="对象的上传、浏览与删除">
    <a-card class="sb-card">
      <div class="sb-toolbar">
        <a-input
          v-model:value="prefix"
          style="min-width:240px"
          placeholder="前缀筛选"
          allow-clear
        >
          <template #prefix><CloudUploadOutlined /></template>
        </a-input>
        <a-button @click="load">
          <template #icon><ReloadOutlined /></template>
          刷新
        </a-button>
        <a-upload :show-upload-list="false" :customRequest="handleUpload">
          <a-button type="primary">
            <template #icon><UploadOutlined /></template>
            上传对象
          </a-button>
        </a-upload>
      </div>
    </a-card>

    <a-card class="sb-card" title="对象列表">
      <a-alert v-if="err" type="error" :message="err" show-icon style="margin-bottom:12px" />
      <a-table :data-source="objects" :loading="loading" row-key="key" :pagination="{ pageSize: 10, size: 'small' }">
        <a-table-column title="Key" dataIndex="key" />
        <a-table-column title="大小" dataIndex="size" :width="140" />
        <a-table-column title="操作" :width="160" :customRender="renderOps" />
      </a-table>
    </a-card>
  </PageContainer>
</template>
<script setup lang="ts">
import { ref, h, onMounted } from 'vue'
import {
  CloudUploadOutlined,
  ReloadOutlined,
  UploadOutlined,
  DeleteOutlined,
  EyeOutlined
} from '@ant-design/icons-vue'
import { api } from '../services/api'
import PageContainer from '../components/PageContainer.vue'

const prefix = ref('')
const objects = ref<any[]>([])
const loading = ref(false)
const err = ref('')

async function load() {
  loading.value = true
  err.value = ''
  try {
    objects.value = await api.s3.list(prefix.value || undefined)
  } catch (e: any) {
    err.value = e?.message || '加载失败'
  }
  loading.value = false
}

function renderOps({ record }: any) {
  return h('div', { style: 'display:flex;gap:8px' }, [
    h(
      'a-button',
      { type: 'link', size: 'small', danger: true, onClick: () => remove(record.key) },
      () => [h(DeleteOutlined), ' 删除']
    ),
    h(
      'a-button',
      { type: 'link', size: 'small', onClick: () => open(record.key) },
      () => [h(EyeOutlined), ' 打开']
    )
  ])
}

async function remove(key: string) {
  try {
    await api.s3.remove(key)
    await load()
  } catch (e: any) {
    err.value = e?.message || '删除失败'
  }
}

async function open(key: string) {
  try {
    const { url } = await api.s3.presign(key)
    window.open(url, '_blank')
  } catch (e: any) {
    err.value = e?.message || '生成链接失败'
  }
}

async function handleUpload({ file, onSuccess, onError }: any) {
  try {
    await api.s3.upload((file as File).name, file as File)
    onSuccess('ok')
    await load()
  } catch (e: any) {
    onError(e)
  }
}
onMounted(load)
</script>
