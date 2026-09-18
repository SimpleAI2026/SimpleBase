<template>
  <SbModal
    :open="open"
    :title="`定时执行：${agent?.name || ''}`"
    :width="640"
    :confirm-loading="saving"
    ok-text="保存"
    @ok="save"
    @update:open="(v: boolean) => emit('update:open', v)"
  >
    <FieldGroup>
      <Field>
        <FieldLabel for="schedule-prompt">提示词 *</FieldLabel>
        <Textarea
          id="schedule-prompt"
          v-model="form.prompt"
          :rows="4"
          placeholder="作为定时触发的用户消息发送给该 Agent，例如：检查订单表今日写入量，异常时在回复中标注。"
        />
      </Field>

      <Field>
        <FieldLabel>执行频率 (UTC)</FieldLabel>
        <div class="flex items-center justify-between gap-4">
          <Select :model-value="frequencyKey" class="min-w-50" @update:model-value="onFrequencyChange">
            <SelectTrigger class="w-full">
              <SelectValue placeholder="选择频率" />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem v-for="opt in frequencyOptions" :key="opt.value" :value="opt.value">
                  {{ opt.label }}
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
          <div class="flex items-center gap-2">
            <Switch :checked="form.enabled" @update:checked="(v: boolean) => (form.enabled = v)" />
            <span class="text-sm">{{ form.enabled ? '已启用' : '已停用' }}</span>
          </div>
        </div>
        <div v-if="frequencyKey === 'custom'" class="mt-2">
          <Input v-model="form.cron_expr" placeholder="*/15 * * * *" @blur="validateCron" />
          <p v-if="cronError" class="mt-1 text-xs text-destructive">✗ {{ cronError }}</p>
          <p v-else-if="cronNext" class="mt-1 text-xs text-muted-foreground">✓ 下次 {{ cronNext }}</p>
          <p v-else class="mt-1 text-xs text-muted-foreground">5 字段 cron，分钟粒度，按 UTC 解释</p>
        </div>
      </Field>

      <template v-if="schedule">
        <Field>
          <FieldLabel>排期信息</FieldLabel>
          <div class="grid grid-cols-2 gap-2 text-sm">
            <div class="rounded-lg border bg-muted/30 px-3 py-2">
              <div class="text-xs text-muted-foreground">下次执行</div>
              <div>{{ formatTime(schedule.next_run_at) || '—' }}</div>
            </div>
            <div class="rounded-lg border bg-muted/30 px-3 py-2">
              <div class="text-xs text-muted-foreground">上次执行</div>
              <div>{{ formatTime(schedule.last_run_at) || '—' }}</div>
            </div>
          </div>
          <p class="mt-1.5 text-xs text-muted-foreground">结果会话：Scheduled: {{ agent?.name }}</p>
        </Field>

        <Field>
          <FieldLabel>执行历史（最近 5 次）</FieldLabel>
          <SbEmptyState v-if="!runs.length" description="暂无执行记录" />
          <div v-else class="flex flex-col gap-1.5">
            <div
              v-for="r in runs.slice(0, 5)"
              :key="r.id"
              class="flex items-center justify-between gap-2 rounded-lg border px-3 py-2 text-sm"
            >
              <div class="flex min-w-0 items-center gap-2">
                <Badge :variant="statusVariant(r.status)" class="shrink-0">
                  <Spinner v-if="r.status === 'running'" data-icon="inline-start" class="size-3" />
                  {{ r.status }}
                </Badge>
                <span class="truncate text-xs text-muted-foreground">
                  {{ shortTime(r.started_at || r.created_at) }}{{ r.error ? ' · ' + r.error : '' }}
                </span>
              </div>
              <Button variant="ghost" size="xs" @click="emit('view-thread', schedule.thread_id)">查看会话</Button>
            </div>
          </div>
        </Field>
      </template>
    </FieldGroup>

    <template #footer>
      <ConfirmAction v-if="schedule" title="确认删除该定时任务？" @confirm="remove">
        <Button variant="ghost" size="sm" class="text-destructive hover:bg-destructive/10">删除</Button>
      </ConfirmAction>
      <div class="flex-1" />
      <Button variant="outline" :disabled="!schedule || triggering" @click="triggerNow">
        <Spinner v-if="triggering" data-icon="inline-start" />
        立即执行
      </Button>
      <Button variant="outline" :disabled="false" @click="emit('update:open', false)">取消</Button>
      <Button :disabled="!form.prompt.trim() || !!cronError" @click="save">
        <Spinner v-if="saving" data-icon="inline-start" />
        保存
      </Button>
    </template>
  </SbModal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
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
import SbEmptyState from '@/components/SbEmptyState.vue'
import SbModal from '@/components/modal/SbModal.vue'
import { api } from '@/services/api'
import type { AgentSchedule, AgentScheduleRun, CloudAgent } from '@/services/types'

const props = withDefaults(
  defineProps<{
    open: boolean
    agent: CloudAgent | null
    schedule: AgentSchedule | null
    projectId?: string
  }>(),
  { projectId: '' }
)

const emit = defineEmits<{
  'update:open': [value: boolean]
  saved: [schedule: AgentSchedule]
  removed: [scheduleId: string]
  'view-thread': [threadId: string]
}>()

const frequencyOptions = [
  { label: '每 15 分钟', value: '*/15 * * * *' },
  { label: '每小时', value: '0 * * * *' },
  { label: '每天 08:00 (UTC)', value: '0 8 * * *' },
  { label: '每周一 08:00 (UTC)', value: '0 8 * * 1' },
  { label: '自定义 Cron…', value: 'custom' }
]

const form = ref({ prompt: '', cron_expr: '0 8 * * *', enabled: true })
const frequencyKey = ref('0 8 * * *')
const runs = ref<AgentScheduleRun[]>([])
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
      const known = frequencyOptions.find((o) => o.value === props.schedule?.cron_expr)
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
  try {
    runs.value = await api.agentSchedules.runs(props.projectId, props.schedule.id)
  } catch {
    runs.value = []
  }
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
    cronError.value = '表达式非法：需 5 个空格分隔的字段（分 时 日 月 周）'
    cronNext.value = ''
    return
  }
  cronError.value = ''
  cronNext.value = ''
}

async function save() {
  if (!props.agent || !props.projectId) return
  if (!form.value.prompt.trim()) {
    toast.warning('请填写提示词')
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
          toast.error('该 Agent 已有定时任务，请刷新后重试')
          return
        } else {
          throw e
        }
      }
    }
    toast.success('定时任务已保存')
    emit('saved', saved)
    emit('update:open', false)
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '保存失败')
  } finally {
    saving.value = false
  }
}

async function remove() {
  if (!props.schedule || !props.projectId) return
  try {
    await api.agentSchedules.remove(props.projectId, props.schedule.id)
    toast.success('定时任务已删除')
    emit('removed', props.schedule.id)
    emit('update:open', false)
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '删除失败')
  }
}

async function triggerNow() {
  if (!props.schedule || !props.projectId || triggering.value) return
  triggering.value = true
  try {
    await api.agentSchedules.trigger(props.projectId, props.schedule.id)
    toast.success('已触发，稍后可在执行历史查看结果')
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
    toast.error(e instanceof Error ? e.message : '触发失败')
  } finally {
    triggering.value = false
  }
}

function statusVariant(status: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  if (status === 'completed') return 'default'
  if (status === 'failed') return 'destructive'
  if (status === 'running' || status === 'queued') return 'secondary'
  return 'outline'
}

function formatTime(v?: string) {
  if (!v) return ''
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleString('zh-CN', { hour12: false }) + ' UTC'
}

function shortTime(v?: string) {
  if (!v) return '—'
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return '—'
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}
</script>
