<template>
  <!-- AgentPlanCard：群轮次的 Plan 卡（planv6.8 §13.9）。
       头部：目标 + 复杂度徽标 + 进度条 + 进度摘要；主体：任务分工（目标、指令、依赖、风险、状态），
       complex 计划另带步骤清单；底部：失败任务重试、发现展开。与气泡区用虚线边框区分为「系统决策」。
       只展示 + 一个重试入口：取消 / 确认在 Plan 弹窗与输入区，不堆在卡片里。 -->
  <div
    v-if="plan"
    class="rounded-lg border border-dashed border-border bg-muted/40 px-3 py-2"
    data-agent-plan-card
    :data-plan-action="plan.decision.action"
  >
    <div class="flex flex-wrap items-start gap-x-2 gap-y-1">
      <h3 class="min-w-0 flex-1 text-sm font-medium text-foreground" data-plan-goal>{{ plan.decision.goal || t('components.agentWorkspace.dispatchTitle') }}</h3>
      <Badge
        v-if="plan.decision.complexity === 'complex'"
        variant="outline"
        class="shrink-0 text-[10px]"
        data-plan-complexity="complex"
      >{{ t('components.agentWorkspace.planComplex') }}</Badge>
      <span class="shrink-0 text-[10px] text-muted-foreground" data-plan-progress-label>{{ progress.done }}/{{ progress.total }}</span>
    </div>
    <div
      class="mt-1.5 h-1 w-full overflow-hidden rounded-full bg-border"
      role="progressbar"
      aria-valuemin="0"
      aria-valuemax="100"
      :aria-valuenow="percent"
      :aria-label="t('components.agentWorkspace.planProgress')"
    >
      <div class="h-full rounded-full bg-primary transition-[width]" :style="{ width: `${percent}%` }" data-plan-progress-bar />
    </div>
    <p v-if="progressNote" class="mt-1 text-xs text-muted-foreground" data-plan-summary>{{ progressNote }}</p>

    <ol v-if="showSteps" class="mt-2 flex flex-col gap-1" data-plan-steps>
      <li
        v-for="step in plan.decision.steps"
        :key="step.id"
        class="flex items-start gap-1.5 text-xs"
        :data-plan-step="step.id"
        :data-plan-step-status="stepStatus(step)"
      >
        <span class="mt-0.5 size-1.5 shrink-0 rounded-full" :class="dotClass(stepStatus(step))" aria-hidden="true" />
        <span class="font-medium">{{ step.title }}</span>
        <span v-if="step.note" class="min-w-0 truncate text-muted-foreground">{{ step.note }}</span>
      </li>
    </ol>

    <ol class="mt-2 flex flex-col gap-1" data-plan-tasks>
      <li
        v-for="(task, i) in tasks"
        :key="task.id"
        class="flex items-start gap-1.5 text-xs"
        :data-dispatch-step="task.id"
        :data-dispatch-status="task.status"
      >
        <span
          class="mt-1 size-1.5 shrink-0 rounded-full"
          :class="dotClass(task.status)"
          :aria-label="t(`components.agentWorkspace.taskStatus.${task.status}`)"
        />
        <span class="min-w-0 flex-1 leading-relaxed">
          <span class="font-medium text-foreground">{{ i + 1 }}. </span>
          <span class="text-foreground">{{ labelOf(task) }}</span>
          <span class="text-muted-foreground">：{{ task.instruction }}</span>
          <span v-if="task.dependsOn.length" class="block text-[10px] text-muted-foreground">
            {{ task.dependsOn.join(' → ') }} → {{ task.ref || task.id }}
          </span>
        </span>
        <span v-if="task.risk" class="shrink-0 text-[10px] text-muted-foreground">{{ task.risk }}</span>
        <span class="shrink-0 text-[10px] text-muted-foreground">{{ t(`components.agentWorkspace.taskStatus.${task.status}`) }}</span>
        <button
          v-if="task.status === 'failed' || task.status === 'interrupted' || task.status === 'blocked'"
          type="button"
          class="shrink-0 text-[10px] text-primary hover:underline"
          :data-dispatch-retry="task.id"
          @click="emit('retry-task', task.id)"
        >{{ t('components.agentWorkspace.retryTask') }}</button>
      </li>
    </ol>

    <details v-if="plan.decision.findings.length" class="mt-2 text-xs text-muted-foreground" data-plan-findings>
      <summary class="cursor-pointer select-none">{{ t('components.agentWorkspace.planFindings') }}</summary>
      <ul class="mt-1 list-disc pl-5">
        <li v-for="(finding, i) in plan.decision.findings" :key="i">{{ finding }}</li>
      </ul>
    </details>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Badge } from '@/components/ui/badge'
import type { AgentPlanSnapshot, AgentPlanStep, AgentTask } from '@/services/types'

const props = defineProps<{
  /** 轮次的 Plan 快照（栅栏决策 + 确认状态）；没有决策时不渲染。 */
  plan: AgentPlanSnapshot | null
  /** 本轮任务（任务状态是执行推进的事实来源）。 */
  tasks: AgentTask[]
  /** agentId → 显示名（人被指派时用「需要你处理」）。 */
  names?: Record<string, string>
}>()

const emit = defineEmits<{ (e: 'retry-task', taskId: string): void }>()

const { t } = useI18n()

const progress = computed(() => {
  const list = props.tasks
  return { done: list.filter((task) => task.status === 'completed').length, total: list.length }
})
const percent = computed(() => (progress.value.total ? Math.round((progress.value.done / progress.value.total) * 100) : 0))
/** 进度摘要：优先 Plan 自带的 summary / next，其次是任务计数。 */
const progressNote = computed(() => {
  const p = props.plan?.decision.progress
  const note = p?.note?.trim()
  if (note) return note
  return progress.value.total ? t('components.agentWorkspace.planProgressCount', { done: progress.value.done, total: progress.value.total }) : ''
})
const showSteps = computed(() => props.plan?.decision.complexity === 'complex' && props.plan.decision.steps.length > 0)

/** 步骤状态：Plan 记录的 done/in_progress/blocked/skipped 原样使用，否则按关联任务的完成度归约。 */
function stepStatus(step: AgentPlanStep): string {
  if (step.status && step.status !== 'pending') return step.status
  const linked = props.tasks.filter((task) => task.stepId === step.id)
  if (!linked.length) return step.status || 'pending'
  if (linked.every((task) => task.status === 'completed')) return 'done'
  if (linked.some((task) => task.status === 'running')) return 'in_progress'
  return step.status || 'pending'
}

/** 任务目标显示名：助手名，或「需要你处理」。 */
function labelOf(task: AgentTask): string {
  if (task.targetKind === 'human') return t('components.agentWorkspace.humanTaskTitle')
  return props.names?.[task.targetId] || task.targetId
}

/** 状态点：完成=绿、执行中=主色、等人=琥珀、失败=红。 */
function dotClass(status: string): string {
  switch (status) {
    case 'completed':
    case 'done':
      return 'bg-emerald-500'
    case 'running':
    case 'in_progress':
      return 'bg-primary animate-pulse motion-reduce:animate-none'
    case 'waiting_human':
      return 'bg-amber-500'
    case 'failed':
    case 'blocked':
    case 'interrupted':
      return 'bg-destructive'
    default:
      return 'bg-muted-foreground/40'
  }
}
</script>
