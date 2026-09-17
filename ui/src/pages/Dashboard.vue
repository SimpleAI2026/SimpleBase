<template>
  <ProjectScope>
  <PageContainer title="监控大盘" subtitle="系统运行状态与数据库概览">
    <a-row :gutter="16">
      <a-col v-for="card in cards" :key="card.label" :xs="24" :sm="12" :lg="6">
        <a-card class="sb-card sb-stat-card" :loading="loading">
          <div class="sb-stat">
            <div class="sb-stat-icon" :style="{ background: card.bg, color: card.color }">
              <component :is="card.icon" />
            </div>
            <div class="sb-stat-body">
              <div class="sb-stat-label">{{ card.label }}</div>
              <div class="sb-stat-value">
                {{ card.value }}<span v-if="card.unit" class="sb-unit">{{ card.unit }}</span>
              </div>
            </div>
          </div>
        </a-card>
      </a-col>
    </a-row>

    <a-card class="sb-card">
      <template #title>
        <div class="sb-card-title">
          请求趋势
          <a-tag v-if="isMock" color="orange" class="sb-mock-tag">Mock</a-tag>
        </div>
      </template>
      <template #extra>
        <div class="sb-toolbar">
          <span v-if="summary.totalRequests" class="sb-hint">
            请求 {{ summary.totalRequests }} · 错误率 {{ summary.errorRate }}% · 延迟
            {{ Math.round(summary.avgLatencyMs) }}ms
          </span>
          <span v-if="lastUpdate" class="sb-hint">最近更新：{{ lastUpdate }}</span>
          <a-button type="primary" :loading="loading" @click="load">
            <template #icon><ReloadOutlined /></template>
            刷新数据
          </a-button>
        </div>
      </template>

      <div v-if="trend.length" class="sb-trend">
        <div v-for="p in trend" :key="p.date" class="sb-trend-col">
          <div class="sb-trend-bars">
            <div
              class="sb-trend-bar sb-trend-bar--req"
              :style="{ height: barHeight(p.requests, maxRequests) }"
              :title="`请求 ${p.requests}`"
            />
            <div
              class="sb-trend-bar sb-trend-bar--err"
              :style="{ height: barHeight(p.errors, maxRequests) }"
              :title="`错误 ${p.errors}`"
            />
          </div>
          <div class="sb-trend-label">{{ p.date }}</div>
        </div>
      </div>
      <a-empty v-else-if="!loading" description="暂无趋势数据" style="margin-bottom: 12px" />

      <a-table
        :columns="dbColumns"
        :data-source="databases"
        :loading="loading"
        row-key="id"
        :pagination="pagination"
        size="small"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status'">
            <a-tag :color="statusColor(record.status)">{{ record.status }}</a-tag>
          </template>
          <template v-else-if="column.key === 'createdAt'">
            {{ formatTime(record.createdAt) }}
          </template>
        </template>
        <template #emptyText>
          <SbEmptyState description="暂无数据库" action-text="去创建" @action="goDatabases" />
        </template>
      </a-table>
    </a-card>
  </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import {
  DatabaseOutlined,
  CheckCircleOutlined,
  ExclamationCircleOutlined,
  ReloadOutlined
} from '@ant-design/icons-vue'
import { api, isMock } from '../services/api'
import type { DatabaseItem, QuotaStatus, TrendPoint } from '../services/api'
import { useProjectStore } from '../stores/project'
import { usePagination } from '../composables/usePagination'
import { softBg, colors } from '../styles/tokens'
import { formatTime } from '../utils/format'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import SbEmptyState from '../components/SbEmptyState.vue'

const router = useRouter()
const projectStore = useProjectStore()
const pagination = usePagination()

const databases = ref<DatabaseItem[]>([])
const quota = ref<QuotaStatus | null>(null)
const loading = ref(false)
const lastUpdate = ref('')
const err = ref('')
const trend = ref<TrendPoint[]>([])
const summary = ref({ totalRequests: 0, errorRate: 0, avgLatencyMs: 0, activeDatabases: 0 })

const maxRequests = computed(() => Math.max(1, ...trend.value.map((p) => p.requests)))

function barHeight(value: number, max: number) {
  return `${Math.max(4, Math.round((value / max) * 120))}px`
}

const dbColumns = [
  { title: '名称', dataIndex: 'name', key: 'name', width: 160 },
  { title: 'ID', dataIndex: 'id', key: 'id', ellipsis: true },
  { title: '状态', key: 'status', width: 110 },
  { title: '创建时间', key: 'createdAt', width: 180 }
]

/** 统计卡配色从 tokens.ts 派生（替代原 8 处硬编码 rgba） */
const cards = computed(() => [
  {
    label: '数据库总数',
    value: databases.value.length,
    unit: '',
    icon: DatabaseOutlined,
    bg: softBg(colors.primary),
    color: colors.primary
  },
  {
    label: '就绪数据库',
    value: databases.value.filter((d) => d.status === 'ready').length,
    unit: '',
    icon: CheckCircleOutlined,
    bg: softBg(colors.success),
    color: colors.success
  },
  {
    label: '异常数据库',
    value: databases.value.filter((d) => ['degraded', 'deleting'].includes(d.status)).length,
    unit: '',
    icon: ExclamationCircleOutlined,
    bg: softBg(colors.danger),
    color: colors.danger
  },
  {
    label: '配额状态',
    value: quota.value ? (quota.value.llmAllowed && quota.value.databaseAllowed ? '正常' : '受限') : '-',
    unit: '',
    icon: ExclamationCircleOutlined,
    bg: quota.value?.llmAllowed === false ? softBg(colors.warning) : softBg(colors.info),
    color: quota.value?.llmAllowed === false ? colors.warning : colors.info
  }
])

const statusColorMap: Record<string, string> = {
  ready: 'success',
  creating: 'processing',
  opening: 'processing',
  closing: 'processing',
  recovering: 'processing',
  closed: 'default',
  degraded: 'warning',
  deleting: 'warning',
  deleted: 'default'
}
function statusColor(s: string) {
  return statusColorMap[s] || 'default'
}

async function load() {
  loading.value = true
  err.value = ''
  // 分别请求：databases 失败不影响 quota 卡渲染
  const [dbRes, quotaRes, trendRes, summaryRes] = await Promise.allSettled([
    api.databases.list(projectStore.id),
    api.quota.status(projectStore.id),
    api.metrics.trend(projectStore.id),
    api.metrics.summary(projectStore.id)
  ])
  if (dbRes.status === 'fulfilled') {
    databases.value = dbRes.value
  } else {
    err.value = (dbRes.reason as Error)?.message || '加载失败'
  }
  if (quotaRes.status === 'fulfilled') {
    quota.value = quotaRes.value
  }
  if (trendRes.status === 'fulfilled') {
    trend.value = trendRes.value
  }
  if (summaryRes.status === 'fulfilled') {
    summary.value = summaryRes.value
  }
  if (
    dbRes.status === 'fulfilled' ||
    quotaRes.status === 'fulfilled' ||
    trendRes.status === 'fulfilled'
  ) {
    lastUpdate.value = new Date().toLocaleTimeString('zh-CN', { hour12: false })
  }
  loading.value = false
}

function goDatabases() {
  router.push({ name: 'databases' })
}

onMounted(load)
watch(
  () => projectStore.id,
  () => {
    void load()
  }
)
</script>

<style scoped>
.sb-stat-card {
  height: 100%;
}
.sb-stat-card :deep(.ant-card-body) {
  padding: 18px 20px;
}
.sb-unit {
  font-size: var(--sb-fs-sm);
  font-weight: 500;
  color: var(--sb-text-secondary);
  margin-left: 4px;
}
.sb-hint {
  color: var(--sb-text-muted);
  font-size: var(--sb-fs-xs);
}
.sb-card-title {
  display: flex;
  align-items: center;
  gap: var(--sb-space-2);
}
.sb-trend {
  display: flex;
  align-items: flex-end;
  gap: 12px;
  min-height: 160px;
  margin-bottom: 16px;
  padding: 8px 4px 0;
}
.sb-trend-col {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
}
.sb-trend-bars {
  display: flex;
  align-items: flex-end;
  gap: 4px;
  height: 120px;
}
.sb-trend-bar {
  width: 10px;
  border-radius: 4px 4px 0 0;
}
.sb-trend-bar--req {
  background: var(--sb-primary);
}
.sb-trend-bar--err {
  background: var(--sb-danger);
}
.sb-trend-label {
  color: var(--sb-text-muted);
  font-size: var(--sb-fs-xs);
}
</style>
