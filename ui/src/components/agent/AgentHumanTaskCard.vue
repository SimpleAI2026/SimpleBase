<template>
  <!-- AgentHumanTaskCard：人工待办卡（planv6.7 v2 §6.3）。
       与工具审批严格分离：这里只收集「人的回复/无法处理」，
       不放行任何工具执行；工具审批仍是行内 AgentConfirmCard。 -->
  <div class="rounded-lg border border-amber-500/40 bg-amber-500/5 px-3 py-2" data-agent-human-task>
    <p class="flex items-center gap-1.5 text-xs font-medium text-amber-700 dark:text-amber-400">
      <UserRoundCheckIcon class="size-3.5 shrink-0" aria-hidden="true" />
      {{ t('components.agentWorkspace.humanTaskTitle') }}
      <span v-if="task.assigneeId" class="ml-auto text-[10px] font-normal text-muted-foreground">{{ task.assigneeId }}</span>
    </p>
    <p class="mt-1 text-xs leading-relaxed text-foreground">{{ task.instruction }}</p>
    <textarea
      v-if="!readonly"
      v-model="text"
      rows="2"
      class="mt-2 w-full resize-none rounded-md border border-border bg-transparent px-2 py-1 text-xs"
      :placeholder="t('components.agentWorkspace.humanReplyPlaceholder')"
      data-human-reply-input
    />
    <div v-if="!readonly" class="mt-2 flex items-center gap-2">
      <Button size="xs" :disabled="busy" data-human-continue @click="submit('continue')">
        {{ t('components.agentWorkspace.humanContinue') }}
      </Button>
      <Button size="xs" variant="outline" :disabled="busy" data-human-unable @click="submit('unable')">
        {{ t('components.agentWorkspace.humanUnable') }}
      </Button>
    </div>
    <p v-else class="mt-1.5 text-[11px] text-muted-foreground" data-human-done>
      {{ replySummary }}
    </p>
  </div>
</template>

<script setup lang="ts">
/**
 * 人工待办卡（planv6.7 v2 §6.3）。
 *
 * readonly 态用于「已经处理过」的回放：显示结论而不是再给一次按钮，
 * 避免刷新后重复提交（服务端也有版本校验兜底）。
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { UserRoundCheckIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import type { AgentTask } from '@/services/types'

const props = defineProps<{
  task: AgentTask
  /** 正在提交（禁用按钮防重复点击）。 */
  busy?: boolean
}>()

const emit = defineEmits<{
  (e: 'reply', taskId: string, outcome: 'continue' | 'unable', text?: string): void
}>()

const { t } = useI18n()

const text = ref('')

// 换任务时清空草稿，避免把上一个待办的说明带到下一个。
watch(() => props.task.id, () => { text.value = '' })

/** 已处理（completed / failed）后不再显示输入。 */
const readonly = computed(() => props.task.status !== 'waiting_human' && props.task.status !== 'pending')

const replySummary = computed(() => {
  const reply = props.task.replyText?.trim()
  if (props.task.replyOutcome === 'unable') {
    return t('components.agentWorkspace.humanUnable') + (reply ? `：${reply}` : '')
  }
  return reply || t('components.agentWorkspace.taskStatus.completed')
})

function submit(outcome: 'continue' | 'unable') {
  emit('reply', props.task.id, outcome, text.value.trim() || undefined)
  text.value = ''
}
</script>
