<template>
  <!-- AgentTagOverview：标签空态总览（planv6.7 AGC-UI §4.2）。
       上半屏：标签名 + 助手条 + 提示词卡片；下半屏：最近会话列表。 -->
  <div class="flex min-h-0 flex-1 flex-col overflow-y-auto" data-agent-tag-overview>
    <div class="flex flex-col items-center gap-4 py-8">
      <span class="flex size-11 items-center justify-center rounded-full bg-primary/10 text-primary">
        <HashIcon class="size-5" aria-hidden="true" />
      </span>
      <p class="text-sm font-medium text-foreground">#{{ name }}</p>
      <Button
        v-if="agents.length"
        size="sm"
        variant="outline"
        data-tag-new-thread
        @click="emit('new-thread')"
      >{{ t('components.agentTagOverview.newThread') }}</Button>
      <p v-else-if="!loading" class="max-w-md text-center text-xs text-muted-foreground">
        {{ t('components.agentWorkspace.noAvailableAgent') }}
      </p>
    </div>

    <!-- 助手条：点击 = 草稿插入 @名称（不切换页面）。 -->
    <div v-if="agents.length" class="flex flex-col gap-2">
      <p class="text-xs font-medium text-muted-foreground">{{ t('components.agentTagOverview.agents') }}</p>
      <div class="flex flex-wrap gap-2">
        <button
          v-for="a in agents"
          :key="a.id"
          type="button"
          class="flex items-center gap-2 rounded-full border border-border bg-card px-3 py-1.5 text-xs hover:border-primary/40 hover:bg-primary/5"
          :data-tag-agent="a.id"
          @click="emit('mention-agent', a)"
        >
          <component :is="iconForAgentModule(a.module)" class="size-3.5 shrink-0 opacity-80" aria-hidden="true" />
          <span class="max-w-32 truncate">{{ a.name }}</span>
          <LoaderCircleIcon v-if="runningAgentIds?.has(a.id)" class="sb-spin size-3 shrink-0 text-primary" :aria-label="t('components.threadSwitcher.running')" />
        </button>
      </div>
    </div>

    <!-- 提示词卡片：点击 = 填入草稿（不自动发送）。 -->
    <div v-if="prompts.length" class="mt-6 flex flex-col gap-2">
      <p class="text-xs font-medium text-muted-foreground">{{ t('components.agentTagOverview.prompts') }}</p>
      <div class="grid gap-2 sm:grid-cols-2">
        <button
          v-for="(p, i) in prompts"
          :key="p.id || i"
          type="button"
          class="flex min-h-11 items-start gap-2 rounded-lg border border-border bg-card px-3 py-2 text-left text-xs leading-relaxed text-muted-foreground transition-colors hover:border-primary/40 hover:bg-primary/5 hover:text-foreground"
          :data-tag-prompt-index="i"
          @click="emit('pick-prompt', p.content)"
        >
          <SparklesIcon class="mt-0.5 size-3.5 shrink-0 text-primary" aria-hidden="true" />
          <span class="min-w-0">{{ p.title }}</span>
        </button>
      </div>
    </div>

    <!-- 最近会话：继续之前的线程。 -->
    <div v-if="recentThreads.length" class="mt-6 flex flex-col gap-2">
      <p class="text-xs font-medium text-muted-foreground">{{ t('components.agentTagOverview.recent') }}</p>
      <button
        v-for="th in recentThreads"
        :key="th.id"
        type="button"
        class="flex items-center gap-2 rounded-lg border border-border bg-card px-3 py-2 text-left text-xs hover:border-primary/40 hover:bg-primary/5"
        :data-tag-recent="th.id"
        @click="emit('open-thread', th.id)"
      >
        <MessageCircleIcon class="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
        <span class="min-w-0 flex-1 truncate">{{ th.title || t('components.agentWorkspace.untitled') }}</span>
        <span class="shrink-0 text-muted-foreground">{{ relativeTime(th.updated_at) }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * 标签分区空态总览：助手条 + 提示词卡片 + 最近会话（planv6.7 §4.2）。
 * 所有交互只 emit：宿主决定填草稿、插点名还是切会话。
 */
import { useI18n } from 'vue-i18n'
import { HashIcon, LoaderCircleIcon, MessageCircleIcon, SparklesIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { iconForAgentModule } from '@/components/nav-icons'
import { relativeTime } from '@/utils/relative-time'
import type { AgentTagPrompt, AgentThread, MentionAgentLike } from '@/services/types'

defineProps<{
  name: string
  /** 标签内可用助手（含 module，渲染图标）。 */
  agents: MentionAgentLike[]
  prompts: AgentTagPrompt[]
  recentThreads: AgentThread[]
  runningAgentIds?: Set<string>
  /** 助手列表尚未返回：不显示「无可用助手」误报。 */
  loading?: boolean
}>()

const emit = defineEmits<{
  (e: 'new-thread'): void
  (e: 'mention-agent', agent: MentionAgentLike): void
  (e: 'pick-prompt', content: string): void
  (e: 'open-thread', id: string): void
}>()

const { t } = useI18n()
</script>
