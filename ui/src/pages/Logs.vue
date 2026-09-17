<template>
  <ProjectScope>
  <PageContainer subtitle="按项目查询运行日志与保留策略">
    <Card>
      <CardHeader class="border-b">
        <CardTitle>日志</CardTitle>
        <CardDescription>按级别、关键字与时间范围过滤</CardDescription>
      </CardHeader>
      <CardContent class="flex flex-wrap items-center gap-3 border-b py-5">
        <Select :model-value="level" @update:model-value="(v: string) => (level = v || undefined)">
          <SelectTrigger class="w-30">
            <SelectValue placeholder="级别" />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value="info">info</SelectItem>
              <SelectItem value="warn">warn</SelectItem>
              <SelectItem value="error">error</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
        <InputGroup class="min-w-55 max-w-80">
          <InputGroupAddon>
            <FilterIcon />
          </InputGroupAddon>
          <InputGroupInput v-model="keyword" placeholder="关键字" />
        </InputGroup>
        <Input v-model="from" type="datetime-local" class="w-52" />
        <span class="text-xs text-muted-foreground">至</span>
        <Input v-model="to" type="datetime-local" class="w-52" />
        <Button :disabled="loading" @click="load">
          <Spinner v-if="loading" data-icon="inline-start" />
          <RefreshCwIcon v-else data-icon="inline-start" />
          刷新
        </Button>
        <div class="flex items-center gap-2">
          <Switch :checked="autoRefresh" @update:checked="autoRefresh = $event" />
          <span class="text-sm text-muted-foreground">{{ autoRefresh ? '轮询' : '手动' }}</span>
        </div>
        <span class="w-full text-xs text-muted-foreground sm:ml-auto sm:w-auto">{{ events.length }} 条</span>
      </CardContent>
    </Card>

    <Card>
      <CardHeader class="border-b">
        <CardTitle>保留策略</CardTitle>
      </CardHeader>
      <CardContent class="flex flex-wrap items-center gap-3 pt-5">
        <span class="text-sm text-foreground">保留天数</span>
        <Input v-model="keepDaysText" type="number" class="w-24" min="1" max="365" />
        <Button variant="outline" :disabled="savingRetention" @click="saveRetention">
          <Spinner v-if="savingRetention" data-icon="inline-start" />
          保存
        </Button>
        <span v-if="retentionUpdatedAt" class="text-xs text-muted-foreground">更新于 {{ retentionUpdatedAt }}</span>
      </CardContent>
    </Card>

    <Card>
      <CardHeader class="border-b">
        <CardTitle>日志</CardTitle>
      </CardHeader>
      <CardContent class="p-0">
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
            <TableEmpty v-if="!paged.length && !loading" :colspan="5">
              <SbEmptyState description="暂无匹配日志" />
            </TableEmpty>
            <TableRow v-for="record in paged" :key="record.id" class="hover:bg-muted/40 font-mono text-xs">
              <TableCell class="text-muted-foreground">{{ formatTime(record.occurredAt) }}</TableCell>
              <TableCell>
                <Badge :variant="logLevelVariant(record.level)">{{ record.level || '-' }}</Badge>
              </TableCell>
              <TableCell class="text-muted-foreground">{{ record.logger }}</TableCell>
              <TableCell class="max-w-md truncate font-sans text-xs text-foreground">{{ record.message }}</TableCell>
              <TableCell class="max-w-48 truncate text-muted-foreground">{{ record.requestId }}</TableCell>
            </TableRow>
          </TableBody>
        </Table>
        <div class="flex items-center justify-between gap-3 border-t border-border px-4 py-3 sm:px-5">
          <TablePager
            :page="page"
            :page-size="pageSize"
            :total="total"
            :page-count="pageCount"
            @update:page="page = $event"
          />
        </div>
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
import { logLevelVariant } from '@/lib/status'
import { formatTime } from '../utils/format'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import SbEmptyState from '../components/SbEmptyState.vue'
import TablePager from '../components/TablePager.vue'

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
