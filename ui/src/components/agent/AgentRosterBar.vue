<template>
  <!-- AgentRosterBar：标签分区的底部助手条（planv6.7 AGC-UI §4.3）。
       md+ 显示在 composer 上方；<md 折叠进 popover。点击 = 草稿插入 @名称，
       不发送、不切换页面。 -->
  <div class="flex min-w-0 items-center gap-2" data-agent-roster-bar>
    <Popover v-model:open="expanded">
      <PopoverTrigger as-child>
        <Button size="xs" variant="ghost" class="shrink-0 md:hidden" :aria-label="t('components.agentWorkspace.openNav')">
          <UsersIcon />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" class="w-64 p-2">
        <div class="flex flex-col gap-1">
          <button
            v-for="a in agents"
            :key="a.id"
            type="button"
            class="flex min-h-8 w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs hover:bg-muted"
            :data-roster-agent="a.id"
            @click="pick(a)"
          >
            <component :is="iconForAgentModule(a.module)" class="size-3.5 shrink-0 opacity-80" aria-hidden="true" />
            <span class="min-w-0 flex-1 truncate">{{ a.name }}</span>
            <LoaderCircleIcon v-if="runningAgentIds?.has(a.id)" class="sb-spin size-3 shrink-0 text-primary" :aria-label="t('components.threadSwitcher.running')" />
          </button>
        </div>
      </PopoverContent>
    </Popover>

    <div class="hidden min-w-0 flex-wrap items-center gap-1.5 md:flex">
      <span class="shrink-0 text-xs text-muted-foreground">{{ t('components.agentTagOverview.agents') }}</span>
      <button
        v-for="a in agents"
        :key="a.id"
        type="button"
        class="flex max-w-40 items-center gap-1.5 rounded-full border border-border bg-card px-2.5 py-1 text-xs hover:border-primary/40 hover:bg-primary/5"
        :data-roster-agent="a.id"
        :title="a.name"
        @click="pick(a)"
      >
        <component :is="iconForAgentModule(a.module)" class="size-3.5 shrink-0 opacity-80" aria-hidden="true" />
        <span class="min-w-0 truncate">{{ a.name }}</span>
        <LoaderCircleIcon v-if="runningAgentIds?.has(a.id)" class="sb-spin size-3 shrink-0 text-primary" :aria-label="t('components.threadSwitcher.running')" />
      </button>
      <span v-if="!agents.length" class="text-xs text-muted-foreground">{{ t('components.agentWorkspace.noAvailableAgent') }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * 底部助手条：点击注入 @点名草稿（planv6.7 §4.3）。
 * 仅展示与转发；草稿写入由宿主经 AgentComposer.insertMention 完成。
 */
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { LoaderCircleIcon, UsersIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { iconForAgentModule } from '@/components/nav-icons'
import type { MentionAgentLike } from '@/services/types'

defineProps<{
  agents: MentionAgentLike[]
  runningAgentIds?: Set<string>
}>()

const emit = defineEmits<{ (e: 'mention-agent', agent: MentionAgentLike): void }>()

const { t } = useI18n()
const expanded = ref(false)

function pick(a: MentionAgentLike) {
  expanded.value = false
  emit('mention-agent', a)
}
</script>
