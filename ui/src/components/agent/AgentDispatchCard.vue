<template>
  <!-- AgentDispatchCard：路由分工卡（planv6.7 v2 §6.2）。
       显示「路由协调 → @助手A：做什么 → @你：确认什么」，每步带状态点。
       只展示，不提供操作：重试/取消在消息流下方或输入区。 -->
  <div class="rounded-lg border border-border bg-muted/40 px-3 py-2" data-agent-dispatch-card>
    <p class="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
      <GitBranchIcon class="size-3.5 shrink-0" aria-hidden="true" />
      {{ t('components.agentWorkspace.dispatchTitle') }}
      <span v-if="total" class="ml-auto text-[10px] font-normal">{{ done }}/{{ total }}</span>
    </p>
    <ol class="mt-1.5 flex flex-col gap-1">
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
        </span>
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
  </div>
</template>

<script setup lang="ts">
/**
 * 路由分工卡：把本轮任务与状态摊开给用户看（planv6.7 v2 §6.2）。
 *
 * 为什么单独成型：一轮多任务时，如果只靠消息气泡，用户看不出「还剩几步、卡在哪」。
 * 卡片只读 + 一个重试入口，避免在卡片里堆操作。
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { GitBranchIcon } from '@lucide/vue'
import type { AgentTask } from '@/services/types'

const props = defineProps<{
  tasks: AgentTask[]
  /** agentId → 显示名（人被指派时用 assignee 名字）。 */
  names?: Record<string, string>
}>()

const emit = defineEmits<{ (e: 'retry-task', taskId: string): void }>()

const { t } = useI18n()

const total = computed(() => props.tasks.length)
const done = computed(() => props.tasks.filter((task) => task.status === 'completed').length)

/** 任务目标显示名：助手名，或「你」/指点名的接收人。 */
function labelOf(task: AgentTask): string {
  if (task.targetKind === 'human') return t('components.agentWorkspace.humanTaskTitle')
  return props.names?.[task.targetId] || task.targetId
}

/** 状态点：完成=绿、执行中=主色、等人=琥珀、失败=红。 */
function dotClass(status: string): string {
  switch (status) {
    case 'completed':
      return 'bg-emerald-500'
    case 'running':
      return 'bg-primary animate-pulse'
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
