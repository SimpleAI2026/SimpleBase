<template>
  <ProjectScope>
  <PageContainer subtitle="系统运行状态与项目资源概览">
    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <Card v-for="card in cards" :key="card.label">
        <CardContent class="p-5">
          <div v-if="loading" class="flex items-start gap-4">
            <Skeleton class="size-12 rounded-lg" />
            <div class="flex flex-1 flex-col gap-2.5">
              <Skeleton class="h-3 w-20" />
              <Skeleton class="h-7 w-16" />
            </div>
          </div>
          <div v-else class="flex items-start gap-4">
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
          请求趋势
          <Badge v-if="isMock" variant="warning">Mock</Badge>
        </CardTitle>
        <CardDescription v-if="summary.totalRequests || lastUpdate" class="flex flex-wrap gap-3">
          <span v-if="summary.totalRequests">
            请求 {{ summary.totalRequests }} · 错误率 {{ summary.errorRate }}% · 延迟
            {{ Math.round(summary.avgLatencyMs) }}ms
          </span>
          <span v-if="lastUpdate">最近更新：{{ lastUpdate }}</span>
        </CardDescription>
        <CardAction>
          <Button variant="outline" size="sm" :disabled="loading" @click="load">
            <Spinner v-if="loading" data-icon="inline-start" />
            <RefreshCwIcon v-else data-icon="inline-start" />
            刷新数据
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent class="flex flex-col gap-6 pt-6">
        <TrendChart
          v-if="trend.length"
          :points="trend"
          :mode="chartMode"
          @update:mode="chartMode = $event"
        />
        <Skeleton v-else-if="loading" class="h-44 w-full rounded-xl" />
        <SbEmptyState
          v-else
          :title="trendFailed ? '趋势加载失败' : '暂无数据'"
          :description="trendFailed ? '请求未完成，请刷新后重试' : '暂无趋势数据'"
        />

        <!-- 资源汇总：只展示各类型数量，不列明细名称 -->
        <div class="overflow-hidden rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead class="sb-col-name">资源类型</TableHead>
                <TableHead class="w-28 text-right">数量</TableHead>
                <TableHead class="max-w-md">说明</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <template v-if="loading && !resourcesLoaded">
                <TableRow v-for="n in 3" :key="'sk-' + n">
                  <TableCell colspan="3"><Skeleton class="h-8 w-full" /></TableCell>
                </TableRow>
              </template>
              <TableRow v-for="row in resourceRows" v-else :key="row.key">
                <TableCell class="font-medium">
                  <span class="inline-flex items-center gap-2">
                    <component :is="row.icon" class="size-4 shrink-0 text-primary opacity-80" />
                    {{ row.label }}
                  </span>
                </TableCell>
                <TableCell class="w-28 text-right text-base font-semibold tabular-nums">
                  {{ row.count }}
                </TableCell>
                <TableCell class="max-w-md text-xs text-muted-foreground">{{ row.hint }}</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>
      </CardContent>
    </Card>
  </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
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
import { Badge } from '@/components/ui/badge'
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
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { api, isMock } from '../services/api'
import type { QuotaStatus, TrendPoint } from '../services/api'
import { useProjectStore } from '../stores/project'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import SbEmptyState from '../components/SbEmptyState.vue'
import TrendChart, { type ChartMode } from '../components/TrendChart.vue'

const router = useRouter()
const projectStore = useProjectStore()

const quota = ref<QuotaStatus | null>(null)
const loading = ref(false)
const trendFailed = ref(false)
const lastUpdate = ref('')
const trend = ref<TrendPoint[]>([])
const chartMode = ref<ChartMode>('bar')
const summary = ref({ totalRequests: 0, errorRate: 0, avgLatencyMs: 0, activeDatabases: 0 })

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
    label: '数据库总数',
    value: resourcesLoaded.value ? counts.value.databases : '-',
    icon: DatabaseIcon,
    tone: 'bg-primary/12 text-primary',
  },
  {
    label: '就绪数据库',
    value: resourcesLoaded.value ? dbStatus.value.ready : '-',
    icon: CircleCheckIcon,
    tone: 'bg-success/12 text-success',
  },
  {
    label: '异常数据库',
    value: resourcesLoaded.value ? dbStatus.value.abnormal : '-',
    icon: TriangleAlertIcon,
    tone: 'bg-destructive/10 text-destructive',
  },
  {
    label: '配额状态',
    value: quota.value ? (quota.value.llmAllowed && quota.value.databaseAllowed ? '正常' : '受限') : '-',
    icon: quota.value && !(quota.value.llmAllowed && quota.value.databaseAllowed) ? TriangleAlertIcon : CircleCheckIcon,
    tone:
      quota.value?.llmAllowed === false ? 'bg-warning/12 text-warning' : 'bg-info/12 text-info',
  },
])

const resourceRows = computed(() => [
  {
    key: 'databases',
    label: '数据库',
    icon: DatabaseIcon,
    count: counts.value.databases,
    hint: 'DuckLake 逻辑库（含就绪 / 未就绪）',
  },
  {
    key: 's3',
    label: 'S3 对象存储',
    icon: CloudUploadIcon,
    count: counts.value.s3Objects,
    hint: '当前项目前缀下的对象文件',
  },
  {
    key: 'gofunctions',
    label: '云函数',
    icon: CodeIcon,
    count: counts.value.gofunctions,
    hint: '已部署的 Go 云函数',
  },
  {
    key: 'cronjobs',
    label: '定时任务',
    icon: TimerIcon,
    count: counts.value.cronjobs,
    hint: '云函数调度任务（含停用）',
  },
  {
    key: 'agents',
    label: '云 Agent',
    icon: BotIcon,
    count: counts.value.agents,
    hint: '项目内智能体配置',
  },
])

async function load() {
  loading.value = true
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
  if (all.some((r) => r.status === 'rejected')) {
    toast.error('部分数据加载失败')
  }
  if (all.slice(0, 6).some((r) => r.status === 'fulfilled')) {
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
