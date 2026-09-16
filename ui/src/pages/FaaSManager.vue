<template>
  <PageContainer title="云函数" subtitle="函数包的上传部署与调用测试（后端能力未就绪）">
    <a-alert
      type="warning"
      show-icon
      message="后端暂无 FaaS 模块"
      description="FaaS 相关接口（/faas/*）在后端尚未立项实现（proto-http.md §3.8）。本页当前仅 Mock 模式可演示；菜单已隐藏此入口。"
      style="border-radius: var(--sb-radius)"
    />

    <a-card class="sb-card" title="部署函数">
      <div class="sb-toolbar">
        <a-upload :show-upload-list="false" :before-upload="() => false" @change="onFileChange">
          <a-button>
            <template #icon><UploadOutlined /></template>
            选择函数包
          </a-button>
        </a-upload>
        <a-input v-model:value="funcName" style="min-width: 240px" placeholder="函数名称" />
        <a-button type="primary" :disabled="!file || !funcName" :loading="deploying" @click="deploy">
          <template #icon><RocketOutlined /></template>
          部署
        </a-button>
        <a-button :loading="loading" @click="load">
          <template #icon><ReloadOutlined /></template>
          刷新
        </a-button>
        <a-tag v-if="file" color="success" class="sb-file-hint">
          <PaperClipOutlined /> {{ file.name }}
        </a-tag>
      </div>
    </a-card>

    <a-card class="sb-card" title="函数列表">
      <a-table
        :columns="columns"
        :data-source="funcs"
        :loading="loading"
        row-key="name"
        :pagination="pagination"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'name'">
            <span class="sb-mono"><CodeOutlined /> {{ record.name }}</span>
          </template>
          <template v-else-if="column.key === 'version'">
            <a-tag color="orange">{{ record.version }}</a-tag>
          </template>
          <template v-else-if="column.key === 'updatedAt'">
            {{ formatTime(record.updatedAt) }}
          </template>
          <template v-else-if="column.key === 'ops'">
            <a-button type="link" size="small" @click="openInvoke(record)">
              <template #icon><PlayCircleOutlined /></template>
              调用
            </a-button>
          </template>
        </template>
        <template #emptyText>
          <SbEmptyState description="暂无函数" />
        </template>
      </a-table>
    </a-card>

    <a-modal
      v-model:open="invokeVisible"
      :title="`测试调用 · ${current?.name || ''}`"
      :confirm-loading="invoking"
      @ok="invoke"
    >
      <a-textarea
        v-model:value="payload"
        :rows="6"
        placeholder='{"k":"v"}'
        :status="jsonError ? 'error' : ''"
      />
      <div v-if="jsonError" class="sb-json-error">{{ jsonError }}</div>
      <template v-if="result">
        <a-divider style="margin: 12px 0">调用结果</a-divider>
        <SbCodeBlock :value="result" max-height="240px" />
      </template>
    </a-modal>
  </PageContainer>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import {
  UploadOutlined,
  RocketOutlined,
  ReloadOutlined,
  PlayCircleOutlined,
  CodeOutlined,
  PaperClipOutlined
} from '@ant-design/icons-vue'
import { api } from '../services/api'
import type { FaasFunction } from '../services/api'
import { usePagination } from '../composables/usePagination'
import { formatTime } from '../utils/format'
import PageContainer from '../components/PageContainer.vue'
import SbCodeBlock from '../components/SbCodeBlock.vue'
import SbEmptyState from '../components/SbEmptyState.vue'

const columns = [
  { title: '名称', dataIndex: 'name', key: 'name' },
  { title: '版本', dataIndex: 'version', key: 'version', width: 120 },
  { title: '更新时间', dataIndex: 'updatedAt', key: 'updatedAt', width: 180 },
  { title: '操作', key: 'ops', width: 110 }
]

const pagination = usePagination()

const funcs = ref<FaasFunction[]>([])
const loading = ref(false)
const file = ref<File | null>(null)
const funcName = ref('')
const deploying = ref(false)
const invokeVisible = ref(false)
const invoking = ref(false)
const payload = ref('')
const jsonError = ref('')
const current = ref<FaasFunction | null>(null)
const result = ref<unknown>(null)

watch(payload, (v) => {
  if (!v.trim()) {
    jsonError.value = ''
    return
  }
  try {
    JSON.parse(v)
    jsonError.value = ''
  } catch (e) {
    jsonError.value = `JSON 格式错误：${e instanceof Error ? e.message : String(e)}`
  }
})

function onFileChange(info: { file: { originFileObj?: File } & File }) {
  file.value = info.file.originFileObj || info.file
}

async function load() {
  loading.value = true
  try {
    funcs.value = await api.faas.list()
  } catch (e) {
    message.error(e instanceof Error ? e.message : '加载失败')
  } finally {
    loading.value = false
  }
}

async function deploy() {
  if (!file.value || !funcName.value) return
  deploying.value = true
  try {
    const fn = await api.faas.deploy(funcName.value, file.value)
    message.success(`${fn.name} 部署成功（${fn.version}）`)
    file.value = null
    funcName.value = ''
    await load()
  } catch (e) {
    message.error(e instanceof Error ? e.message : '部署失败')
  } finally {
    deploying.value = false
  }
}

function openInvoke(record: FaasFunction) {
  current.value = record
  result.value = null
  payload.value = ''
  invokeVisible.value = true
}

async function invoke() {
  if (!current.value) return
  let body: unknown = {}
  try {
    body = JSON.parse(payload.value || '{}')
  } catch {
    jsonError.value = 'JSON 格式错误，请检查输入'
    return
  }
  invoking.value = true
  try {
    result.value = await api.faas.invoke(current.value.name, body)
    message.success('调用成功')
  } catch (e) {
    message.error(e instanceof Error ? e.message : '调用失败')
  } finally {
    invoking.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.sb-file-hint {
  margin-inline-end: 0;
}
.sb-json-error {
  color: var(--sb-danger);
  font-size: var(--sb-fs-xs);
  margin-top: 6px;
}
</style>
