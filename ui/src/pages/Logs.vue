<template>
  <PageContainer title="日志管理" subtitle="实时日志流推送与过滤">
    <a-card class="sb-card">
      <div class="sb-toolbar">
        <a-input
          v-model:value="filter"
          style="min-width: 240px"
          placeholder="过滤关键字"
          allow-clear
        >
          <template #prefix><FilterOutlined /></template>
        </a-input>
        <a-button type="primary" :disabled="connected" @click="connect">
          <template #icon><LinkOutlined /></template>
          连接
        </a-button>
        <a-button danger :disabled="!connected" @click="disconnect">
          <template #icon><DisconnectOutlined /></template>
          断开
        </a-button>
        <a-tag :color="connected ? 'success' : 'default'" class="sb-status-tag">
          <span class="sb-status-dot" :class="{ 'is-on': connected }" />
          {{ connected ? '已连接' : '未连接' }}
        </a-tag>
        <a-button type="text" :disabled="!logs.length" @click="logs = []">
          <template #icon><ClearOutlined /></template>
          清空
        </a-button>
        <span class="sb-count">{{ filtered.length }} / {{ logs.length }} 条</span>
      </div>
    </a-card>

    <a-card class="sb-card" title="实时日志">
      <div ref="terminalRef" class="sb-terminal">
        <div v-for="(l, i) in filtered" :key="i" class="sb-log-line" :class="levelClass(l)">
          {{ l }}
        </div>
        <div v-if="!filtered.length" class="sb-empty">暂无日志，点击"连接"开始接收</div>
      </div>
    </a-card>
  </PageContainer>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import {
  FilterOutlined,
  LinkOutlined,
  DisconnectOutlined,
  ClearOutlined
} from '@ant-design/icons-vue'
import { api } from '../services/api'
import type { LogConnection } from '../services/api'
import PageContainer from '../components/PageContainer.vue'

const filter = ref('')
const logs = ref<string[]>([])
const connected = ref(false)
const terminalRef = ref<HTMLElement | null>(null)
let conn: LogConnection | null = null

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

// 兼容多种后端日志格式：[ERROR] / " ERROR " / level=error
function levelClass(line: string) {
  if (/\[ERROR\]|\bERROR\b|level=error/i.test(line)) return 'sb-log-line--error'
  if (/\[WARN\]|\bWARN\b|level=warn/i.test(line)) return 'sb-log-line--warn'
  return ''
}

function connect() {
  if (conn) return
  conn = api.logs.connect({
    onOpen: () => {
      connected.value = true
    },
    onMessage: (line) => {
      logs.value.push(line)
      if (logs.value.length > 2000) logs.value.splice(0, logs.value.length - 2000)
    },
    onClose: () => {
      connected.value = false
      conn = null
    }
  })
}

function disconnect() {
  conn?.close()
  conn = null
  connected.value = false
}

onBeforeUnmount(disconnect)
</script>

<style scoped>
.sb-empty {
  color: var(--sb-text-muted);
  text-align: center;
  padding: 40px 0;
}
.sb-status-tag {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.sb-status-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #9ca3af;
}
.sb-status-dot.is-on {
  background: var(--sb-success);
  box-shadow: 0 0 6px rgba(48, 209, 88, 0.6);
  animation: sb-pulse 1.6s ease-in-out infinite;
}
.sb-count {
  margin-left: auto;
  color: var(--sb-text-muted);
  font-size: 12px;
}
@media (max-width: 768px) {
  .sb-count {
    margin-left: 0;
    flex: 1 1 100%;
  }
}
.sb-log-line--error {
  color: var(--sb-danger);
  background: rgba(192, 69, 47, 0.08);
}
.sb-log-line--warn {
  color: #9a7b1a;
}
@keyframes sb-pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.4;
  }
}
</style>
