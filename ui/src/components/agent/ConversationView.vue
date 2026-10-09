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
          <details v-if="m.thinking" class="mb-2 text-xs text-muted-foreground">
            <summary>思考过程</summary>
            <pre class="whitespace-pre-wrap">{{ m.thinking }}</pre>
          </details>
          <pre class="m-0 font-sans text-sm leading-relaxed whitespace-pre-wrap break-words text-foreground">{{ m.content }}<span v-if="sending && i === messages.length - 1" class="text-primary">▍</span></pre>
          <p v-if="m.error" class="mt-2 flex items-center gap-2 text-sm text-destructive">
            {{ m.error }}
            <Button v-if="canRetry" size="xs" variant="outline" @click="$emit('retry')">重试</Button>
          </p>
          <span v-if="m.canceled" class="mt-2 block text-xs text-muted-foreground">已停止</span>
        </div>
      </div>
    </div>
  </MessageScroller>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { BotIcon, UserIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import SbEmptyState from '../SbEmptyState.vue'
import MessageScroller from '../chat/MessageScroller.vue'
import AgentToolCard from './AgentToolCard.vue'
import type { ChatMsg } from '@/composables/useAiChat'

interface Props {
  messages: ChatMsg[]
  sending?: boolean
  /** 最后一条消息是否失败且可重试（依赖后端 retry_of_run_id）。 */
  canRetry?: boolean
  /** 历史尚未返回时不要画出空会话文案。 */
  historyLoading?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  sending: false,
  canRetry: false,
  historyLoading: false
})
defineEmits<{ (e: 'retry'): void }>()

const followKey = computed(() => {
  const list = props.messages
  const last = list[list.length - 1]
  return `${list.length}:${last?.content.length || 0}`
})
</script>
