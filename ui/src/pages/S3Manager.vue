<template>
  <PageContainer title="S3 对象存储" subtitle="对象的上传、浏览与删除">
    <a-card class="sb-card">
      <div class="sb-toolbar">
        <ProjectPicker />
        <a-input
          v-model:value="prefix"
          style="min-width: 200px"
          placeholder="前缀筛选，如 images/"
          allow-clear
          @press-enter="load"
        >
          <template #prefix><CloudUploadOutlined /></template>
        </a-input>
        <a-button :loading="loading" @click="load">
          <template #icon><ReloadOutlined /></template>
          刷新
        </a-button>
        <a-upload :show-upload-list="false" :before-upload="handleBeforeUpload">
          <a-button type="primary" :loading="uploading">
            <template #icon><UploadOutlined /></template>
            上传对象
          </a-button>
        </a-upload>
        <a-progress
          v-if="uploading && uploadPercent > 0"
          type="circle"
          :percent="uploadPercent"
          :size="32"
        />
      </div>
    </a-card>

    <a-card class="sb-card" title="对象列表">
      <a-table
        :columns="columns"
        :data-source="objects"
        :loading="loading"
        row-key="key"
        :pagination="pagination"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'key'">
            <span class="sb-mono sb-key"><FileOutlined /> {{ record.key }}</span>
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
          <SbEmptyState description="暂无对象" action-text="上传对象" @action="triggerUpload" />
        </template>
      </a-table>
    </a-card>
  </PageContainer>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import axios from 'axios'
import {
  CloudUploadOutlined,
  ReloadOutlined,
  UploadOutlined,
  DeleteOutlined,
  EyeOutlined,
  FileOutlined
} from '@ant-design/icons-vue'
import { api, isMock } from '../services/api'
import type { S3Object } from '../services/api'
import { useProjectStore } from '../stores/project'
import { usePagination } from '../composables/usePagination'
import { formatBytes, formatTime } from '../utils/format'
import { getApiKey } from '../services/http'
import { baseURL } from '../services/http'
import PageContainer from '../components/PageContainer.vue'
import ProjectPicker from '../components/ProjectPicker.vue'
import SbEmptyState from '../components/SbEmptyState.vue'

const columns = [
  { title: 'Key', dataIndex: 'key', key: 'key', ellipsis: true },
  { title: '大小', dataIndex: 'size', key: 'size', width: 120 },
  { title: '修改时间', dataIndex: 'lastModified', key: 'lastModified', width: 180 },
  { title: '操作', key: 'ops', width: 170 }
]

const projectStore = useProjectStore()
const pagination = usePagination()

const prefix = ref('')
const objects = ref<S3Object[]>([])
const loading = ref(false)
const uploading = ref(false)
const uploadPercent = ref(0)

/** 体积上限（与后端 limits.max_request_bytes 对齐，config.yaml 默认 10MB；mock 模式放宽） */
const MAX_UPLOAD_BYTES = isMock ? 1024 * 1024 * 1024 : 10 * 1024 * 1024

/** 服务端 key 校验规则（objectstore.ValidateFileKey）：非空、相对路径、无 .. 段 */
function validateKey(key: string): string {
  if (!key) return 'key 不能为空'
  if (key.length > 1024) return 'key 不能超过 1024 字节'
  if (key.includes('\0')) return 'key 不能包含 NUL 字符'
  if (key.startsWith('/') || key.includes('\\')) return 'key 必须是相对路径'
  for (const seg of key.split('/')) {
    if (seg === '..' || seg === '.') return 'key 不能包含 . 或 .. 路径段'
  }
  return ''
}

function handleBeforeUpload(file: File) {
  // 预校验：体积（后端 BodyLimit 10MB）与 key 规则，避免上传完才失败
  if (file.size > MAX_UPLOAD_BYTES) {
    message.warning(`文件 ${(file.size / 1024 / 1024).toFixed(1)}MB 超过上限 ${MAX_UPLOAD_BYTES / 1024 / 1024}MB`)
    return false
  }
  const err = validateKey(file.name)
  if (err) {
    message.warning(`文件名不符合 key 规则：${err}`)
    return false
  }
  doUpload(file)
  return false // 阻止 antd 默认上传，由 doUpload 控制
}

/** 用独立 axios 请求以支持 onUploadProgress（api 抽象层不透传进度） */
function doUpload(file: File) {
  const pid = projectStore.id
  const fd = new FormData()
  fd.append('key', file.name)
  fd.append('file', file)
  uploading.value = true
  uploadPercent.value = 0
  if (isMock) {
    // mock 走 api 抽象（无进度）
    api.s3
      .upload(pid, file.name, file)
      .then(() => {
        message.success(`${file.name} 上传成功`)
        return load()
      })
      .catch((e) => message.error(e instanceof Error ? e.message : '上传失败'))
      .finally(() => {
        uploading.value = false
      })
    return
  }
  axios
    .post(`${baseURL}/v1/projects/${encodeURIComponent(pid)}/s3/objects`, fd, {
      headers: { Authorization: `Bearer ${getApiKey()}` },
      onUploadProgress: (e) => {
        if (e.total) uploadPercent.value = Math.round((e.loaded / e.total) * 100)
      }
    })
    .then(async () => {
      message.success(`${file.name} 上传成功`)
      await load()
    })
    .catch((e) => {
      const msg = e?.response?.data?.error?.message || e?.message || '上传失败'
      message.error(msg)
    })
    .finally(() => {
      uploading.value = false
    })
}

function triggerUpload() {
  // 空态 CTA：antd Upload 无法编程式触发，提示用户点击上传按钮
  message.info('请点击上方「上传对象」按钮选择文件')
}

async function load() {
  loading.value = true
  try {
    objects.value = await api.s3.list(projectStore.id, prefix.value || undefined)
  } catch (e) {
    message.error(e instanceof Error ? e.message : '加载失败')
  } finally {
    loading.value = false
  }
}

async function remove(key: string) {
  try {
    await api.s3.remove(projectStore.id, key)
    message.success('删除成功')
    await load()
  } catch (e) {
    message.error(e instanceof Error ? e.message : '删除失败')
  }
}

async function open(key: string) {
  try {
    const { url } = await api.s3.presign(projectStore.id, key)
    window.open(url, '_blank')
  } catch (e) {
    message.error(e instanceof Error ? e.message : '生成链接失败')
  }
}

onMounted(load)
watch(() => projectStore.id, () => {
  void load()
})
</script>

<style scoped>
.sb-key {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--sb-text);
}
</style>
