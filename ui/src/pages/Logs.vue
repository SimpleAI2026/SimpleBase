<template>
  <ProjectScope>
  <PageContainer title="日志管理" subtitle="按项目查询运行日志与保留策略">
    <a-card class="sb-card">
      <div class="sb-toolbar">
        <a-select
          v-model:value="level"
          allow-clear
          placeholder="级别"
          style="width: 120px"
          :options="levelOptions"
        />
        <a-input
          v-model:value="keyword"
          style="min-width: 220px"
          placeholder="关键字"
          allow-clear
        >
          <template #prefix><FilterOutlined /></template>
        </a-input>
        <a-input v-model:value="from" type="datetime-local" style="width: 210px" />
        <span class="sb-hint">至</span>
        <a-input v-model:value="to" type="datetime-local" style="width: 210px" />
        <a-button type="primary" :loading="loading" @click="load">
          <template #icon><ReloadOutlined /></template>
          刷新
        </a-button>
        <a-switch v-model:checked="autoRefresh" checked-children="轮询" un-checked-children="手动" />
        <span class="sb-count">{{ events.length }} 条</span>
      </div>
    </a-card>

    <a-card class="sb-card" title="保留策略">
      <a-space>
        <span>保留天数</span>
        <a-input-number v-model:value="keepDays" :min="1" :max="365" />
        <a-button :loading="savingRetention" @click="saveRetention">保存</a-button>
        <span v-if="retentionUpdatedAt" class="sb-hint">更新于 {{ retentionUpdatedAt }}</span>
      </a-space>
    </a-card>

    <a-card class="sb-card" title="日志">
      <a-table
        :columns="columns"
        :data-source="events"
        :loading="loading"
        row-key="id"
        :pagination="pagination"
        size="small"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'level'">
            <a-tag :color="levelColor(record.level)">{{ record.level || '-' }}</a-tag>
          </template>
          <template v-else-if="column.key === 'occurredAt'">
            {{ formatTime(record.occurredAt) }}
          </template>
        </template>
        <template #emptyText>
          <SbEmptyState description="暂无匹配日志" />
        </template>
      </a-table>
    </a-card>
  </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { FilterOutlined, ReloadOutlined } from '@ant-design/icons-vue'
import { api } from '../services/api'
import type { LogEvent } from '../services/api'
import { useProjectStore } from '../stores/project'
import { usePagination } from '../composables/usePagination'
import { formatTime } from '../utils/format'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import SbEmptyState from '../components/SbEmptyState.vue'

const projectStore = useProjectStore()
const pagination = usePagination()

const level = ref<string | undefined>()
const keyword = ref('')
const from = ref('')
const to = ref('')
const events = ref<LogEvent[]>([])
const loading = ref(false)
const autoRefresh = ref(false)
const keepDays = ref(14)
const savingRetention = ref(false)
const retentionUpdatedAt = ref('')
let timer: number | null = null

const levelOptions = [
  { label: 'info', value: 'info' },
  { label: 'warn', value: 'warn' },
  { label: 'error', value: 'error' }
]

const columns = [
  { title: '时间', key: 'occurredAt', width: 180 },
  { title: '级别', key: 'level', width: 90 },
  { title: '来源', dataIndex: 'logger', key: 'logger', width: 140 },
  { title: '消息', dataIndex: 'message', key: 'message', ellipsis: true },
  { title: 'Request ID', dataIndex: 'requestId', key: 'requestId', ellipsis: true, width: 200 }
]

function levelColor(s: string) {
  if (s === 'error') return 'error'
  if (s === 'warn') return 'warning'
  return 'default'
}

function queryParams() {
  return {
    level: level.value || undefined,
    q: keyword.value.trim() || undefined,
    from: from.value ? new Date(from.value).toISOString() : undefined,
    to: to.value ? new Date(to.value).toISOString() : undefined,
    limit: 200
  }
}

async function load() {
  if (!projectStore.id) return
  loading.value = true
  try {
    const [list, retention] = await Promise.all([
      api.logs.list(projectStore.id, queryParams()),
      api.logs.getRetention(projectStore.id)
    ])
    events.value = list
    keepDays.value = retention.keepDays
    retentionUpdatedAt.value = retention.updatedAt ? formatTime(retention.updatedAt) : ''
  } catch (e) {
    message.error((e as Error)?.message || '加载日志失败')
  } finally {
    loading.value = false
  }
}

async function saveRetention() {
  if (!projectStore.id) return
  savingRetention.value = true
  try {
    await api.logs.putRetention(projectStore.id, keepDays.value)
    message.success('已保存保留策略')
    await load()
  } catch (e) {
    message.error((e as Error)?.message || '保存失败')
  } finally {
    savingRetention.value = false
  }
}

function stopPolling() {
  if (timer != null) {
    window.clearInterval(timer)
    timer = null
  }
}

watch(autoRefresh, (on) => {
  stopPolling()
  if (on) {
    timer = window.setInterval(() => {
      void load()
    }, 10000)
  }
})

watch(
  () => projectStore.id,
  () => {
    void load()
  }
)

onMounted(load)
onBeforeUnmount(stopPolling)
</script>

<style scoped>
.sb-count {
  margin-left: auto;
  color: var(--sb-text-muted);
  font-size: var(--sb-fs-xs);
}
.sb-hint {
  color: var(--sb-text-muted);
  font-size: var(--sb-fs-xs);
}
@media (max-width: 768px) {
  .sb-count {
    margin-left: 0;
    flex: 1 1 100%;
  }
}
</style>
