<template>
  <PageContainer title="监控大盘" subtitle="实时查看系统运行状态">
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
          近 7 天请求趋势
          <a-tag v-if="isMock" color="orange" class="sb-mock-tag">Mock</a-tag>
        </div>
      </template>
      <template #extra>
        <div class="sb-toolbar">
          <span v-if="lastUpdate" class="sb-hint">最近更新：{{ lastUpdate }}</span>
          <a-button type="primary" @click="load">
            <template #icon><ReloadOutlined /></template>
            刷新数据
          </a-button>
        </div>
      </template>

      <a-alert v-if="err" type="error" :message="err" show-icon style="margin-bottom: 12px" />

      <div class="sb-chart">
        <div v-for="p in trend" :key="p.date" class="sb-chart-col">
          <a-tooltip :title="`${p.date} · 请求 ${p.requests} · 错误 ${p.errors}`">
            <div class="sb-chart-bars">
              <div
                class="sb-chart-bar sb-chart-bar--req"
                :style="{ height: barHeight(p.requests) }"
              />
              <div
                class="sb-chart-bar sb-chart-bar--err"
                :style="{ height: barHeight(p.errors) }"
              />
            </div>
          </a-tooltip>
          <div class="sb-chart-date">{{ p.date }}</div>
        </div>
        <div v-if="!trend.length && !loading" class="sb-chart-empty">暂无数据</div>
      </div>
      <div class="sb-chart-legend">
        <span class="sb-dot sb-dot--req" />请求数
        <span class="sb-dot sb-dot--err" />错误数
      </div>
    </a-card>
  </PageContainer>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  BarChartOutlined,
  WarningOutlined,
  ClockCircleOutlined,
  ThunderboltOutlined,
  ReloadOutlined
} from '@ant-design/icons-vue'
import { api, isMock } from '../services/api'
import type { MetricsSummary, TrendPoint } from '../services/api'
import PageContainer from '../components/PageContainer.vue'

const summary = ref<MetricsSummary>({
  totalRequests: 0,
  errorRate: 0,
  avgLatencyMs: 0,
  activeFunctions: 0
})
const trend = ref<TrendPoint[]>([])
const loading = ref(false)
const err = ref('')
const lastUpdate = ref('')

const cards = computed(() => [
  {
    label: '请求总数',
    value: summary.value.totalRequests.toLocaleString(),
    unit: '',
    icon: BarChartOutlined,
    bg: 'rgba(217, 119, 87, 0.12)',
    color: '#d97757'
  },
  {
    label: '错误率',
    value: summary.value.errorRate,
    unit: '%',
    icon: WarningOutlined,
    bg: 'rgba(192, 69, 47, 0.1)',
    color: '#c0452f'
  },
  {
    label: '平均耗时',
    value: summary.value.avgLatencyMs,
    unit: 'ms',
    icon: ClockCircleOutlined,
    bg: 'rgba(201, 154, 44, 0.12)',
    color: '#c99a2c'
  },
  {
    label: '活跃函数',
    value: summary.value.activeFunctions,
    unit: '',
    icon: ThunderboltOutlined,
    bg: 'rgba(63, 138, 90, 0.12)',
    color: '#3f8a5a'
  }
])

const maxRequests = computed(() => Math.max(1, ...trend.value.map((p) => p.requests)))
function barHeight(v: number) {
  return `${Math.max(3, Math.round((v / maxRequests.value) * 140))}px`
}

async function load() {
  loading.value = true
  err.value = ''
  // 分别请求：trend 接口在后端未就绪时不影响统计卡渲染
  const [summaryRes, trendRes] = await Promise.allSettled([
    api.metrics.summary(),
    api.metrics.trend()
  ])
  if (summaryRes.status === 'fulfilled') {
    summary.value = { ...summary.value, ...summaryRes.value }
  } else {
    err.value = (summaryRes.reason as Error)?.message || '加载失败'
  }
  if (trendRes.status === 'fulfilled') {
    trend.value = trendRes.value
  }
  if (summaryRes.status === 'fulfilled' || trendRes.status === 'fulfilled') {
    lastUpdate.value = new Date().toLocaleTimeString('zh-CN', { hour12: false })
  }
  loading.value = false
}
onMounted(load)
</script>

<style scoped>
.sb-stat-card {
  height: 100%;
}
.sb-stat-card :deep(.ant-card-body) {
  padding: 18px 20px;
}
.sb-unit {
  font-size: 14px;
  font-weight: 500;
  color: var(--sb-text-secondary);
  margin-left: 4px;
}
.sb-hint {
  color: var(--sb-text-muted);
  font-size: 12px;
}
.sb-card-title {
  display: flex;
  align-items: center;
  gap: 8px;
}
.sb-mock-tag {
  margin-inline-end: 0;
}
.sb-chart {
  display: flex;
  align-items: flex-end;
  gap: 18px;
  padding: 8px 4px 0;
  min-height: 170px;
  overflow-x: auto;
}
.sb-chart-col {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  flex: 1;
  min-width: 40px;
}
.sb-chart-bars {
  display: flex;
  align-items: flex-end;
  gap: 4px;
  height: 140px;
}
.sb-chart-bar {
  width: 14px;
  border-radius: 4px 4px 0 0;
  transition: height 0.4s ease;
}
.sb-chart-bar--req {
  background: var(--sb-primary);
}
.sb-chart-bar--err {
  background: var(--sb-danger);
}
.sb-chart-date {
  font-size: 12px;
  color: var(--sb-text-secondary);
}
.sb-chart-empty {
  width: 100%;
  text-align: center;
  color: var(--sb-text-muted);
  padding: 60px 0;
}
.sb-chart-legend {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
  font-size: 12px;
  color: var(--sb-text-secondary);
}
.sb-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
  margin-left: 12px;
}
.sb-dot--req {
  background: var(--sb-primary);
}
.sb-dot--err {
  background: var(--sb-danger);
}
</style>
