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
      @click="$emit('send')"
    >
      <ArrowUpOutlined />
    </button>
  </div>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { PlusOutlined, AudioOutlined, ArrowUpOutlined, BorderOutlined } from '@ant-design/icons-vue'

const props = withDefaults(
  defineProps<{
    modelValue: string
    placeholder?: string
    disabled?: boolean
    sending?: boolean
  }>(),
  {
    placeholder: '输入消息，Enter 发送，Shift+Enter 换行',
    disabled: false,
    sending: false
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', v: string): void
  (e: 'send'): void
  (e: 'stop'): void
}>()

const ta = ref<HTMLTextAreaElement | null>(null)

function onInput(e: Event) {
  const el = e.target as HTMLTextAreaElement
  emit('update:modelValue', el.value)
  autosize()
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    if (!props.sending && props.modelValue.trim()) emit('send')
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
</style>
