<template>
  <div class="ai-chat">
    <div v-if="showToolbar" class="ai-chat-toolbar">
      <slot name="toolbar">
        <a-select
          v-if="modelOptions.length"
          :value="model"
          style="min-width: 200px"
          placeholder="模型"
          allow-clear
          :options="modelOptions"
          @update:value="(v: string) => $emit('update:model', v)"
        />
        <a-switch
          :checked="streaming"
          checked-children="流式"
          un-checked-children="整段"
          @update:checked="(v: boolean) => $emit('update:streaming', v)"
        />
        <a-button :disabled="sending || !messages.length" @click="clear">清空会话</a-button>
      </slot>
    </div>

    <div ref="listEl" class="ai-chat-list">
      <slot name="empty">
        <SbEmptyState v-if="!messages.length" description="开始一段对话吧" />
      </slot>
      <div
        v-for="(m, i) in messages"
        :key="i"
        class="ai-msg"
        :class="m.role === 'user' ? 'ai-msg-user' : 'ai-msg-assistant'"
      >
        <span class="ai-avatar">
          <UserOutlined v-if="m.role === 'user'" />
          <RobotOutlined v-else />
        </span>
        <div class="ai-bubble">
          <pre>{{ m.content }}<span v-if="sending && streaming && i === messages.length - 1" class="ai-cursor">▍</span></pre>
        </div>
      </div>
    </div>

    <AiChatComposer
      v-model="draft"
      :sending="sending"
      :disabled="disabled"
      :placeholder="placeholder"
      @send="onSend"
      @stop="stop"
    />
  </div>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { RobotOutlined, UserOutlined } from '@ant-design/icons-vue'
import SbEmptyState from '../SbEmptyState.vue'
import AiChatComposer from './AiChatComposer.vue'
import { useAiChat } from '../../composables/useAiChat'

const props = withDefaults(
  defineProps<{
    projectId: string
    model?: string
    streaming?: boolean
    placeholder?: string
    disabled?: boolean
    showToolbar?: boolean
    modelOptions?: { label: string; value: string }[]
  }>(),
  {
    streaming: true,
    placeholder: '输入消息，Enter 发送，Shift+Enter 换行',
    disabled: false,
    showToolbar: true,
    modelOptions: () => []
  }
)

const emit = defineEmits<{
  (e: 'update:model', v: string | undefined): void
  (e: 'update:streaming', v: boolean): void
  (e: 'sent', payload: { role: string; content: string }): void
  (e: 'finished'): void
}>()

const draft = ref('')
const listEl = ref<HTMLElement | null>(null)

const chat = useAiChat({
  projectId: () => props.projectId,
  model: () => props.model,
  streaming: () => props.streaming !== false
})

const { messages, sending, send, stop, clear } = chat

async function onSend() {
  const text = draft.value
  if (!text.trim()) return
  draft.value = ''
  emit('sent', { role: 'user', content: text.trim() })
  await send(text)
  emit('finished')
}

watch(
  () => {
    const last = messages.value[messages.value.length - 1]
    return `${messages.value.length}:${last?.content.length || 0}`
  },
  async () => {
    await nextTick()
    if (listEl.value) listEl.value.scrollTop = listEl.value.scrollHeight
  }
)

defineExpose({ clear, stop, messages })
</script>

<style scoped>
.ai-chat {
  display: flex;
  flex-direction: column;
  gap: var(--sb-space-3);
}
.ai-chat-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sb-space-2);
  align-items: center;
}
.ai-chat-list {
  min-height: 240px;
  max-height: 480px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: var(--sb-space-3);
  padding: 4px 0;
}
.ai-msg {
  display: flex;
  gap: var(--sb-space-2);
  align-items: flex-start;
}
.ai-msg-user {
  flex-direction: row-reverse;
}
.ai-avatar {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: var(--sb-bg-soft);
  border: 1px solid var(--sb-border-soft);
  color: var(--sb-text-secondary);
  flex-shrink: 0;
}
.ai-msg-user .ai-avatar {
  color: var(--sb-primary);
  border-color: var(--sb-primary);
}
.ai-bubble {
  max-width: 75%;
  padding: 8px 12px;
  border-radius: var(--sb-radius-sm);
  background: var(--sb-bg-soft);
  border: 1px solid var(--sb-border-soft);
}
.ai-msg-user .ai-bubble {
  background: var(--sb-primary-light);
  border-color: transparent;
}
.ai-bubble pre {
  margin: 0;
  font-family: inherit;
  font-size: var(--sb-fs-sm);
  line-height: var(--sb-lh-relaxed);
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--sb-text);
}
.ai-cursor {
  color: var(--sb-primary);
  animation: ai-blink 1s step-end infinite;
}
@keyframes ai-blink {
  50% {
    opacity: 0;
  }
}
@media (max-width: 768px) {
  .ai-bubble {
    max-width: 85%;
  }
}
</style>
