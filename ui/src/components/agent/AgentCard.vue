<template>
  <!-- AgentCard：侧边简化卡片（planv4.1 §3.2）。内置徽标 + 模块 + 一句话描述；操作收进下拉。 -->
  <article
    class="rounded-xl border bg-card p-3 transition-colors"
    :class="active ? 'border-primary/60 bg-primary/8 shadow-xs' : 'border-border hover:border-primary/30'"
  >
    <button
      type="button"
      class="block w-full rounded-md text-left"
      :aria-label="`选择 Agent ${agent.name}`"
      :aria-pressed="active"
      @click="$emit('select')"
    >
      <span class="flex items-center gap-2">
        <span class="flex size-7 shrink-0 items-center justify-center rounded-full border bg-muted text-muted-foreground" :data-agent-icon="navKey">
          <component :is="logo" class="size-4" aria-hidden="true" />
        </span>
        <strong class="min-w-0 flex-1 truncate text-sm font-semibold text-foreground">{{ agent.name }}</strong>
        <Badge v-if="agent.builtin_key" variant="secondary" class="shrink-0 text-[10px]">内置</Badge>
        <Badge variant="outline" class="shrink-0 text-[10px] text-muted-foreground">{{ moduleLabel }}</Badge>
      </span>
      <span class="mt-1.5 block text-xs leading-relaxed text-muted-foreground line-clamp-2">
        {{ agent.description || '无描述' }}
      </span>
      <span v-if="scheduleSummary" class="mt-1.5 flex items-center gap-1.5 text-[11px] text-muted-foreground">
        <ClockIcon aria-hidden="true" class="size-3" />
        <span>{{ scheduleSummary }}</span>
        <span :class="['size-1.5 rounded-full', scheduleEnabled ? 'bg-success' : 'bg-muted-foreground/40']" aria-hidden="true" />
      </span>
    </button>
    <div class="mt-2 flex justify-end gap-1 border-t border-border/60 pt-2">
      <Button variant="ghost" size="xs" @click="$emit('edit')">编辑</Button>
      <Button variant="ghost" size="xs" @click="$emit('schedule')">定时</Button>
      <ConfirmAction :title="`确认删除 Agent「${agent.name}」？`" @confirm="$emit('remove')">
        <Button variant="destructiveGhost" size="xs">删除</Button>
      </ConfirmAction>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { ClockIcon } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { iconForAgentModule, navKeyForAgentModule } from '@/components/nav-icons'
import ConfirmAction from '../ConfirmAction.vue'
import type { CloudAgent } from '@/services/types'

const props = defineProps<{
  agent: CloudAgent
  active?: boolean
  /** 定时任务摘要文案（有则显示）。 */
  scheduleSummary?: string
  scheduleEnabled?: boolean
}>()
defineEmits<{
  (e: 'select'): void
  (e: 'edit'): void
  (e: 'schedule'): void
  (e: 'remove'): void
}>()

const MODULE_LABELS: Record<string, string> = {
  general: '通用',
  database: '数据库',
  s3: '对象',
  logs: '日志',
  sandbox: '沙盒'
}
const moduleLabel = computed(() => MODULE_LABELS[props.agent.module] || props.agent.module)
const navKey = computed(() => navKeyForAgentModule(props.agent.module))
const logo = computed(() => iconForAgentModule(props.agent.module))
</script>
