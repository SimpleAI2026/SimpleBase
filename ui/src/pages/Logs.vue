<template>
  <ProjectScope>
  <PageContainer subtitle="按项目查询运行日志与保留策略">
    <Card>
      <CardHeader class="border-b">
        <CardTitle>日志</CardTitle>
        <CardDescription>按级别、关键字与时间范围过滤</CardDescription>
      </CardHeader>
      <CardContent class="flex flex-wrap items-end gap-2.5 border-b py-4">
        <div class="flex flex-col gap-1">
          <span class="text-xs text-muted-foreground">级别</span>
          <Select :model-value="level" @update:model-value="(v: string) => (level = v || undefined)">
            <SelectTrigger class="w-30" size="sm">
              <SelectValue placeholder="级别" />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value="info">{{ logLevelText('info') }}</SelectItem>
                <SelectItem value="warn">{{ logLevelText('warn') }}</SelectItem>
                <SelectItem value="error">{{ logLevelText('error') }}</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>
        <div class="flex min-w-55 flex-1 flex-col gap-1 sm:max-w-80">
          <span class="text-xs text-muted-foreground">关键字</span>
          <InputGroup>
            <InputGroupAddon>
              <FilterIcon />
            </InputGroupAddon>
            <InputGroupInput v-model="keyword" placeholder="关键字" />
          </InputGroup>
        </div>
        <div class="flex w-full min-w-0 items-end gap-2 sm:w-auto">
          <div class="flex min-w-0 flex-1 flex-col gap-1 sm:w-52 sm:flex-none">
            <span class="text-xs text-muted-foreground">开始</span>
            <Input v-model="from" type="datetime-local" class="w-full" />
          </div>
          <span class="pb-2 text-xs text-muted-foreground">至</span>
          <div class="flex min-w-0 flex-1 flex-col gap-1 sm:w-52 sm:flex-none">
            <span class="text-xs text-muted-foreground">结束</span>
            <Input v-model="to" type="datetime-local" class="w-full" />
          </div>
        </div>
        <Button size="sm" :disabled="loading" @click="load">
          <Spinner v-if="loading" data-icon="inline-start" />
          <RefreshCwIcon v-else data-icon="inline-start" />
          刷新
        </Button>
        <div class="flex items-center gap-2 pb-1">
          <Switch :checked="autoRefresh" @update:checked="autoRefresh = $event" />
          <span class="text-sm text-muted-foreground">{{ autoRefresh ? `每 ${POLL_SEC} 秒轮询` : '手动刷新' }}</span>
        </div>
        <span class="w-full text-xs text-muted-foreground sm:ml-auto sm:w-auto sm:pb-1">已加载 {{ events.length }} 条 / 最多 200</span>
      </CardContent>
      <div class="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="w-48">时间</TableHead>
              <TableHead class="w-24">级别</TableHead>
              <TableHead class="w-40">来源</TableHead>
              <TableHead>消息</TableHead>
              <TableHead class="w-52">Request ID</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <template v-if="loading && !events.length">
              <TableRow v-for="n in 3" :key="'sk-' + n">
                <TableCell colspan="5"><Skeleton class="h-8 w-full" /></TableCell>
              </TableRow>
            </template>
            <TableEmpty v-else-if="!paged.length" :colspan="5">
              <SbEmptyState
                description="暂无匹配日志"
                :action-text="hasFilter ? '清除筛选' : undefined"
                @action="clearFilters"
              />
            </TableEmpty>
            <TableRow v-for="record in paged" :key="record.id" class="font-mono text-xs">
              <TableCell class="text-muted-foreground">{{ formatTime(record.occurredAt) }}</TableCell>
              <TableCell>
                <Badge :variant="logLevelVariant(record.level)">{{ logLevelText(record.level) }}</Badge>
              </TableCell>
              <TableCell class="text-muted-foreground">{{ record.logger }}</TableCell>
              <TableCell class="max-w-md truncate font-sans text-xs text-foreground">{{ record.message }}</TableCell>
              <TableCell class="max-w-48 truncate text-muted-foreground">{{ record.requestId }}</TableCell>
            </TableRow>
          </TableBody>
        </Table>
        <TablePager
          variant="footer"
          :page="page"
          :page-size="pageSize"
          :total="total"
          :page-count="pageCount"
          @update:page="page = $event"
        />
      </div>
    </Card>

    <Card>
      <CardHeader class="border-b">
        <CardTitle>保留策略</CardTitle>
      </CardHeader>
      <CardContent class="flex flex-wrap items-end gap-2.5 py-4">
        <div class="flex flex-col gap-1">
          <span class="text-xs text-muted-foreground">保留天数</span>
          <Input id="keep-days" v-model="keepDaysText" type="number" class="w-24" min="1" max="365" />
        </div>
        <Button variant="outline" size="sm" :disabled="savingRetention" @click="saveRetention">
          <Spinner v-if="savingRetention" data-icon="inline-start" />
          保存
        </Button>
        <span v-if="retentionUpdatedAt" class="pb-1 text-xs text-muted-foreground">更新于 {{ retentionUpdatedAt }}</span>
      </CardContent>
    </Card>
  </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { FilterIcon, RefreshCwIcon } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableEmpty,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { api } from '../services/api'
import type { LogEvent } from '../services/api'
import { useProjectStore } from '../stores/project'
import { usePagination } from '../composables/usePagination'
import { logLevelText, logLevelVariant } from '@/lib/status'
import { formatTime } from '../utils/format'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import SbEmptyState from '../components/SbEmptyState.vue'
import TablePager from '../components/TablePager.vue'

const POLL_MS = 10000
const POLL_SEC = POLL_MS / 1000

const projectStore = useProjectStore()
const level = ref<string | undefined>()
const keyword = ref('')
const from = ref('')
const to = ref('')
const events = ref<LogEvent[]>([])
const { page, pageSize, total, pageCount, items: paged } = usePagination(events)
const loading = ref(false)
const autoRefresh = ref(false)
const keepDays = ref(14)
const keepDaysText = computed({
  get: () => String(keepDays.value),
  set: (v: string | number) => {
    const n = Number(v)
    keepDays.value = Number.isFinite(n) ? n : 14
  },
})
const savingRetention = ref(false)
const retentionUpdatedAt = ref('')
const hasFilter = computed(() => Boolean(level.value || keyword.value.trim() || from.value || to.value))
let timer: number | null = null

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
    toast.error((e as Error)?.message || '加载日志失败')
  } finally {
    loading.value = false
  }
}

function clearFilters() {
  level.value = undefined
  keyword.value = ''
  from.value = ''
  to.value = ''
  void load()
}

async function saveRetention() {
  if (!projectStore.id) return
  savingRetention.value = true
  try {
    await api.logs.putRetention(projectStore.id, keepDays.value)
    toast.success('已保存保留策略')
    await load()
  } catch (e) {
    toast.error((e as Error)?.message || '保存失败')
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
    }, POLL_MS)
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
