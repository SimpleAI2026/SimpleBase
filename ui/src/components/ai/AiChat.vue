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
          <div v-if="m.toolCalls?.length" class="ai-tool-cards">
            <div v-for="(t, ti) in m.toolCalls" :key="ti" class="ai-tool-card">
              <span class="ai-tool-name">{{ t.name || 'tool' }}</span>
              <pre v-if="t.arguments">{{ t.arguments }}</pre>
              <pre v-if="t.content">{{ t.content }}</pre>
            </div>
          </div>
          <pre>{{ m.content }}<span v-if="sending && streaming && i === messages.length - 1" class="ai-cursor">▍</span></pre>
        </div>
      </div>
    </div>

    <AiChatComposer
      v-model="draft"
      :sending="sending"
      :disabled="disabled"
      :placeholder="placeholder"
      :mention-agents="mentionAgents"
      @send="onSend"
      @stop="stop"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { RobotOutlined, UserOutlined } from '@ant-design/icons-vue'
import SbEmptyState from '../SbEmptyState.vue'
import AiChatComposer from './AiChatComposer.vue'
import type { MentionAgent } from './AiChatComposer.vue'
import { useAiChat } from '../../composables/useAiChat'
import type { ChatMsg } from '../../composables/useAiChat'

const props = withDefaults(
  defineProps<{
    projectId: string
    model?: string
    streaming?: boolean
    placeholder?: string
    disabled?: boolean
    showToolbar?: boolean
    modelOptions?: { label: string; value: string }[]
    mentionAgents?: MentionAgent[]
    customSend?: (text: string, mentions: { agent_id: string }[]) => Promise<void>
    messages?: ChatMsg[]
    sending?: boolean
  }>(),
  {
    streaming: true,
    placeholder: '输入消息，Enter 发送，Shift+Enter 换行；输入 @ 点名 Agent',
    disabled: false,
    showToolbar: true,
    modelOptions: () => [],
    mentionAgents: () => []
  }
)

const emit = defineEmits<{
  (e: 'update:model', v: string | undefined): void
  (e: 'update:streaming', v: boolean): void
  (e: 'sent', payload: { role: string; content: string }): void
  (e: 'finished'): void
  (e: 'stop'): void
}>()

const draft = ref('')
const listEl = ref<HTMLElement | null>(null)

const chat = useAiChat({
  projectId: () => props.projectId,
  model: () => props.model,
  streaming: () => props.streaming !== false
})

const messages = computed(() => props.messages || chat.messages.value)
const sending = computed(() => (props.customSend ? !!props.sending : chat.sending.value))

async function onSend(mentions: { agent_id: string }[] = []) {
  const text = draft.value
  if (!text.trim()) return
  draft.value = ''
  emit('sent', { role: 'user', content: text.trim() })
  if (props.customSend) {
    await props.customSend(text, mentions)
  } else {
    await chat.send(text)
  }
  emit('finished')
}

function stop() {
  if (props.customSend) emit('stop')
  else chat.stop()
}

function clear() {
  if (!props.customSend) chat.clear()
}

watch(
  () => {
    const list = messages.value
    const last = list[list.length - 1]
    return `${list.length}:${last?.content.length || 0}`
  },
  async () => {
    await nextTick()
    if (listEl.value) listEl.value.scrollTop = listEl.value.scrollHeight
  }
)

defineExpose({ clear, stop, messages: chat.messages })
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
.ai-tool-cards {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 6px;
}
.ai-tool-card {
  border: 1px solid var(--sb-border-soft);
  background: var(--sb-bg, #fff);
  border-radius: var(--sb-radius-xs, 6px);
  padding: 6px 8px;
}
.ai-tool-name {
  font-size: var(--sb-fs-xs);
  color: var(--sb-primary);
  font-weight: 600;
}
.ai-tool-card pre {
  margin: 4px 0 0;
  font-family: var(--sb-font-mono);
  font-size: var(--sb-fs-xs);
  white-space: pre-wrap;
  color: var(--sb-text-secondary);
  max-height: 120px;
  overflow: auto;
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
