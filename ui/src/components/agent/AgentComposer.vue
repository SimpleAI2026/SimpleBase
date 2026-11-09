<template>
  <!-- AgentComposer：对话输入区（planv4.1 §3.2）。发送/停止切换；@ 只点名助手。 -->
  <AiChatComposer
    ref="inner"
    v-model="model"
    :sending="sending"
    :disabled="disabled"
    :placeholder="placeholder"
    :mention-agents="mentionAgents"
    @send="(mentions) => $emit('send', model, mentions)"
    @stop="$emit('stop')"
  />
</template>

<script setup lang="ts">
import { ref } from 'vue'
import AiChatComposer from '../ai/AiChatComposer.vue'
import type { MentionAgent } from '../ai/AiChatComposer.vue'

const model = defineModel<string>({ default: '' })

withDefaults(
  defineProps<{
    sending?: boolean
    disabled?: boolean
    placeholder?: string
    mentionAgents?: MentionAgent[]
  }>(),
  { sending: false, disabled: false, mentionAgents: () => [] }
)

defineEmits<{
  (e: 'send', text: string, mentions: { agent_id: string }[]): void
  (e: 'stop'): void
}>()

const inner = ref<{ focus: () => void } | null>(null)

/** 透传聚焦能力：页面点提示词后把焦点交给输入框（planv6.0 §3.5）。 */
defineExpose({ focus: () => inner.value?.focus() })
</script>
