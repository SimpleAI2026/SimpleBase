<template>
  <!-- AgentWelcome：空会话的欢迎页 + 提示词（planv6.0 §3.5）。
       点击提示词由宿主写入输入框并聚焦，不自动发送。
       planv6.7 AGC-UI：mode=chat-empty（分区无会话）显示「新建会话」按钮。 -->
  <div class="flex flex-col items-center gap-4 py-6">
    <span class="flex size-11 items-center justify-center rounded-full bg-primary/10 text-primary">
      <component :is="icon" class="size-5" aria-hidden="true" />
    </span>
    <div class="text-center">
      <p class="text-sm font-medium text-foreground">
        {{ t('components.agentWelcome.greeting', { name: agentName }) }}
      </p>
      <p class="mt-1 text-xs text-muted-foreground">{{ t('components.agentWelcome.subtitle') }}</p>
    </div>
    <Button
      v-if="mode === 'chat-empty'"
      size="sm"
      variant="outline"
      data-welcome-new-thread
      @click="$emit('new-thread')"
    >{{ t('components.agentWorkspace.newThread') }}</Button>
    <ul class="grid w-full max-w-xl gap-2 sm:grid-cols-2">
      <li v-for="(key, i) in prompts" :key="key">
        <button
          type="button"
          class="flex min-h-11 w-full items-center gap-2 rounded-lg border border-border bg-card px-3 py-2 text-left text-xs leading-relaxed text-muted-foreground transition-colors hover:border-primary/40 hover:bg-primary/5 hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
          :data-prompt-index="i"
          @click="$emit('pick', t(key))"
        >
          <SparklesIcon class="size-3.5 shrink-0 text-primary" aria-hidden="true" />
          <span>{{ t(key) }}</span>
        </button>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { computed, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import { BotIcon, SparklesIcon } from '@lucide/vue'
import { iconForAgentModule } from '@/components/nav-icons'
import { promptI18nKeys } from '@/constants/agent-prompts'
import { Button } from '@/components/ui/button'

const props = defineProps<{
  /** 当前选中的 Agent 名称，用于问候语。 */
  agentName?: string
  /** 当前 Agent 的 module，决定提示词集合。 */
  module?: string
  /** planv6.7 AGC-UI：chat-empty = 分区无会话（显示新建按钮）；thread-empty = 会话为空。 */
  mode?: 'chat-empty' | 'thread-empty'
}>()

defineEmits<{
  (e: 'pick', text: string): void
  (e: 'new-thread'): void
}>()

const { t } = useI18n()

const agentName = computed(() => props.agentName?.trim() || t('components.agentWelcome.defaultName'))
const icon = computed<Component>(() => (props.module ? iconForAgentModule(props.module) : BotIcon))
const prompts = computed(() => promptI18nKeys(props.module))
</script>
