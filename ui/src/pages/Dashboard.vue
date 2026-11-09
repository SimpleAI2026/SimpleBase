<template>
  <ProjectScope>
  <PageContainer :subtitle="t('pages.dashboard.subtitle')">
    <SbBlockSkeleton
      v-if="pending && !hasData"
      variant="cards"
      :count="4"
      :class="['gap-4 sm:grid-cols-2 lg:grid-cols-4', !showSkeleton && 'invisible']"
    />
    <div v-else-if="!showError" class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <Card v-for="card in cards" :key="card.label">
        <CardContent class="p-5">
          <div class="flex items-start gap-4">
            <div
              class="flex size-12 shrink-0 items-center justify-center rounded-lg"
              :class="card.tone"
            >
              <component :is="card.icon" class="size-5" />
            </div>
            <div class="min-w-0 flex-1">
              <div class="text-xs tracking-wide text-muted-foreground uppercase font-medium">{{ card.label }}</div>
              <div class="mt-1 text-2xl leading-tight font-semibold text-foreground">{{ card.value }}</div>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>

    <Card>
      <CardHeader class="border-b">
        <CardTitle class="flex items-center gap-2">
          {{ t('pages.dashboard.requestTrend') }}
        </CardTitle>
        <CardDescription v-if="summary.totalRequests || lastUpdate" class="flex flex-wrap gap-3">
          <span v-if="summary.totalRequests">
            {{ t('pages.dashboard.summaryLine', {
              requests: summary.totalRequests,
              rate: formatErrorRate(summary.errorRate),
              latency: Math.round(summary.avgLatencyMs)
            }) }}
          </span>
          <span v-if="lastUpdate">{{ t('pages.dashboard.lastUpdate', { time: lastUpdate }) }}</span>
        </CardDescription>
        <CardAction>
          <Button variant="outline" size="sm" :disabled="pending" @click="load">
            <Spinner v-if="pending" data-icon="inline-start" />
            <RefreshCwIcon v-else data-icon="inline-start" />
            {{ t('pages.dashboard.refreshData') }}
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent class="flex flex-col gap-6 pt-6">
        <!-- 接口耗时分位数：全项目所有接口，近 24 小时，固定桶估算 -->
        <div data-testid="latency-percentiles" class="flex flex-col gap-2">
          <div class="flex flex-wrap items-baseline justify-between gap-2">
            <span class="text-sm font-medium text-foreground">{{ t('pages.dashboard.latencyTitle') }}</span>
            <span class="text-xs text-muted-foreground">
              {{ t('pages.dashboard.latencyDesc') }}<template v-if="hasLatencySamples">{{ t('pages.dashboard.latencySamples', { n: summary.latencySampleCount }) }}</template>
            </span>
          </div>
          <div v-if="pending && !hasData" class="grid grid-cols-1 gap-3 sm:grid-cols-3" :class="showSkeleton ? '' : 'invisible'" aria-hidden="true">
            <Skeleton v-for="n in 3" :key="'lp-' + n" class="h-16 w-full rounded-lg" />
          </div>
          <div v-else-if="hasLatencySamples" class="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <div
              v-for="item in latencyItems"
              :key="item.label"
              class="rounded-lg border border-border px-4 py-3"
            >
              <div class="text-xs font-medium text-muted-foreground">{{ item.label }}</div>
              <div class="mt-1 text-xl font-semibold tabular-nums text-foreground">{{ item.value }}</div>
            </div>
          </div>
          <div v-else-if="!showError" class="rounded-lg border border-dashed border-border px-4 py-3 text-sm text-muted-foreground">
            {{ t('pages.dashboard.latencyNoSamples') }}
          </div>
        </div>

        <TrendChart
          v-if="trend.length"
          :points="trend"
          :mode="chartMode"
          @update:mode="chartMode = $event"
        />
        <SbBlockSkeleton
          v-else-if="pending && !hasData"
          variant="chart"
          :class="!showSkeleton && 'invisible'"
        />
        <SbEmptyState
          v-else-if="!showError"
          :title="trendFailed ? t('pages.dashboard.trendFailedTitle') : t('pages.dashboard.trendEmptyTitle')"
          :description="trendFailed ? t('pages.dashboard.trendFailedDesc') : t('pages.dashboard.trendEmptyDesc')"
        />

        <!-- 资源汇总：只展示各类型数量，不列明细名称 -->
        <div class="overflow-hidden rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead class="sb-col-name">{{ t('pages.dashboard.resourceType') }}</TableHead>
                <TableHead class="w-28">{{ t('pages.dashboard.quantity') }}</TableHead>
                <TableHead class="max-w-md">{{ t('pages.dashboard.resourceHint') }}</TableHead>
              </TableRow>
            </TableHeader>
            <SbAsyncRegion
              as="tbody"
              :columns="3"
              :rows="5"
              :pending="pending"
              :show-skeleton="showSkeleton"
              :show-empty="false"
              :show-error="showError"
              :refreshing="refreshing"
              :error="loadError"
              @retry="load"
            >
              <TableRow v-for="row in resourceRows" :key="row.key">
                <TableCell class="font-medium">
                  <span class="inline-flex items-center gap-2">
                    <component :is="row.icon" class="size-4 shrink-0 text-primary opacity-80" />
                    {{ row.label }}
                  </span>
                </TableCell>
                <TableCell class="w-28 text-base font-semibold tabular-nums">
                  {{ row.count }}
                </TableCell>
                <TableCell class="max-w-md text-xs text-muted-foreground">{{ row.hint }}</TableCell>
              </TableRow>
            </SbAsyncRegion>
          </Table>
        </div>
      </CardContent>
    </Card>
  </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import {
  BotIcon,
  CircleCheckIcon,
  CloudUploadIcon,
  CodeIcon,
  DatabaseIcon,
  RefreshCwIcon,
  TimerIcon,
  TriangleAlertIcon
} from '@lucide/vue'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { api } from '../services/api'
import type { MetricsSummary, QuotaStatus, TrendPoint } from '../services/api'
import { useProjectStore } from '../stores/project'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import SbBlockSkeleton from '@/components/SbBlockSkeleton.vue'
import SbEmptyState from '../components/SbEmptyState.vue'
import { useLoadState } from '@/composables/useLoadState'
import { currentBcp47, formatErrorRate } from '@/utils/format'
import TrendChart, { type ChartMode } from '../components/TrendChart.vue'

const projectStore = useProjectStore()
const { t } = useI18n()

const quota = ref<QuotaStatus | null>(null)
const trendFailed = ref(false)
const lastUpdate = ref('')
const trend = ref<TrendPoint[]>([])
const chartMode = ref<ChartMode>('line')
const summary = ref<MetricsSummary>({ totalRequests: 0, errorRate: 0, avgLatencyMs: 0, activeDatabases: 0 })

const hasLatencySamples = computed(
  () => (summary.value.latencySampleCount ?? 0) > 0 && summary.value.latencyP50Ms != null
)

/** 格式化分位数：null → —；>= 超量程阈值 → ≥30s；>= 1s 用秒，否则整数毫秒 */
function formatLatency(v: number | null | undefined): string {
  if (v == null || !Number.isFinite(v)) return '—'
  const overflow = summary.value.latencyOverflowMs ?? 30000
  if (v >= overflow) return `≥${Math.round(overflow / 1000)}s`
  if (v >= 1000) return `${(v / 1000).toFixed(2)}s`
  return `${Math.round(v)}ms`
}

const latencyItems = computed(() => [
  { label: 'P50', value: formatLatency(summary.value.latencyP50Ms) },
  { label: 'P90', value: formatLatency(summary.value.latencyP90Ms) },
  { label: 'P99', value: formatLatency(summary.value.latencyP99Ms) },
])

/** 资源计数（按类型汇总，不列明细） */
const counts = ref({
  databases: 0,
  s3Objects: 0,
  gofunctions: 0,
  cronjobs: 0,
  agents: 0
})
const dbStatus = ref({ ready: 0, abnormal: 0 })
const resourcesLoaded = ref(false)

const cards = computed(() => [
  {
    label: t('pages.dashboard.statDbTotal'),
    value: resourcesLoaded.value ? counts.value.databases : '-',
    icon: DatabaseIcon,
    tone: 'bg-primary/12 text-primary',
  },
  {
    label: t('pages.dashboard.statDbReady'),
    value: resourcesLoaded.value ? dbStatus.value.ready : '-',
    icon: CircleCheckIcon,
    tone: 'bg-success/12 text-success',
  },
  {
    label: t('pages.dashboard.statDbAbnormal'),
    value: resourcesLoaded.value ? dbStatus.value.abnormal : '-',
    icon: TriangleAlertIcon,
    tone: 'bg-destructive/10 text-destructive',
  },
  {
    label: t('pages.dashboard.statQuota'),
    value: quota.value
      ? quota.value.llmAllowed && quota.value.databaseAllowed
        ? t('pages.dashboard.quotaNormal')
        : t('pages.dashboard.quotaLimited')
      : '-',
    icon: quota.value && !(quota.value.llmAllowed && quota.value.databaseAllowed) ? TriangleAlertIcon : CircleCheckIcon,
    tone:
      quota.value?.llmAllowed === false ? 'bg-warning/12 text-warning' : 'bg-info/12 text-info',
  },
])

const resourceRows = computed(() => [
  {
    key: 'databases',
    label: t('pages.dashboard.resDatabase'),
    icon: DatabaseIcon,
    count: counts.value.databases,
    hint: t('pages.dashboard.resDatabaseHint'),
  },
  {
    key: 's3',
    label: t('pages.dashboard.resStorage'),
    icon: CloudUploadIcon,
    count: counts.value.s3Objects,
    hint: t('pages.dashboard.resStorageHint'),
  },
  {
    key: 'gofunctions',
    label: t('pages.dashboard.resFunctions'),
    icon: CodeIcon,
    count: counts.value.gofunctions,
    hint: t('pages.dashboard.resFunctionsHint'),
  },
  {
    key: 'cronjobs',
    label: t('pages.dashboard.resCron'),
    icon: TimerIcon,
    count: counts.value.cronjobs,
    hint: t('pages.dashboard.resCronHint'),
  },
  {
    key: 'agents',
    label: t('pages.dashboard.resAgents'),
    icon: BotIcon,
    count: counts.value.agents,
    hint: t('pages.dashboard.resAgentsHint'),
  },
])

const {
  pending,
  hasData,
  showSkeleton,
  showError,
  refreshing,
  error: loadError,
  run
} = useLoadState({ fallback: 'pages.dashboard.loadFailed' })

function load() {
  return run(async () => {
    const [dbRes, s3Res, fnRes, cronRes, agentRes, quotaRes, trendRes, summaryRes] =
      await Promise.allSettled([
        api.databases.list(projectStore.id),
        api.s3.list(projectStore.id),
        api.gofunctions.list(projectStore.id),
        api.cronjobs.list(projectStore.id),
        api.agents.list(projectStore.id),
        api.quota.status(projectStore.id),
        api.metrics.trend(projectStore.id),
        api.metrics.summary(projectStore.id),
      ])

    if (dbRes.status === 'fulfilled') {
      const dbs = Array.isArray(dbRes.value) ? dbRes.value : []
      counts.value.databases = dbs.length
      dbStatus.value = {
        ready: dbs.filter((d) => d.status === 'ready').length,
        abnormal: dbs.filter((d) => ['degraded', 'deleting'].includes(d.status)).length,
      }
    }
    if (s3Res.status === 'fulfilled') counts.value.s3Objects = Array.isArray(s3Res.value) ? s3Res.value.length : 0
    if (fnRes.status === 'fulfilled') counts.value.gofunctions = Array.isArray(fnRes.value) ? fnRes.value.length : 0
    if (cronRes.status === 'fulfilled') counts.value.cronjobs = Array.isArray(cronRes.value) ? cronRes.value.length : 0
    if (agentRes.status === 'fulfilled') counts.value.agents = Array.isArray(agentRes.value) ? agentRes.value.length : 0
    resourcesLoaded.value =
      dbRes.status === 'fulfilled' ||
      s3Res.status === 'fulfilled' ||
      fnRes.status === 'fulfilled' ||
      cronRes.status === 'fulfilled' ||
      agentRes.status === 'fulfilled'

    if (quotaRes.status === 'fulfilled') quota.value = quotaRes.value
    if (trendRes.status === 'fulfilled') {
      trend.value = trendRes.value
      trendFailed.value = false
    } else {
      trendFailed.value = true
    }
    if (summaryRes.status === 'fulfilled') summary.value = summaryRes.value

    const all = [dbRes, s3Res, fnRes, cronRes, agentRes, quotaRes, trendRes, summaryRes]
    const rejected = all.filter((r) => r.status === 'rejected').length
    if (rejected === all.length) {
      throw new Error(t('pages.dashboard.loadFailed'))
    }
    if (rejected > 0) toast.error(t('pages.dashboard.loadFailed'))
    if (all.slice(0, 6).some((r) => r.status === 'fulfilled')) {
      lastUpdate.value = new Date().toLocaleTimeString(currentBcp47(), { hour12: false })
    }
    return true
  })
}

void load()
watch(
  () => projectStore.id,
  () => {
    void load()
  }
)
</script>
