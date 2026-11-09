<template>
  <!-- ConversationView：消息流渲染（planv4.1 §3.2）。
       工具卡支持 is_error 徽标与进度心跳；错误消息支持基于 retry_of_run_id 的重试。
       planv6.0 §3.5/§3.6：空态由本组件的 empty 插槽承载（宿主传 AgentWelcome），
       破坏性操作改用行内确认卡，不再打断对话。 -->
  <MessageScroller :follow-key="followKey">
    <slot v-if="!historyLoading && !messages.length && !sending" name="empty" />
    <div v-for="(m, i) in messages" :key="i" :class="cn('flex items-start gap-2', m.role === 'user' && 'flex-row-reverse')">
      <div
        :class="cn(
          'flex size-7 shrink-0 items-center justify-center rounded-full border bg-primary/10 text-primary',
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
            <summary>{{ t('components.conversation.thinking') }}</summary>
            <pre class="whitespace-pre-wrap">{{ m.thinking }}</pre>
          </details>
          <AgentMarkdown v-if="m.role === 'assistant' && m.content" :content="m.content" />
          <p v-else-if="m.role !== 'assistant' && m.content" class="m-0 whitespace-pre-wrap break-words text-sm leading-relaxed text-foreground">{{ m.content }}</p>
          <!-- planv6.0 v2 §1.2：忙碌态用三点跳动（头像保持模块图标）。
               role/aria-label 由容器承担（§1.3 选项 a）：Spinner 已移除，不能没有
               加载语义；prefers-reduced-motion 下三点静止但仍可见。 -->
          <span
            v-if="sending && !confirmation && i === messages.length - 1 && m.role === 'assistant'"
            data-typing-dots
            role="status"
            :aria-label="t('common.loading')"
            class="mt-1 flex h-4 items-center gap-1 text-primary"
          >
            <span class="size-1.5 animate-bounce rounded-full bg-current" aria-hidden="true" />
            <span class="size-1.5 animate-bounce rounded-full bg-current [animation-delay:-0.15s]" aria-hidden="true" />
            <span class="size-1.5 animate-bounce rounded-full bg-current [animation-delay:-0.3s]" aria-hidden="true" />
          </span>
          <p v-if="m.error" class="mt-2 flex items-center gap-2 text-sm text-destructive">
            {{ m.error }}
            <Button v-if="canRetry" size="xs" variant="outline" @click="$emit('retry')">{{ t('components.conversation.retry') }}</Button>
          </p>
          <span v-if="m.canceled" class="mt-2 block text-xs text-muted-foreground">{{ t('components.conversation.canceled') }}</span>
        </div>
      </div>
    </div>
    <AgentConfirmCard
      v-if="confirmation"
      class="mt-1"
      :message="confirmation.message"
      :argv="confirmation.argv"
      @confirm="$emit('confirm')"
      @deny="$emit('deny')"
    />
  </MessageScroller>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { UserIcon } from '@lucide/vue'
import { iconForAgentModule, navKeyForAgentModule } from '@/components/nav-icons'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import MessageScroller from '../chat/MessageScroller.vue'
import AgentMarkdown from './AgentMarkdown.vue'
import AgentToolCard from './AgentToolCard.vue'
import AgentConfirmCard from './AgentConfirmCard.vue'
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

const { t } = useI18n()

const followKey = computed(() => {
  const list = props.messages
  const last = list[list.length - 1]
  return `${list.length}:${last?.content.length || 0}:${last?.thinking?.length || 0}:${last?.toolCalls?.map((tool) => `${tool.status}:${tool.arguments?.length || 0}:${tool.content?.length || 0}`).join(',') || ''}:${props.confirmation?.message || ''}:${props.sending}`
})
</script>
