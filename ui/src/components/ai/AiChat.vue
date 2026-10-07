<template>
  <div class="flex flex-col gap-3">
    <div v-if="showToolbar" class="flex flex-wrap items-center gap-2">
      <slot name="toolbar">
        <Select
          v-if="modelOptions.length"
          :model-value="model"
          @update:model-value="(v: string) => $emit('update:model', v)"
        >
          <SelectTrigger class="min-w-50">
            <SelectValue placeholder="模型" />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem v-for="opt in modelOptions" :key="opt.value" :value="opt.value">
                {{ opt.label }}
              </SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
        <div class="flex items-center gap-2">
          <Switch
            :checked="streaming"
            @update:checked="(v: boolean) => $emit('update:streaming', v)"
          />
          <span class="text-sm">{{ streaming ? '流式' : '整段' }}</span>
        </div>
        <Button variant="outline" :disabled="sending || !messages.length" @click="clear">清空会话</Button>
      </slot>
    </div>

    <MessageScroller :follow-key="followKey">
      <slot name="empty">
        <SbEmptyState v-if="!messages.length" description="开始一段对话吧" />
      </slot>
      <div
        v-for="(m, i) in messages"
        :key="i"
        :class="cn('flex items-start gap-2', m.role === 'user' && 'flex-row-reverse')"
      >
        <div
          :class="cn(
            'flex size-7 shrink-0 items-center justify-center rounded-full border bg-muted text-muted-foreground',
            m.role === 'user' && 'border-primary text-primary',
          )"
        >
          <UserIcon v-if="m.role === 'user'" />
          <BotIcon v-else />
        </div>
        <div class="flex min-w-0 max-w-[75%] flex-col gap-1 max-[768px]:max-w-[85%]">
          <div
            :class="cn(
              'rounded-lg border px-3 py-2',
              m.role === 'user' ? 'bg-primary/12 border-transparent' : 'bg-muted border-border',
            )"
          >
            <div v-if="m.toolCalls?.length" class="mb-1.5 flex flex-col gap-1.5">
              <AgentToolCard v-for="(t, ti) in m.toolCalls" :key="t.call_id || ti" :tool="t" />
            </div>
            <details v-if="m.thinking" class="mb-2 text-xs text-muted-foreground"><summary>思考过程</summary><pre class="whitespace-pre-wrap">{{ m.thinking }}</pre></details>
            <pre class="m-0 font-sans text-sm leading-relaxed whitespace-pre-wrap break-words text-foreground">{{ m.content }}<span v-if="sending && streaming && i === messages.length - 1" class="text-primary">▍</span></pre>
            <p v-if="m.error" class="mt-2 text-sm text-destructive">{{ m.error }} <Button size="xs" variant="outline" @click="$emit('retry')">重试</Button></p>
            <span v-if="m.canceled" class="mt-2 text-xs text-muted-foreground">已停止</span>
          </div>
        </div>
      </div>
    </MessageScroller>

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
import { computed, ref } from 'vue'
import { BotIcon, UserIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import AgentToolCard from './AgentToolCard.vue'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import SbEmptyState from '../SbEmptyState.vue'
import MessageScroller from '../chat/MessageScroller.vue'
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
  (e: 'retry'): void
}>()

const draft = ref('')

const chat = useAiChat({
  projectId: () => props.projectId,
  model: () => props.model,
  streaming: () => props.streaming !== false
})

const messages = computed(() => props.messages || chat.messages.value)
const sending = computed(() => (props.customSend ? !!props.sending : chat.sending.value))
const followKey = computed(() => {
  const list = messages.value
  const last = list[list.length - 1]
  return `${list.length}:${last?.content.length || 0}`
})

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

defineExpose({ clear, stop, messages: chat.messages })
</script>
