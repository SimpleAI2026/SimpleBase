<template>
  <!-- AgentCard：侧栏紧凑条目（planv6.0 §3.3）。
       只留 主题色图标 + 名称 + 一行描述；编辑/定时/删除移到宿主头部的「⋯」菜单。 -->
  <button
    type="button"
    class="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left transition-colors"
    :class="active ? 'bg-primary/10 text-foreground' : 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'"
    :aria-label="running ? `${t('components.agentCard.selectAria', { name: agent.name })}（${t('components.agentCard.running')}）` : t('components.agentCard.selectAria', { name: agent.name })"
    :aria-pressed="active"
    :aria-busy="running || undefined"
    @click="$emit('select')"
  >
    <!-- 正在回复：头像外圈转动的进度环（样式 B），不占额外横向空间。 -->
    <span class="relative flex size-7 shrink-0 items-center justify-center" :title="running ? t('components.agentCard.running') : undefined">
      <!-- 淡色底环 + 约 30% 的主题色弧段（有缺口），整体旋转。 -->
      <svg
        v-if="running"
        class="sb-spin pointer-events-none absolute -top-1 -left-1 size-9 text-primary"
        viewBox="0 0 36 36"
        fill="none"
        data-agent-running
        aria-hidden="true"
      >
        <circle cx="18" cy="18" r="16" stroke="currentColor" stroke-opacity="0.18" stroke-width="2.5" />
        <circle
          cx="18"
          cy="18"
          r="16"
          stroke="currentColor"
          stroke-width="2.5"
          stroke-linecap="round"
          pathLength="100"
          stroke-dasharray="30 70"
        />
      </svg>
      <span class="flex size-7 items-center justify-center rounded-full bg-primary/10 text-primary" :data-agent-icon="navKey">
        <component :is="logo" class="size-4" aria-hidden="true" />
      </span>
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
  /** 该助手有 run 正在执行（交互或定时任务）。 */
  running?: boolean
}>()
defineEmits<{ (e: 'select'): void }>()

const { t } = useI18n()

const navKey = computed(() => navKeyForAgentModule(props.agent.module))
const logo = computed(() => iconForAgentModule(props.agent.module))
</script>
