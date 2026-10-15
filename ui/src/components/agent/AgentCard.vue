<template>
  <!-- AgentCard：侧栏紧凑条目（planv6.0 §3.3）。
       只留 主题色图标 + 名称 + 一行描述；编辑/定时/删除移到宿主头部的「⋯」菜单。 -->
  <button
    type="button"
    class="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left transition-colors"
    :class="active ? 'bg-primary/10 text-foreground' : 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'"
    :aria-label="t('components.agentCard.selectAria', { name: agent.name })"
    :aria-pressed="active"
    @click="$emit('select')"
  >
    <span class="flex size-7 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary" :data-agent-icon="navKey">
      <component :is="logo" class="size-4" aria-hidden="true" />
    </span>
    <span class="flex min-w-0 flex-1 flex-col">
      <span class="flex items-center gap-1.5">
        <strong class="min-w-0 flex-1 truncate text-sm font-medium">{{ agent.name }}</strong>
        <span
          v-if="agent.builtin_key"
          class="size-1.5 shrink-0 rounded-full bg-primary"
          :title="t('components.agentCard.builtin')"
          :aria-label="t('components.agentCard.builtin')"
        />
        <span
          v-if="agent.writable === false"
          class="shrink-0 rounded border border-border px-1 text-[10px] leading-4 text-muted-foreground"
          data-agent-readonly
          :title="t('components.agentCard.readonlyHint')"
        >{{ t('components.agentCard.readonly') }}</span>
        <ClockIcon
          v-if="scheduleSummary"
          class="size-3 shrink-0 text-muted-foreground"
          :class="scheduleEnabled ? 'text-primary' : ''"
          :aria-label="scheduleSummary"
        />
      </span>
      <span class="truncate text-xs leading-relaxed text-muted-foreground">
        {{ agent.description || t('components.agentCard.noDescription') }}
      </span>
    </span>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ClockIcon } from '@lucide/vue'
import { iconForAgentModule, navKeyForAgentModule } from '@/components/nav-icons'
import type { CloudAgent } from '@/services/types'

const props = defineProps<{
  agent: CloudAgent
  active?: boolean
  /** 定时任务摘要文案（有则显示一个时钟图标）。 */
  scheduleSummary?: string
  scheduleEnabled?: boolean
}>()
defineEmits<{ (e: 'select'): void }>()

const { t } = useI18n()

const navKey = computed(() => navKeyForAgentModule(props.agent.module))
const logo = computed(() => iconForAgentModule(props.agent.module))
</script>
