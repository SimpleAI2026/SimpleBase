<template>
  <!-- AgentComposer：对话输入区（planv4.1 §3.2）。发送/停止切换；@ 点名。 -->
  <AiChatComposer
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
</script>
