<template>
  <SbModal
    :open="open"
    :title="t('components.agentSchedule.title', { name: agent?.name || '' })"
    :width="640"
    :confirm-loading="saving"
    :ok-text="t('common.save')"
    @ok="save"
    @update:open="(v: boolean) => emit('update:open', v)"
  >
    <FieldGroup>
      <Field>
        <FieldLabel for="schedule-prompt">{{ t('components.agentSchedule.promptLabel') }}</FieldLabel>
        <Textarea
          id="schedule-prompt"
          v-model="form.prompt"
          :rows="4"
          :placeholder="t('components.agentSchedule.promptPlaceholder')"
        />
      </Field>

      <Field>
        <FieldLabel>{{ t('components.agentSchedule.frequencyLabel') }}</FieldLabel>
        <div class="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
          <Select :model-value="frequencyKey" @update:model-value="onFrequencyChange">
            <SelectTrigger class="min-w-0 flex-1 basis-48">
              <SelectValue :placeholder="t('components.agentSchedule.frequencyPlaceholder')" />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem v-for="opt in frequencyOptions" :key="opt.value" :value="opt.value">
                  {{ opt.label }}
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
          <div class="flex shrink-0 items-center gap-2">
            <Switch :model-value="form.enabled" @update:model-value="(v: boolean) => (form.enabled = v)" />
            <span class="text-sm">{{ form.enabled ? t('components.agentSchedule.enabled') : t('components.agentSchedule.disabled') }}</span>
          </div>
        </div>
        <div v-if="frequencyKey === 'custom'" class="mt-2">
          <Input v-model="form.cron_expr" placeholder="*/15 * * * *" @blur="validateCron" />
          <p v-if="cronError" class="mt-1 text-xs text-destructive">✗ {{ cronError }}</p>
          <p v-else-if="cronNext" class="mt-1 text-xs text-muted-foreground">{{ t('components.agentSchedule.nextRun', { time: cronNext }) }}</p>
          <p v-else class="mt-1 text-xs text-muted-foreground">{{ t('components.agentSchedule.cronHint') }}</p>
        </div>
      </Field>

      <template v-if="schedule">
        <Field>
          <FieldLabel>{{ t('components.agentSchedule.scheduleInfo') }}</FieldLabel>
          <div class="grid grid-cols-1 gap-2 text-sm sm:grid-cols-2">
            <div class="rounded-lg border bg-muted/30 px-3 py-2">
              <div class="text-xs text-muted-foreground">{{ t('components.agentSchedule.nextExecution') }}</div>
              <div>{{ formatTime(schedule.next_run_at, '—') }}</div>
            </div>
            <div class="rounded-lg border bg-muted/30 px-3 py-2">
              <div class="text-xs text-muted-foreground">{{ t('components.agentSchedule.lastExecution') }}</div>
              <div>{{ formatTime(schedule.last_run_at, '—') }}</div>
            </div>
          </div>
          <p class="mt-1.5 text-xs text-muted-foreground">{{ t('components.agentSchedule.resultThread', { name: agent?.name }) }}</p>
        </Field>

        <Field>
          <FieldLabel>{{ t('components.agentSchedule.history') }}</FieldLabel>
          <SbAsyncRegion
            block="spinner"
            :pending="pending"
            :show-skeleton="showSkeleton"
            :show-empty="showEmpty"
            :show-error="showError"
            :refreshing="refreshing"
            :error="error"
            @retry="loadRuns"
          >
            <template #empty>
              <SbEmptyState :description="t('components.agentSchedule.historyEmpty')" />
            </template>
            <div class="flex flex-col gap-1.5">
            <div
              v-for="r in runs.slice(0, 5)"
              :key="r.id"
              class="flex items-center justify-between gap-2 rounded-lg border px-3 py-2 text-sm"
            >
              <div class="flex min-w-0 items-center gap-2">
                <Badge :variant="runStatusVariant(r.status)" class="shrink-0">
                  <Spinner v-if="r.status === 'running'" data-icon="inline-start" class="size-3" />
                  {{ r.status }}
                </Badge>
                <span class="truncate text-xs text-muted-foreground">
                  {{ shortTime(r.started_at || r.created_at) }}{{ r.error ? ' · ' + r.error : '' }}
                </span>
              </div>
              <Button variant="ghost" size="xs" @click="emit('view-thread', schedule.thread_id)">{{ t('components.agentSchedule.viewThread') }}</Button>
            </div>
          </div>
          </SbAsyncRegion>
        </Field>
      </template>
    </FieldGroup>

    <template #footer>
      <ConfirmAction v-if="schedule" :title="t('components.agentSchedule.deleteConfirm')" @confirm="remove">
        <Button variant="destructiveGhost" size="sm">{{ t('components.agentSchedule.delete') }}</Button>
      </ConfirmAction>
      <div class="flex-1" />
      <Button variant="outline" :disabled="!schedule || triggering" @click="triggerNow">
        <Spinner v-if="triggering" data-icon="inline-start" />
        {{ t('components.agentSchedule.runNow') }}
      </Button>
      <Button variant="outline" :disabled="false" @click="emit('update:open', false)">{{ t('common.cancel') }}</Button>
      <Button :disabled="!form.prompt.trim() || !!cronError" @click="save">
        <Spinner v-if="saving" data-icon="inline-start" />
        {{ t('common.save') }}
      </Button>
    </template>
  </SbModal>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import ConfirmAction from '@/components/ConfirmAction.vue'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import SbEmptyState from '@/components/SbEmptyState.vue'
import SbModal from '@/components/modal/SbModal.vue'
import { useLoadState } from '@/composables/useLoadState'
import { api } from '@/services/api'
import type { AgentSchedule, AgentScheduleRun, CloudAgent } from '@/services/types'
import { runStatusVariant } from '@/lib/status'
import { formatTime, shortTime } from '@/utils/format'

const props = withDefaults(
  defineProps<{
    open: boolean
    agent: CloudAgent | null
    schedule: AgentSchedule | null
    projectId?: string
  }>(),
  { projectId: '' }
)

const { t } = useI18n()

const emit = defineEmits<{
  'update:open': [value: boolean]
  saved: [schedule: AgentSchedule]
  removed: [scheduleId: string]
  'view-thread': [threadId: string]
}>()

const FREQUENCY_OPTIONS = [
  { labelKey: 'components.agentSchedule.preset15Min', value: '*/15 * * * *' },
  { labelKey: 'components.agentSchedule.presetHourly', value: '0 * * * *' },
  { labelKey: 'components.agentSchedule.presetDaily8', value: '0 8 * * *' },
  { labelKey: 'components.agentSchedule.presetWeeklyMon8', value: '0 8 * * 1' },
  { labelKey: 'components.agentSchedule.presetCustom', value: 'custom' }
]

/** 数据源仍是 value（cron 表达式），仅 label 走 i18n。 */
const frequencyOptions = computed(() =>
  FREQUENCY_OPTIONS.map((o) => ({ label: t(o.labelKey), value: o.value }))
)

const form = ref({ prompt: '', cron_expr: '0 8 * * *', enabled: true })
const frequencyKey = ref('0 8 * * *')
const runs = ref<AgentScheduleRun[]>([])
const { pending, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({
  fallback: 'components.agentSchedule.loadingHistoryFailed'
})
const saving = ref(false)
const triggering = ref(false)
const cronError = ref('')
const cronNext = ref('')

const projectId = defineModel<string>('projectId')

watch(
  () => props.open,
  (v) => {
    if (!v) return
    cronError.value = ''
    cronNext.value = ''
    if (props.schedule) {
      form.value.prompt = props.schedule.prompt
      form.value.enabled = props.schedule.enabled
      const known = FREQUENCY_OPTIONS.find((o) => o.value === props.schedule?.cron_expr)
      if (known) {
        frequencyKey.value = known.value
        form.value.cron_expr = props.schedule.cron_expr
      } else {
        frequencyKey.value = 'custom'
        form.value.cron_expr = props.schedule.cron_expr
      }
      void loadRuns()
    } else {
      form.value = { prompt: '', cron_expr: '0 8 * * *', enabled: true }
      frequencyKey.value = '0 8 * * *'
      runs.value = []
    }
  }
)

async function loadRuns() {
  if (!props.schedule || !props.projectId) return
  const scheduleId = props.schedule.id
  const project = props.projectId
  return run(async () => {
    const list = await api.agentSchedules.runs(project, scheduleId)
    runs.value = list
    return list.length > 0
  })
}

function onFrequencyChange(v: string) {
  frequencyKey.value = v
  if (v !== 'custom') {
    form.value.cron_expr = v
    cronError.value = ''
    cronNext.value = ''
  }
}

const cronPattern = /^\S+\s+\S+\s+\S+\s+\S+\s+\S+$/

function validateCron() {
  const expr = form.value.cron_expr.trim()
  if (!cronPattern.test(expr)) {
    cronError.value = t('components.agentSchedule.cronInvalid')
    cronNext.value = ''
    return
  }
  cronError.value = ''
  cronNext.value = ''
}

async function save() {
  if (!props.agent || !props.projectId) return
  if (!form.value.prompt.trim()) {
    toast.warning(t('components.agentSchedule.promptRequired'))
    return
  }
  validateCron()
  if (cronError.value) return
  saving.value = true
  try {
    const body = {
      agent_id: props.agent.id,
      prompt: form.value.prompt.trim(),
      cron_expr: form.value.cron_expr.trim(),
      enabled: form.value.enabled
    }
    let saved: AgentSchedule
    if (props.schedule) {
      saved = await api.agentSchedules.patch(props.projectId, props.schedule.id, body)
    } else {
      try {
        saved = await api.agentSchedules.create(props.projectId, body)
      } catch (e) {
        // 已存在 → 409，自动转 PATCH 重试一次。
        if (e instanceof Object && 'status' in e && (e as { status?: number }).status === 409 && props.schedule) {
          saved = await api.agentSchedules.patch(props.projectId, props.schedule.id, body)
        } else if (
          props.schedule === null &&
          e instanceof Error &&
          /already exists/i.test(e.message)
        ) {
          toast.error(t('components.agentSchedule.alreadyScheduled'))
          return
        } else {
          throw e
        }
      }
    }
    toast.success(t('components.agentSchedule.saved'))
    emit('saved', saved)
    emit('update:open', false)
  } catch (e) {
    toast.error(errorMessage(e, t('components.agentSchedule.saveFailed')))
  } finally {
    saving.value = false
  }
}

async function remove() {
  if (!props.schedule || !props.projectId) return
  try {
    await api.agentSchedules.remove(props.projectId, props.schedule.id)
    toast.success(t('components.agentSchedule.deleted'))
    emit('removed', props.schedule.id)
    emit('update:open', false)
  } catch (e) {
    toast.error(errorMessage(e, t('components.agentSchedule.deleteFailed')))
  }
}

async function triggerNow() {
  if (!props.schedule || !props.projectId || triggering.value) return
  triggering.value = true
  try {
    await api.agentSchedules.trigger(props.projectId, props.schedule.id)
    toast.success(t('components.agentSchedule.triggered'))
    await loadRuns()
    // 轮询刷新：2s × 15 次。
    let polls = 0
    const timer = setInterval(async () => {
      polls++
      await loadRuns()
      const active = runs.value.some((r) => r.status === 'running' || r.status === 'queued')
      if (!active || polls >= 15) clearInterval(timer)
    }, 2000)
  } catch (e) {
    toast.error(errorMessage(e, t('components.agentSchedule.triggerFailed')))
  } finally {
    triggering.value = false
  }
}


</script>
