<template>
  <PageContainer title="日志管理" subtitle="实时日志流推送与过滤">
    <a-card class="sb-card">
      <div class="sb-toolbar">
        <a-input
          v-model:value="filter"
          style="min-width:240px"
          placeholder="过滤关键字"
          allow-clear
        >
          <template #prefix><FilterOutlined /></template>
        </a-input>
        <a-button type="primary" @click="connect" :disabled="connected">
          <template #icon><LinkOutlined /></template>
          连接
        </a-button>
        <a-button danger @click="disconnect" :disabled="!connected">
          <template #icon><DisconnectOutlined /></template>
          断开
        </a-button>
        <a-tag :color="connected ? 'success' : 'default'">
          {{ connected ? '已连接' : '未连接' }}
        </a-tag>
        <a-button type="text" @click="logs = []" :disabled="!logs.length">
          <template #icon><ClearOutlined /></template>
          清空
        </a-button>
      </div>
    </a-card>

    <a-card class="sb-card" title="实时日志">
      <div class="sb-terminal" ref="terminalRef">
        <div v-for="(l, i) in filtered" :key="i" class="sb-log-line">{{ l }}</div>
        <div v-if="!filtered.length" class="sb-empty">暂无日志，点击"连接"开始接收</div>
      </div>
    </a-card>
  </PageContainer>
</template>
<script setup lang="ts">
import { ref, computed, nextTick, watch } from 'vue'
import {
  FilterOutlined,
  LinkOutlined,
  DisconnectOutlined,
  ClearOutlined
} from '@ant-design/icons-vue'
import { api } from '../services/api'
import PageContainer from '../components/PageContainer.vue'

const filter = ref('')
const logs = ref<string[]>([])
const ws = ref<WebSocket | null>(null)
const connected = ref(false)
const terminalRef = ref<HTMLElement | null>(null)

const filtered = computed(() =>
  logs.value.filter((l) => !filter.value || l.includes(filter.value))
)

watch(
  () => filtered.value.length,
  async () => {
    await nextTick()
    if (terminalRef.value) {
      terminalRef.value.scrollTop = terminalRef.value.scrollHeight
    }
  }
)

function connect() {
  if (ws.value) return
  ws.value = new WebSocket(api.logs.streamUrl())
  ws.value.onopen = () => {
    connected.value = true
  }
  ws.value.onmessage = (ev) => {
    logs.value.push(ev.data)
  }
  ws.value.onclose = () => {
    connected.value = false
    ws.value = null
  }
}
function disconnect() {
  ws.value?.close()
  ws.value = null
}
</script>
<style scoped>
.sb-empty {
  color: #4b5563;
  text-align: center;
  padding: 40px 0;
}
</style>
