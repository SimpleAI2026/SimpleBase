<template>
  <!-- AgentComposer：对话输入区（planv4.1 §3.2）。发送/停止切换；@ 点名。 -->
  <AiChatComposer
    ref="inner"
    v-model="model"
    :sending="sending"
    :disabled="disabled"
    :placeholder="placeholder"
    :mention-agents="mentionAgents"
    :mention-skills="mentionSkills"
    @send="(mentions, skills) => $emit('send', model, mentions, skills || [])"
    @stop="$emit('stop')"
  />
</template>

<script setup lang="ts">
import { ref } from 'vue'
import AiChatComposer from '../ai/AiChatComposer.vue'
import type { MentionAgent, MentionSkill } from '../ai/AiChatComposer.vue'

const model = defineModel<string>({ default: '' })

withDefaults(
  defineProps<{
    sending?: boolean
    disabled?: boolean
    placeholder?: string
    mentionAgents?: MentionAgent[]
    mentionSkills?: MentionSkill[]
  }>(),
  { sending: false, disabled: false, mentionAgents: () => [], mentionSkills: () => [] }
)

defineEmits<{
  (e: 'send', text: string, mentions: { agent_id: string }[], skills: string[]): void
  (e: 'stop'): void
}>()

const inner = ref<{ focus: () => void } | null>(null)

/** 透传聚焦能力：页面点提示词后把焦点交给输入框（planv6.0 §3.5）。 */
defineExpose({ focus: () => inner.value?.focus() })
</script>
