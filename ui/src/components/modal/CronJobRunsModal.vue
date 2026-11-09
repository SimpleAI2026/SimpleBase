<template>
  <SbModal
    :open="open"
    :title="t('components.cronRuns.title', { name: job?.name || '' })"
    :description="job ? t('components.cronRuns.descriptionWithJob', { file: job.funcFile, export: job.funcExport, count: job.runCount }) : t('components.cronRuns.description')"
    :max-width="720"
    :hide-footer="true"
    @update:open="$emit('update:open', $event)"
  >
    <div class="flex items-center gap-2 border-b pb-3">
      <Button size="sm" :disabled="!job || triggering" @click="trigger">
        <Spinner v-if="triggering" data-icon="inline-start" />
        <PlayIcon v-else data-icon="inline-start" />
        {{ t('components.cronRuns.runNow') }}
      </Button>
      <Button size="sm" variant="outline" :disabled="pending" @click="load">
        <Spinner v-if="pending" data-icon="inline-start" />
        <RefreshCwIcon v-else data-icon="inline-start" />
        {{ t('common.refresh') }}
      </Button>
      <Select v-model="statusFilter">
        <SelectTrigger class="ml-auto h-8 w-28">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{{ t('components.cronRuns.filterAll') }}</SelectItem>
          <SelectItem value="completed">{{ t('components.cronRuns.filterCompleted') }}</SelectItem>
          <SelectItem value="failed">{{ t('components.cronRuns.filterFailed') }}</SelectItem>
        </SelectContent>
      </Select>
    </div>

    <div class="max-h-[60vh] overflow-y-auto pt-3">
      <SbAsyncRegion
        class="space-y-3"
        block="spinner"
        :pending="pending"
        :show-skeleton="showSkeleton"
        :show-empty="showEmpty"
        :show-error="showError"
        :refreshing="refreshing"
        :error="error"
        :empty-title="statusFilter === 'all' ? t('components.cronRuns.emptyTitleAll') : t('components.cronRuns.emptyTitleFiltered')"
        :empty-description="statusFilter === 'all' ? t('components.cronRuns.emptyDescAll') : ''"
        @retry="load"
      >
      <SbEmptyState
        v-if="hasData && !filteredRuns.length"
        :title="t('components.cronRuns.emptyTitleFiltered')"
        description=""
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
            {{ run.trigger === 'manual' ? t('components.cronRuns.triggerManual') : t('components.cronRuns.triggerScheduled') }}
          </Badge>
          <span class="ml-auto text-xs text-muted-foreground">{{ formatTime(run.createdAt) }}</span>
        </div>
        <div class="mt-2 flex items-center gap-3 text-xs text-muted-foreground">
          <span>{{ t('components.cronRuns.duration', { n: run.durationMs }) }}</span>
          <template v-if="run.finishedAt">
            <span>{{ t('components.cronRuns.finishedAt', { time: formatTime(run.finishedAt) }) }}</span>
          </template>
        </div>
        <div v-if="run.responseJson" class="mt-2">
          <div class="mb-1 flex items-center justify-between">
            <span class="text-xs font-medium text-muted-foreground">{{ t('components.cronRuns.returnValue') }}</span>
            <Button variant="ghost" size="sm" class="h-6 px-2" @click="copy(run.responseJson)">
              <CopyIcon class="size-3.5" />
              {{ t('common.copy') }}
            </Button>
          </div>
          <pre class="sb-mono max-h-40 overflow-auto rounded-md bg-muted/60 p-2 text-xs whitespace-pre-wrap">{{ prettyJson(run.responseJson) }}</pre>
        </div>
        <div v-if="run.error" class="mt-2">
          <div class="mb-1 flex items-center justify-between">
            <span class="text-xs font-medium text-destructive">{{ t('components.cronRuns.errorLabel') }}</span>
            <Button variant="ghost" size="sm" class="h-6 px-2 text-destructive" @click="copy(run.error)">
              <CopyIcon class="size-3.5" />
              {{ t('common.copy') }}
            </Button>
          </div>
          <pre class="max-h-32 overflow-auto rounded-md bg-destructive/10 p-2 text-xs whitespace-pre-wrap text-destructive">{{ run.error }}</pre>
        </div>
      </div>
      </SbAsyncRegion>
    </div>
  </SbModal>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
/**
 * 定时任务运行记录弹窗。
 * 打开即加载；autoTrigger 时打开后自行触发并 1.5s 轮询直至 running 收敛；关闭即停止轮询。
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { CopyIcon, PlayIcon, RefreshCwIcon } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import SbModal from './SbModal.vue'
import SbAsyncRegion from '../SbAsyncRegion.vue'
import SbEmptyState from '../SbEmptyState.vue'
import { useLoadState } from '../../composables/useLoadState'
import { api } from '../../services/api'
import type { CronJobItem, CronJobRunItem } from '../../services/api'
import { useProjectStore } from '../../stores/project'
import { runStatusVariant } from '@/lib/status'
import { formatTime } from '../../utils/format'

const props = defineProps<{
  open: boolean
  job?: CronJobItem
  autoTrigger?: boolean
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  triggered: []
}>()

const { t } = useI18n()
const projectStore = useProjectStore()
const projectId = computed(() => projectStore.projectId)

const runs = ref<CronJobRunItem[]>([])
const triggering = ref(false)
const { pending, hasData, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({
  fallback: 'components.cronRuns.loadFailed'
})
const statusFilter = ref<'all' | 'completed' | 'failed'>('all')

const filteredRuns = computed(() => {
  if (statusFilter.value === 'all') return runs.value
  return runs.value.filter((r) => r.status === statusFilter.value)
})

function load() {
  const job = props.job
  if (!job) return Promise.resolve()
  return run(async () => {
    const data = await api.cronjobs.runs(projectId.value, job.id, 50)
    runs.value = data
    return data.length > 0
  })
}

/** 触发后轮询直至最新记录不再 running；弹窗关闭后立即停止轮询 */
async function trigger() {
  if (!props.job || triggering.value) return
  triggering.value = true
  try {
    await api.cronjobs.trigger(projectId.value, props.job.id)
    toast.success(t('components.cronRuns.triggered'))
    emit('triggered')
    await load()
    for (let i = 0; i < 10; i++) {
      await new Promise((r) => setTimeout(r, 1500))
      if (!props.open) break
      await load()
      const latest = runs.value[0]
      if (!latest || latest.status !== 'running') break
    }
  } catch (e) {
    toast.error(errorMessage(e, t('components.cronRuns.triggerFailed')))
  } finally {
    triggering.value = false
  }
}

watch(
  () => props.open,
  (open) => {
    if (!open) return
    statusFilter.value = 'all'
    void load().then(() => {
      if (props.open && props.autoTrigger) void trigger()
    })
  }
)

function runVariant(status: string) {
  return runStatusVariant(status) === 'outline' ? 'secondary' : runStatusVariant(status)
}
function runText(status: string) {
  if (status === 'completed') return t('cron.completed')
  if (status === 'failed') return t('cron.failed')
  if (status === 'running') return t('cron.running')
  return status || t('components.cronRuns.statusUnknown')
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
    .then(() => toast.success(t('components.cronRuns.copied')))
    .catch(() => toast.error(t('components.cronRuns.copyFailed')))
}
</script>
