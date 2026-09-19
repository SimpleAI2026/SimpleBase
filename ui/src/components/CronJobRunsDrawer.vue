<template>
  <Sheet :open="open" @update:open="$emit('update:open', $event)">
    <SheetContent side="right" class="w-full sm:max-w-lg">
      <SheetHeader class="border-b">
        <SheetTitle class="sb-mono">运行记录 · {{ job?.name || '' }}</SheetTitle>
        <SheetDescription>
          <span v-if="job">{{ job.funcFile }}.{{ job.funcExport }} · 已执行 {{ job.runCount }} 次</span>
          <span v-else>任务运行历史</span>
        </SheetDescription>
      </SheetHeader>

      <div class="flex items-center gap-2 border-b px-4 py-3">
        <Button size="sm" :disabled="!job || triggering" @click="trigger">
          <PlayIcon data-icon="inline-start" />
          立即执行
        </Button>
        <Button size="sm" variant="outline" :disabled="loading" @click="load">
          <RefreshCwIcon data-icon="inline-start" />
          刷新
        </Button>
        <Select v-model="statusFilter">
          <SelectTrigger class="ml-auto h-8 w-28">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部</SelectItem>
            <SelectItem value="completed">成功</SelectItem>
            <SelectItem value="failed">失败</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div class="min-h-0 flex-1 space-y-3 overflow-y-auto p-4">
        <div v-if="loading && !runs.length" class="flex justify-center py-8">
          <Spinner />
        </div>
        <SbEmptyState
          v-else-if="!filteredRuns.length"
          :title="statusFilter === 'all' ? '还没有运行记录' : '没有匹配的记录'"
          :description="statusFilter === 'all' ? '到点或手动触发后此处显示每次执行' : ''"
        />
        <div
          v-for="run in filteredRuns"
          :key="run.id"
          class="rounded-lg border border-border bg-card p-3"
        >
          <div class="flex items-center gap-2">
            <Badge :variant="runVariant(run.status)" class="gap-1">
              {{ runIcon(run.status) }} {{ runText(run.status) }}
            </Badge>
            <Badge variant="outline" class="text-xs">
              {{ run.trigger === 'manual' ? '手动' : '定时' }}
            </Badge>
            <span class="ml-auto text-xs text-muted-foreground">{{ formatTime(run.createdAt) }}</span>
          </div>
          <div class="mt-2 flex items-center gap-3 text-xs text-muted-foreground">
            <span>耗时 {{ run.durationMs }}ms</span>
            <template v-if="run.finishedAt">
              <span>完成于 {{ formatTime(run.finishedAt) }}</span>
            </template>
          </div>
          <div v-if="run.responseJson" class="mt-2">
            <div class="mb-1 flex items-center justify-between">
              <span class="text-xs font-medium text-muted-foreground">返回值</span>
              <Button variant="ghost" size="sm" class="h-6 px-2" @click="copy(run.responseJson)">
                <CopyIcon class="size-3.5" />
                复制
              </Button>
            </div>
            <pre class="sb-mono max-h-40 overflow-auto rounded-md bg-muted/60 p-2 text-xs whitespace-pre-wrap">{{ prettyJson(run.responseJson) }}</pre>
          </div>
          <div v-if="run.error" class="mt-2">
            <div class="mb-1 flex items-center justify-between">
              <span class="text-xs font-medium text-destructive">错误</span>
              <Button variant="ghost" size="sm" class="h-6 px-2 text-destructive" @click="copy(run.error)">
                <CopyIcon class="size-3.5" />
                复制
              </Button>
            </div>
            <pre class="max-h-32 overflow-auto rounded-md bg-destructive/10 p-2 text-xs whitespace-pre-wrap text-destructive">{{ run.error }}</pre>
          </div>
        </div>
      </div>
    </SheetContent>
  </Sheet>
</template>

<script setup lang="ts">
/**
 * 定时任务运行记录抽屉（ui-cronjob-plan §7.4）。
 * 打开即加载；触发后 1.5s 轮询一次直至 running 记录收敛；状态筛选前端过滤。
 */
import { computed, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { CopyIcon, PlayIcon, RefreshCwIcon } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle
} from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'
import SbEmptyState from './SbEmptyState.vue'
import { api } from '../services/api'
import type { CronJobItem, CronJobRunItem } from '../services/api'
import { useProjectStore } from '../stores/project'
import { formatTime } from '../utils/format'

const props = defineProps<{
  open: boolean
  job?: CronJobItem
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  triggered: []
}>()

const projectStore = useProjectStore()
const projectId = computed(() => projectStore.projectId)

const runs = ref<CronJobRunItem[]>([])
const loading = ref(false)
const triggering = ref(false)
const statusFilter = ref<'all' | 'completed' | 'failed'>('all')

const filteredRuns = computed(() => {
  if (statusFilter.value === 'all') return runs.value
  return runs.value.filter((r) => r.status === statusFilter.value)
})

async function load() {
  if (!props.job) return
  loading.value = true
  try {
    runs.value = await api.cronjobs.runs(projectId.value, props.job.id, 50)
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '加载运行记录失败')
  } finally {
    loading.value = false
  }
}

/** 触发后轮询直至最新记录不再 running（§7.4） */
async function trigger() {
  if (!props.job || triggering.value) return
  triggering.value = true
  try {
    await api.cronjobs.trigger(projectId.value, props.job.id)
    toast.success('已触发')
    emit('triggered')
    await load()
    for (let i = 0; i < 10; i++) {
      await new Promise((r) => setTimeout(r, 1500))
      await load()
      const latest = runs.value[0]
      if (!latest || latest.status !== 'running') break
    }
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '触发失败')
  } finally {
    triggering.value = false
  }
}

watch(
  () => props.open,
  (open) => {
    if (open) {
      statusFilter.value = 'all'
      load()
    }
  }
)

function runVariant(status: string) {
  if (status === 'completed') return 'default' as const
  if (status === 'failed') return 'destructive' as const
  return 'secondary' as const
}
function runText(status: string) {
  if (status === 'completed') return '成功'
  if (status === 'failed') return '失败'
  if (status === 'running') return '执行中'
  return status || '未知'
}
function runIcon(status: string) {
  if (status === 'completed') return '●'
  if (status === 'failed') return '✕'
  return '◌'
}

function prettyJson(s: string) {
  try {
    return JSON.stringify(JSON.parse(s), null, 2)
  } catch {
    return s
  }
}

function copy(text: string) {
  navigator.clipboard
    .writeText(text)
    .then(() => toast.success('已复制'))
    .catch(() => toast.error('复制失败'))
}
</script>
