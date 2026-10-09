<template>
  <!-- ConversationView：消息流渲染（planv4.1 §3.2）。
       工具卡支持 is_error 徽标与进度心跳；错误消息支持基于 retry_of_run_id 的重试。 -->
  <MessageScroller :follow-key="followKey">
    <slot v-if="!historyLoading" name="empty">
      <SbEmptyState v-if="!messages.length" :icon="BotIcon" description="开始一段对话吧" />
    </slot>
    <div v-for="(m, i) in messages" :key="i" :class="cn('flex items-start gap-2', m.role === 'user' && 'flex-row-reverse')">
      <div
        :class="cn(
          'flex size-7 shrink-0 items-center justify-center rounded-full border bg-muted text-muted-foreground',
          m.role === 'user' && 'border-primary text-primary',
        )"
      >
        <span v-if="m.role === 'user'" data-user-avatar class="flex size-4 items-center justify-center">
          <UserIcon class="size-4" aria-hidden="true" />
        </span>
        <span v-else class="flex size-4 items-center justify-center" :data-agent-icon="navKeyForAgentModule(m.module)">
          <component :is="iconForAgentModule(m.module)" class="size-4" aria-hidden="true" />
        </span>
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
          <details v-if="m.thinking" class="mb-2 text-xs text-muted-foreground">
            <summary>思考过程</summary>
            <pre class="whitespace-pre-wrap">{{ m.thinking }}</pre>
          </details>
          <pre class="m-0 font-sans text-sm leading-relaxed whitespace-pre-wrap break-words text-foreground">{{ m.content }}<span v-if="sending && !confirmation && i === messages.length - 1" class="text-primary">▍</span></pre>
          <p v-if="m.error" class="mt-2 flex items-center gap-2 text-sm text-destructive">
            {{ m.error }}
            <Button v-if="canRetry" size="xs" variant="outline" @click="$emit('retry')">重试</Button>
          </p>
          <span v-if="m.canceled" class="mt-2 block text-xs text-muted-foreground">已停止</span>
        </div>
      </div>
    </div>
    <ConfirmAction
      v-if="confirmation"
      controlled
      :open="true"
      title="确认破坏性操作"
      :description="confirmText"
      @confirm="$emit('confirm')"
      @cancel="$emit('deny')"
    />
  </MessageScroller>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { BotIcon, UserIcon } from '@lucide/vue'
import { iconForAgentModule, navKeyForAgentModule } from '@/components/nav-icons'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import SbEmptyState from '../SbEmptyState.vue'
import MessageScroller from '../chat/MessageScroller.vue'
import AgentToolCard from './AgentToolCard.vue'
import ConfirmAction from '../ConfirmAction.vue'
import type { ChatMsg } from '@/composables/useAiChat'

interface ConfirmationPrompt {
  message: string
  argv: string[]
}

interface Props {
  messages: ChatMsg[]
  sending?: boolean
  /** 最后一条消息是否失败且可重试（依赖后端 retry_of_run_id）。 */
  canRetry?: boolean
  /** 历史尚未返回时不要画出空会话文案。 */
  historyLoading?: boolean
  confirmation?: ConfirmationPrompt | null
}

const props = withDefaults(defineProps<Props>(), {
  sending: false,
  canRetry: false,
  historyLoading: false,
  confirmation: null
})
defineEmits<{
  (e: 'retry'): void
  (e: 'confirm'): void
  (e: 'deny'): void
}>()

const confirmText = computed(() => {
  const prompt = props.confirmation
  if (!prompt) return ''
  const argv = prompt.argv.join(' ')
  return argv ? `${prompt.message} ${argv}` : prompt.message
})

const followKey = computed(() => {
  const list = props.messages
  const last = list[list.length - 1]
  return `${list.length}:${last?.content.length || 0}`
})
</script>
