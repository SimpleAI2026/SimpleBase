<template>
  <!-- AgentComposer：对话输入区（planv4.1 §3.2）。发送/停止切换；@ 只点名助手。
       planv6.8 §13.9：Plan 状态条——群会话显示「分工由 Plan 决定 / 本轮 @A、@B」，栅栏决策中显示「Plan 决策中…」。 -->
  <AiChatComposer
    ref="inner"
    v-model="model"
    :sending="sending"
    :disabled="disabled"
    :placeholder="placeholder"
    :mention-agents="mentionAgents"
    @send="(mentions) => $emit('send', model, mentions)"
    @stop="$emit('stop')"
  >
    <template #actions>
      <span
        v-if="planHint"
        class="hidden max-w-56 shrink-0 truncate text-xs text-muted-foreground sm:block"
        data-plan-hint
        aria-live="polite"
        :data-planning="planning ? 'true' : undefined"
        :title="planHint"
      >
        <span v-if="planning" class="sb-dot-pulse mr-1 inline-block size-1.5 rounded-full bg-primary align-middle motion-reduce:animate-none" aria-hidden="true" />
        {{ planning ? t('components.agentWorkspace.planDeciding') : planHint }}
      </span>
      <slot name="actions" />
    </template>
  </AiChatComposer>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AiChatComposer from '../ai/AiChatComposer.vue'
import type { MentionAgent, MentionSendTarget } from '../ai/AiChatComposer.vue'

const model = defineModel<string>({ default: '' })

withDefaults(
  defineProps<{
    sending?: boolean
    disabled?: boolean
    placeholder?: string
    mentionAgents?: MentionAgent[]
    /** Plan 状态条：群会话里「分工由 Plan 决定」或「本轮 @A、@B」；空串不显示。 */
    planHint?: string
    /** Plan 栅栏决策中：状态条显示「Plan 决策中…」。 */
    planning?: boolean
  }>(),
  { sending: false, disabled: false, mentionAgents: () => [], planHint: '', planning: false }
)

defineEmits<{
  (e: 'send', text: string, mentions: MentionSendTarget[]): void
  (e: 'stop'): void
}>()

const { t } = useI18n()

const inner = ref<{
  focus: () => void
  insertMention: (a: MentionAgent) => void
  clearMentions: () => void
} | null>(null)

/** 透传聚焦与点名能力：页面点提示词/助手条后交给输入框（planv6.0 §3.5 / planv6.7 §4.3）。 */
defineExpose({
  focus: () => inner.value?.focus(),
  insertMention: (a: MentionAgent) => inner.value?.insertMention(a),
  clearMentions: () => inner.value?.clearMentions()
})
</script>
