<template>
  <div class="flex flex-col gap-3" data-testid="bi-agent">
    <!-- 输入框：Enter 发送，Shift+Enter 换行；运行中发送按钮变停止 -->
    <div
      class="relative flex items-center gap-2 rounded-xl border border-border bg-background px-3 py-2.5 transition-colors focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50"
      :class="disabledReason ? 'opacity-60' : ''"
    >
      <textarea
        ref="ta"
        v-model="text"
        rows="1"
        class="m-0 block max-h-24 min-h-6 min-w-0 flex-1 resize-none self-center border-0 bg-transparent p-0 font-sans text-sm leading-6 text-foreground outline-none placeholder:text-muted-foreground"
        :placeholder="t('components.bi.agent.placeholder')"
        :disabled="Boolean(disabledReason) || sending"
        :aria-label="t('components.bi.agent.placeholder')"
        data-testid="bi-agent-input"
        @input="autosize"
        @keydown="onKeydown"
      />
      <Button
        v-if="sending"
        type="button"
        size="icon"
        class="shrink-0 self-end rounded-full bg-destructive text-white hover:bg-destructive/95"
        :title="t('components.bi.agent.stop')"
        :aria-label="t('components.bi.agent.stop')"
        data-action="stop"
        @click="emit('stop')"
      >
        <SquareIcon />
      </Button>
      <Button
        v-else
        type="button"
        size="icon"
        class="shrink-0 self-end rounded-full"
        :disabled="Boolean(disabledReason) || !text.trim()"
        :title="t('components.bi.agent.send')"
        :aria-label="t('components.bi.agent.send')"
        data-action="send"
        @click="submit"
      >
        <ArrowUpIcon />
      </Button>
    </div>

    <p v-if="disabledReason" class="text-xs text-muted-foreground" data-testid="bi-agent-disabled">{{ disabledReason }}</p>

    <div v-if="sending || statusText" class="flex flex-wrap items-center gap-2 text-xs text-muted-foreground" data-testid="bi-agent-status">
      <Spinner v-if="sending" class="size-3.5" />
      <span>{{ sending ? (statusText || t('components.bi.agent.generating')) : statusText }}</span>
    </div>

    <div v-if="failure" class="flex flex-wrap items-center gap-2" data-testid="bi-agent-failure">
      <p class="text-sm text-destructive">{{ failure }}</p>
      <Button v-if="canRetry" type="button" size="sm" variant="outline" data-action="retry" @click="emit('retry')">
        <RotateCcwIcon data-icon="inline-start" />{{ t('components.bi.agent.retry') }}
      </Button>
    </div>

    <!-- 助手的说明文字（已去掉 sb:chart 块），经 DOMPurify 渲染 -->
    <AgentMarkdown v-if="notes" :content="notes" data-testid="bi-agent-notes" />

    <!-- 被拒绝的块：写明原因，不进入看板 -->
    <ul v-if="rejected.length" class="flex flex-col gap-1" data-testid="bi-agent-rejected">
      <li v-for="(r, i) in rejected" :key="i" class="text-xs text-destructive">
        {{ t('components.bi.agent.rejected', { title: r.title, reason: reasonText(r.reason) }) }}
      </li>
    </ul>

    <div v-if="noBlock && !sending && !failure && lastPrompt" class="flex flex-wrap items-center gap-2" data-testid="bi-agent-noblock">
      <p class="text-sm text-muted-foreground">{{ t('components.bi.agent.noBlock') }}</p>
      <Button type="button" size="sm" variant="outline" data-action="ask-again" @click="emit('ask-again')">
        {{ t('components.bi.agent.askAgain') }}
      </Button>
    </div>

    <!-- 待确认区：每个预览由宿主渲染（需要请求数据），这里只负责容器与标题 -->
    <div v-if="hasPending" class="flex flex-col gap-3" data-testid="bi-agent-pending">
      <div class="flex items-center justify-between">
        <h3 class="text-sm font-medium text-foreground">{{ t('components.bi.agent.resultTitle') }}</h3>
        <Button type="button" size="sm" variant="ghost" data-action="new-chat" @click="emit('new-chat')">
          {{ t('components.bi.agent.newChat') }}
        </Button>
      </div>
      <slot name="pending" />
    </div>
    <div v-else-if="hasHistory" class="flex justify-end">
      <Button type="button" size="sm" variant="ghost" data-action="new-chat" @click="emit('new-chat')">
        {{ t('components.bi.agent.newChat') }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
// BiAgentPanel：数据看板顶部的「输入需求，由 Agent 生成」面板（planv6.7 §4.7，A1–A9）。
// 纯展示 + 输入：会话、请求、校验都在 useBiAgent，面板只发事件、渲染状态。
import { nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowUpIcon, RotateCcwIcon, SquareIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import AgentMarkdown from '@/components/agent/AgentMarkdown.vue'
import type { RejectedBlock } from '@/composables/useBiAgent'

const props = withDefaults(
  defineProps<{
    sending: boolean
    statusText?: string
    failure?: string
    canRetry?: boolean
    notes?: string
    rejected?: RejectedBlock[]
    noBlock?: boolean
    lastPrompt?: string
    hasPending?: boolean
    /** 有对话历史时显示「新对话」。 */
    hasHistory?: boolean
    /** 非空表示禁用，值是已翻译的原因文案。 */
    disabledReason?: string
  }>(),
  { statusText: '', failure: '', canRetry: false, notes: '', rejected: () => [], noBlock: false, lastPrompt: '', hasPending: false, hasHistory: false, disabledReason: '' }
)

const emit = defineEmits<{
  send: [text: string]
  stop: []
  retry: []
  'new-chat': []
  'ask-again': []
}>()

const { t } = useI18n()
const text = ref('')
const ta = ref<HTMLTextAreaElement | null>(null)

function reasonText(reason: string): string {
  const key = `components.bi.reason.${reason}`
  const translated = t(key)
  // 块级原因里除了 bi-compat 的校验码，还有 version_unsupported / title_required / type_unknown，
  // 这三个没有专门文案时回落到原码，保证用户至少看得到原因。
  return translated === key ? reason : translated
}

function submit() {
  const value = text.value.trim()
  if (!value || props.sending || props.disabledReason) return
  emit('send', value)
  text.value = ''
  autosize()
}

function onKeydown(e: KeyboardEvent) {
  // 输入法组字期间的 Enter 是确认候选词，不能当作发送。
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
    e.preventDefault()
    submit()
  }
}

function autosize() {
  nextTick(() => {
    const el = ta.value
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${Math.min(el.scrollHeight, 96)}px`
  })
}

watch(() => props.disabledReason, (reason) => { if (reason) text.value = '' })
</script>
