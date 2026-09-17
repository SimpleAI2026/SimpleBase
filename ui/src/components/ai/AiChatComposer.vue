<template>
  <div class="ai-composer" :class="{ 'is-disabled': disabled }">
    <button
      type="button"
      class="ai-composer-icon-btn"
      disabled
      title="附件（即将支持）"
      aria-label="添加附件"
    >
      <PlusOutlined />
    </button>
    <textarea
      ref="ta"
      class="ai-composer-input"
      :value="modelValue"
      :placeholder="placeholder"
      :disabled="disabled || sending"
      rows="1"
      @input="onInput"
      @keydown="onKeydown"
    />
    <div v-if="mentionOpen && mentionAgents.length" class="ai-mention-pop">
      <button
        v-for="a in filteredMentions"
        :key="a.id"
        type="button"
        class="ai-mention-item"
        @mousedown.prevent="pickMention(a)"
      >
        <span class="ai-mention-at">@</span>{{ a.name }}
        <span class="ai-mention-mod">{{ a.module }}</span>
      </button>
      <div v-if="!filteredMentions.length" class="ai-mention-empty">无匹配 Agent</div>
    </div>
    <button
      type="button"
      class="ai-composer-icon-btn"
      disabled
      title="语音输入（即将支持）"
      aria-label="语音输入"
    >
      <AudioOutlined />
    </button>
    <button
      v-if="sending"
      type="button"
      class="ai-composer-send is-stop"
      title="停止"
      aria-label="停止生成"
      @click="$emit('stop')"
    >
      <BorderOutlined />
    </button>
    <button
      v-else
      type="button"
      class="ai-composer-send"
      :disabled="disabled || !modelValue.trim()"
      title="发送"
      aria-label="发送"
      @click="$emit('send', mentionsForSend(modelValue))"
    >
      <ArrowUpOutlined />
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { PlusOutlined, AudioOutlined, ArrowUpOutlined, BorderOutlined } from '@ant-design/icons-vue'

export interface MentionAgent {
  id: string
  name: string
  module?: string
}

const props = withDefaults(
  defineProps<{
    modelValue: string
    placeholder?: string
    disabled?: boolean
    sending?: boolean
    mentionAgents?: MentionAgent[]
  }>(),
  {
    placeholder: '输入消息，Enter 发送，Shift+Enter 换行；输入 @ 点名 Agent',
    disabled: false,
    sending: false,
    mentionAgents: () => []
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', v: string): void
  (e: 'send', mentions: { agent_id: string }[]): void
  (e: 'stop'): void
}>()

const ta = ref<HTMLTextAreaElement | null>(null)
const mentionOpen = ref(false)
const mentionQuery = ref('')
const selected = ref<{ agent_id: string; name: string }[]>([])

const filteredMentions = computed(() => {
  const q = mentionQuery.value.toLowerCase()
  return (props.mentionAgents || []).filter((a) => !q || a.name.toLowerCase().includes(q) || (a.module || '').includes(q))
})

function parseMentionQuery(value: string) {
  const at = value.lastIndexOf('@')
  if (at < 0) {
    mentionOpen.value = false
    mentionQuery.value = ''
    return
  }
  const after = value.slice(at + 1)
  if (after.includes(' ') || after.includes('\n')) {
    mentionOpen.value = false
    return
  }
  mentionOpen.value = props.mentionAgents.length > 0
  mentionQuery.value = after
}

function pickMention(a: MentionAgent) {
  const value = props.modelValue
  const at = value.lastIndexOf('@')
  const next = (at >= 0 ? value.slice(0, at) : value) + '@' + a.name + ' '
  emit('update:modelValue', next)
  if (!selected.value.some((m) => m.agent_id === a.id)) {
    selected.value = [...selected.value, { agent_id: a.id, name: a.name }]
  }
  mentionOpen.value = false
  nextTick(() => ta.value?.focus())
}

function mentionsForSend(text: string) {
  const found: { agent_id: string }[] = []
  for (const a of props.mentionAgents) {
    if (text.includes('@' + a.name)) found.push({ agent_id: a.id })
  }
  for (const m of selected.value) {
    if (!found.some((x) => x.agent_id === m.agent_id)) found.push({ agent_id: m.agent_id })
  }
  return found
}

function onInput(e: Event) {
  const el = e.target as HTMLTextAreaElement
  emit('update:modelValue', el.value)
  parseMentionQuery(el.value)
  autosize()
}

function onKeydown(e: KeyboardEvent) {
  if (mentionOpen.value && e.key === 'Escape') {
    mentionOpen.value = false
    return
  }
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    if (!props.sending && props.modelValue.trim()) {
      emit('send', mentionsForSend(props.modelValue))
    }
  }
}

function autosize() {
  nextTick(() => {
    const el = ta.value
    if (!el) return
    el.style.height = 'auto'
    const max = 220
    el.style.height = `${Math.min(el.scrollHeight, max)}px`
  })
}

watch(
  () => props.modelValue,
  () => autosize()
)
</script>

<style scoped>
.ai-composer {
  display: flex;
  align-items: flex-end;
  gap: 8px;
  padding: 10px 12px;
  position: relative;
  background: var(--sb-composer-bg, var(--sb-bg-soft));
  border: 1px solid var(--sb-border-soft);
  border-radius: var(--sb-composer-radius, 24px);
  transition: border-color var(--sb-dur) var(--sb-ease), box-shadow var(--sb-dur) var(--sb-ease);
}
.ai-composer:focus-within {
  border-color: var(--sb-border);
  box-shadow: 0 0 0 3px rgba(217, 119, 87, 0.12);
}
.ai-composer.is-disabled {
  opacity: 0.6;
}
.ai-composer-input {
  flex: 1;
  min-width: 0;
  min-height: 24px;
  max-height: 220px;
  resize: none;
  border: 0;
  outline: none;
  background: transparent;
  color: var(--sb-text);
  font-size: var(--sb-fs-md, 14px);
  line-height: 1.5;
  font-family: inherit;
  padding: 4px 0;
}
.ai-composer-input::placeholder {
  color: var(--sb-text-secondary);
}
.ai-composer-icon-btn {
  flex-shrink: 0;
  width: 32px;
  height: 32px;
  border-radius: 50%;
  border: 1px solid var(--sb-border-soft);
  background: var(--sb-bg, #fff);
  color: var(--sb-text-secondary);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: not-allowed;
  opacity: 0.7;
}
.ai-composer-send {
  flex-shrink: 0;
  width: 36px;
  height: 36px;
  border-radius: 50%;
  border: 0;
  background: var(--sb-composer-send-bg, #1f1e1d);
  color: #fff;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: opacity var(--sb-dur) var(--sb-ease), transform var(--sb-dur) var(--sb-ease);
}
.ai-composer-send:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}
.ai-composer-send:not(:disabled):hover {
  transform: translateY(-1px);
}
.ai-composer-send.is-stop {
  background: var(--sb-danger, #c0452f);
}
.ai-mention-pop {
  position: absolute;
  left: 48px;
  right: 48px;
  bottom: calc(100% + 8px);
  background: var(--sb-bg, #fff);
  border: 1px solid var(--sb-border-soft);
  border-radius: var(--sb-radius-sm);
  box-shadow: var(--sb-shadow, 0 8px 24px rgba(31, 30, 29, 0.08));
  max-height: 220px;
  overflow: auto;
  z-index: 5;
  padding: 4px;
}
.ai-mention-item {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  border: 0;
  background: transparent;
  text-align: left;
  padding: 8px 10px;
  border-radius: var(--sb-radius-xs, 6px);
  cursor: pointer;
  color: var(--sb-text);
  font-size: var(--sb-fs-sm);
}
.ai-mention-item:hover {
  background: var(--sb-primary-light);
}
.ai-mention-at {
  color: var(--sb-primary);
  font-weight: 600;
}
.ai-mention-mod {
  margin-left: auto;
  color: var(--sb-text-secondary);
  font-size: var(--sb-fs-xs);
}
.ai-mention-empty {
  padding: 8px 10px;
  color: var(--sb-text-secondary);
  font-size: var(--sb-fs-sm);
}
</style>
