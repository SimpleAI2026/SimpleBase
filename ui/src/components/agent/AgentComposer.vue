<template>
  <!-- AgentComposer：对话输入区（planv4.1 §3.2）。发送/停止切换；@ 只点名助手。
       planv6.7 AGC-UI：routeHint 显示「自动选择助手 / 本轮：@X」，透传点名注入能力。 -->
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
        v-if="routeHint"
        class="hidden max-w-40 shrink-0 truncate text-xs text-muted-foreground sm:block"
        data-route-hint
        aria-live="polite"
        :title="routeHint"
      >{{ routeHint }}</span>
      <slot name="actions" />
    </template>
  </AiChatComposer>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import AiChatComposer from '../ai/AiChatComposer.vue'
import type { MentionAgent, MentionSendTarget } from '../ai/AiChatComposer.vue'

const model = defineModel<string>({ default: '' })

withDefaults(
  defineProps<{
    sending?: boolean
    disabled?: boolean
    placeholder?: string
    mentionAgents?: MentionAgent[]
    /** planv6.7 AGC-UI：「自动选择助手」或「本轮：@X」。 */
    routeHint?: string
  }>(),
  { sending: false, disabled: false, mentionAgents: () => [], routeHint: '' }
)

defineEmits<{
  (e: 'send', text: string, mentions: MentionSendTarget[]): void
  (e: 'stop'): void
}>()

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
