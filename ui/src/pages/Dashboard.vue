<template>
  <PageContainer title="监控大盘" subtitle="实时查看系统运行状态">
    <a-row :gutter="16">
      <a-col :xs="24" :sm="12" :lg="6">
        <a-card class="sb-card sb-stat-card">
          <div class="sb-stat">
            <div class="sb-stat-icon" style="background: linear-gradient(135deg,#4f46e5,#7c83ff)">
              <BarChartOutlined />
            </div>
            <div class="sb-stat-body">
              <div class="sb-stat-label">请求总数</div>
              <div class="sb-stat-value">{{ summary.totalRequests }}</div>
            </div>
          </div>
        </a-card>
      </a-col>
      <a-col :xs="24" :sm="12" :lg="6">
        <a-card class="sb-card sb-stat-card">
          <div class="sb-stat">
            <div class="sb-stat-icon" style="background: linear-gradient(135deg,#ef4444,#f87171)">
              <WarningOutlined />
            </div>
            <div class="sb-stat-body">
              <div class="sb-stat-label">错误率</div>
              <div class="sb-stat-value">{{ summary.errorRate }}<span class="sb-unit">%</span></div>
            </div>
          </div>
        </a-card>
      </a-col>
      <a-col :xs="24" :sm="12" :lg="6">
        <a-card class="sb-card sb-stat-card">
          <div class="sb-stat">
            <div class="sb-stat-icon" style="background: linear-gradient(135deg,#06b6d4,#22d3ee)">
              <ClockCircleOutlined />
            </div>
            <div class="sb-stat-body">
              <div class="sb-stat-label">平均耗时</div>
              <div class="sb-stat-value">{{ summary.avgLatencyMs }}<span class="sb-unit">ms</span></div>
            </div>
          </div>
        </a-card>
      </a-col>
      <a-col :xs="24" :sm="12" :lg="6">
        <a-card class="sb-card sb-stat-card">
          <div class="sb-stat">
            <div class="sb-stat-icon" style="background: linear-gradient(135deg,#10b981,#34d399)">
              <ThunderboltOutlined />
            </div>
            <div class="sb-stat-body">
              <div class="sb-stat-label">活跃函数</div>
              <div class="sb-stat-value">{{ summary.activeFunctions }}</div>
            </div>
          </div>
        </a-card>
      </a-col>
    </a-row>

    <a-card class="sb-card" title="运行概况">
      <a-alert v-if="err" type="error" :message="err" show-icon style="margin-bottom:12px" />
      <div class="sb-toolbar">
        <a-button type="primary" @click="load">
          <template #icon><ReloadOutlined /></template>
          刷新数据
        </a-button>
        <span class="sb-hint" v-if="lastUpdate">最近更新：{{ lastUpdate }}</span>
      </div>
    </a-card>
  </PageContainer>
</template>
<script setup lang="ts">
import { reactive, ref, onMounted } from 'vue'
import {
  BarChartOutlined,
  WarningOutlined,
  ClockCircleOutlined,
  ThunderboltOutlined,
  ReloadOutlined
} from '@ant-design/icons-vue'
import { api } from '../services/api'
import PageContainer from '../components/PageContainer.vue'

const summary = reactive({
  totalRequests: 0,
  errorRate: 0,
  avgLatencyMs: 0,
  activeFunctions: 0
})
const err = ref('')
const lastUpdate = ref('')

async function load() {
  err.value = ''
  try {
    const s = await api.metrics.summary()
    Object.assign(summary, s || {})
    lastUpdate.value = new Date().toLocaleTimeString('zh-CN')
  } catch (e: any) {
    err.value = e?.message || '加载失败'
  }
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
  font-size: 14px !important;
  font-weight: 500 !important;
  color: var(--sb-text-secondary) !important;
  margin-left: 4px;
}
.sb-hint {
  color: var(--sb-text-muted);
  font-size: 12px;
}
</style>
