<template>
  <PageContainer title="S3 对象存储" subtitle="对象的上传、浏览与删除">
    <a-card class="sb-card">
      <div class="sb-toolbar">
        <a-input
          v-model:value="projectId"
          style="width: 200px"
          placeholder="项目 ID"
          allow-clear
        />
        <a-input
          v-model:value="prefix"
          style="min-width: 240px"
          placeholder="前缀筛选，如 images/"
          allow-clear
          @press-enter="load"
        >
          <template #prefix><CloudUploadOutlined /></template>
        </a-input>
        <a-button @click="load">
          <template #icon><ReloadOutlined /></template>
          刷新
        </a-button>
        <a-upload :show-upload-list="false" :custom-request="handleUpload">
          <a-button type="primary">
            <template #icon><UploadOutlined /></template>
            上传对象
          </a-button>
        </a-upload>
      </div>
    </a-card>

    <a-card class="sb-card" title="对象列表">
      <a-table
        :columns="columns"
        :data-source="objects"
        :loading="loading"
        row-key="key"
        :pagination="{ pageSize: 10, size: 'small', showTotal: (t: number) => `共 ${t} 条` }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'key'">
            <span class="sb-key"><FileOutlined /> {{ record.key }}</span>
          </template>
          <template v-else-if="column.key === 'size'">
            {{ formatBytes(record.size) }}
          </template>
          <template v-else-if="column.key === 'lastModified'">
            {{ formatTime(record.lastModified) }}
          </template>
          <template v-else-if="column.key === 'ops'">
            <a-button type="link" size="small" @click="open(record.key)">
              <template #icon><EyeOutlined /></template>
              打开
            </a-button>
            <a-popconfirm title="确认删除该对象？" @confirm="remove(record.key)">
              <a-button type="link" danger size="small">
                <template #icon><DeleteOutlined /></template>
                删除
              </a-button>
            </a-popconfirm>
          </template>
        </template>
        <template #emptyText>
          <a-empty description="暂无对象" />
        </template>
      </a-table>
    </a-card>
  </PageContainer>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import {
  CloudUploadOutlined,
  ReloadOutlined,
  UploadOutlined,
  DeleteOutlined,
  EyeOutlined,
  FileOutlined
} from '@ant-design/icons-vue'
import { api } from '../services/api'
import type { S3Object } from '../services/api'
import { formatBytes, formatTime } from '../utils/format'
import PageContainer from '../components/PageContainer.vue'

const columns = [
  { title: 'Key', dataIndex: 'key', key: 'key', ellipsis: true },
  { title: '大小', dataIndex: 'size', key: 'size', width: 120 },
  { title: '修改时间', dataIndex: 'lastModified', key: 'lastModified', width: 180 },
  { title: '操作', key: 'ops', width: 170 }
]

const projectId = ref('proj-01')
const prefix = ref('')
const objects = ref<S3Object[]>([])
const loading = ref(false)

async function load() {
  if (!projectId.value.trim()) {
    message.warning('请先填写项目 ID')
    return
  }
  loading.value = true
  try {
    objects.value = await api.s3.list(projectId.value.trim(), prefix.value || undefined)
  } catch (e: any) {
    message.error(e?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function remove(key: string) {
  try {
    await api.s3.remove(projectId.value.trim(), key)
    message.success('删除成功')
    await load()
  } catch (e: any) {
    message.error(e?.message || '删除失败')
  }
}

async function open(key: string) {
  try {
    const { url } = await api.s3.presign(projectId.value.trim(), key)
    window.open(url, '_blank')
  } catch (e: any) {
    message.error(e?.message || '生成链接失败')
  }
}

async function handleUpload({ file, onSuccess, onError }: any) {
  try {
    await api.s3.upload(projectId.value.trim(), (file as File).name, file as File)
    message.success(`${(file as File).name} 上传成功`)
    onSuccess('ok')
    await load()
  } catch (e: any) {
    message.error(e?.message || '上传失败')
    onError(e)
  }
}

onMounted(load)
</script>

<style scoped>
.sb-key {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--sb-text);
  font-family: 'JetBrains Mono', Menlo, Consolas, monospace;
  font-size: 13px;
}
</style>
