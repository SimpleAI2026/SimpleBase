<template>
  <ProjectScope>
  <PageContainer title="监控大盘" subtitle="系统运行状态与数据库概览">
    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <Card v-for="card in cards" :key="card.label">
        <CardContent class="px-5 py-4">
          <div v-if="loading" class="flex items-start gap-3.5">
            <Skeleton class="size-11 rounded-md" />
            <div class="flex flex-1 flex-col gap-2">
              <Skeleton class="h-3 w-20" />
              <Skeleton class="h-7 w-16" />
            </div>
          </div>
          <div v-else class="flex items-start gap-3.5">
            <div
              class="flex size-11 shrink-0 items-center justify-center rounded-md"
              :class="card.tone"
            >
              <component :is="card.icon" />
            </div>
            <div class="min-w-0">
              <div class="text-[12px] tracking-wide text-muted-foreground uppercase">{{ card.label }}</div>
              <div class="mt-0.5 text-[26px] leading-tight font-semibold">{{ card.value }}</div>
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
          <Button :disabled="loading" @click="load">
            <Spinner v-if="loading" data-icon="inline-start" />
            <RefreshCwIcon v-else data-icon="inline-start" />
            刷新数据
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent class="flex flex-col gap-4">
        <div v-if="trend.length" class="flex min-h-40 items-end gap-3 px-1 pt-2">
          <div v-for="p in trend" :key="p.date" class="flex flex-1 flex-col items-center gap-2">
            <div class="flex h-[120px] items-end gap-1">
              <div
                class="w-2.5 rounded-t bg-primary"
                :style="{ height: barHeight(p.requests, maxRequests) }"
                :title="`请求 ${p.requests}`"
              />
              <div
                class="w-2.5 rounded-t bg-destructive"
                :style="{ height: barHeight(p.errors, maxRequests) }"
                :title="`错误 ${p.errors}`"
              />
            </div>
            <div class="text-xs text-muted-foreground">{{ p.date }}</div>
          </div>
        </div>
        <SbEmptyState v-else-if="!loading" description="暂无趋势数据" />

        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="w-40">名称</TableHead>
              <TableHead>ID</TableHead>
              <TableHead class="w-28">状态</TableHead>
              <TableHead class="w-44">创建时间</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableEmpty v-if="!paged.length && !loading" :colspan="4">
              <SbEmptyState description="暂无数据库" action-text="去创建" @action="goDatabases" />
            </TableEmpty>
            <TableRow v-for="record in paged" :key="record.id">
              <TableCell>{{ record.name }}</TableCell>
              <TableCell class="max-w-48 truncate">{{ record.id }}</TableCell>
              <TableCell>
                <Badge :variant="statusBadgeVariant(record.status)">{{ record.status }}</Badge>
              </TableCell>
              <TableCell>{{ formatTime(record.createdAt) }}</TableCell>
            </TableRow>
          </TableBody>
        </Table>
        <TablePager
          :page="page"
          :page-size="pageSize"
          :total="total"
          :page-count="pageCount"
          @update:page="page = $event"
        />
      </CardContent>
    </Card>
  </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { CircleCheckIcon, DatabaseIcon, RefreshCwIcon, TriangleAlertIcon } from '@lucide/vue'
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
  TableEmpty,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { api, isMock } from '../services/api'
import type { DatabaseItem, QuotaStatus, TrendPoint } from '../services/api'
import { useProjectStore } from '../stores/project'
import { usePagination } from '../composables/usePagination'
import { statusBadgeVariant } from '@/lib/status'
import { formatTime } from '../utils/format'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import SbEmptyState from '../components/SbEmptyState.vue'
import TablePager from '../components/TablePager.vue'

const router = useRouter()
const projectStore = useProjectStore()

const databases = ref<DatabaseItem[]>([])
const quota = ref<QuotaStatus | null>(null)
const loading = ref(false)
const lastUpdate = ref('')
const trend = ref<TrendPoint[]>([])
const summary = ref({ totalRequests: 0, errorRate: 0, avgLatencyMs: 0, activeDatabases: 0 })
const { page, pageSize, total, pageCount, items: paged } = usePagination(databases)

const maxRequests = computed(() => Math.max(1, ...trend.value.map((p) => p.requests)))

function barHeight(value: number, max: number) {
  return `${Math.max(4, Math.round((value / max) * 120))}px`
}

const cards = computed(() => [
  {
    label: '数据库总数',
    value: databases.value.length,
    icon: DatabaseIcon,
    tone: 'bg-primary/12 text-primary',
  },
  {
    label: '就绪数据库',
    value: databases.value.filter((d) => d.status === 'ready').length,
    icon: CircleCheckIcon,
    tone: 'bg-success/12 text-success',
  },
  {
    label: '异常数据库',
    value: databases.value.filter((d) => ['degraded', 'deleting'].includes(d.status)).length,
    icon: TriangleAlertIcon,
    tone: 'bg-destructive/10 text-destructive',
  },
  {
    label: '配额状态',
    value: quota.value ? (quota.value.llmAllowed && quota.value.databaseAllowed ? '正常' : '受限') : '-',
    icon: TriangleAlertIcon,
    tone:
      quota.value?.llmAllowed === false ? 'bg-warning/12 text-warning' : 'bg-info/12 text-info',
  },
])

async function load() {
  loading.value = true
  const [dbRes, quotaRes, trendRes, summaryRes] = await Promise.allSettled([
    api.databases.list(projectStore.id),
    api.quota.status(projectStore.id),
    api.metrics.trend(projectStore.id),
    api.metrics.summary(projectStore.id),
  ])
  if (dbRes.status === 'fulfilled') databases.value = dbRes.value
  if (quotaRes.status === 'fulfilled') quota.value = quotaRes.value
  if (trendRes.status === 'fulfilled') trend.value = trendRes.value
  if (summaryRes.status === 'fulfilled') summary.value = summaryRes.value
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
