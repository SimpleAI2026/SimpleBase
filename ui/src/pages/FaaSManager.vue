<template>
  <PageContainer title="FaaS 函数" subtitle="函数包的上传部署与调用测试">
    <a-card class="sb-card" title="部署函数">
      <div class="sb-toolbar">
        <a-upload :show-upload-list="false" :beforeUpload="() => false" @change="onFileChange">
          <a-button type="primary">
            <template #icon><UploadOutlined /></template>
            上传函数包
          </a-button>
        </a-upload>
        <a-input v-model:value="funcName" style="min-width:240px" placeholder="函数名称" />
        <a-button type="primary" @click="deploy" :disabled="!file || !funcName">
          <template #icon><RocketOutlined /></template>
          部署
        </a-button>
        <a-button @click="load">
          <template #icon><ReloadOutlined /></template>
          刷新
        </a-button>
        <span v-if="file" class="sb-file-hint">已选：{{ file.name }}</span>
      </div>
    </a-card>

    <a-card class="sb-card" title="函数列表">
      <a-alert v-if="err" type="error" :message="err" show-icon style="margin-bottom:12px" />
      <a-table :data-source="funcs" row-key="name" :pagination="{ pageSize: 10, size: 'small' }">
        <a-table-column title="名称" dataIndex="name" />
        <a-table-column title="版本" dataIndex="version" :width="160" />
        <a-table-column title="操作" :width="120" :customRender="renderOps" />
      </a-table>
    </a-card>

    <a-modal v-model:open="openInvoke" title="测试调用" @ok="invoke">
      <a-input-textarea v-model:value="payload" :rows="8" placeholder='{"k":"v"}' />
    </a-modal>
  </PageContainer>
</template>
<script setup lang="ts">
import { ref, h, onMounted } from 'vue'
import {
  UploadOutlined,
  RocketOutlined,
  ReloadOutlined,
  PlayCircleOutlined
} from '@ant-design/icons-vue'
import { api } from '../services/api'
import PageContainer from '../components/PageContainer.vue'

const funcs = ref<any[]>([])
const err = ref('')
const file = ref<File | null>(null)
const funcName = ref('')
const openInvoke = ref(false)
const payload = ref('')
const current = ref<any>(null)

function onFileChange(info: any) {
  file.value = info.file.originFileObj
}
async function load() {
  try {
    funcs.value = await api.faas.list()
  } catch (e: any) {
    err.value = e?.message || '加载失败'
  }
}
async function deploy() {
  if (!file.value) return
  try {
    await api.faas.deploy(funcName.value, file.value)
    file.value = null
    funcName.value = ''
    await load()
  } catch (e: any) {
    err.value = e?.message || '部署失败'
  }
}
function renderOps({ record }: any) {
  return h(
    'a-button',
    {
      type: 'link',
      size: 'small',
      onClick: () => {
        current.value = record
        openInvoke.value = true
      }
    },
    () => [h(PlayCircleOutlined), ' 调用']
  )
}
async function invoke() {
  try {
    await api.faas.invoke(current.value.name, JSON.parse(payload.value || '{}'))
    openInvoke.value = false
    payload.value = ''
  } catch (e: any) {
    err.value = e?.message || '调用失败'
  }
}
onMounted(load)
</script>
<style scoped>
.sb-file-hint {
  color: var(--sb-success);
  font-size: 13px;
}
</style>
