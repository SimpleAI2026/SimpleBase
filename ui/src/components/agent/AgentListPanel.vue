<template>
  <!-- AgentListPanel：云助手左栏（planv6.0 §3.3）。
       顶部一行是「搜索框 + 新建图标」；条目只渲染名称与一行描述；
       <md 时改为横向滚动条，不再整块堆在对话上方。 -->
  <div class="flex min-h-0 flex-col gap-2 md:h-full">
    <!-- 搜索 + 新建：都在左栏顶部（需求 #1）。搜索框窄栏下只留图标位。 -->
    <div class="flex shrink-0 items-center gap-1">
      <div class="relative min-w-0 flex-1">
        <SearchIcon
          aria-hidden="true"
          class="pointer-events-none absolute top-1/2 left-2 size-3.5 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          v-model="keyword"
          class="h-8 pl-7 pr-6 text-xs"
          :placeholder="t('pages.agents.searchPlaceholder')"
          :aria-label="t('pages.agents.searchPlaceholder')"
        />
        <button
          v-if="keyword"
          type="button"
          class="absolute top-1/2 right-1.5 flex size-4 -translate-y-1/2 items-center justify-center rounded-full text-muted-foreground hover:text-foreground"
          :aria-label="t('pages.agents.searchClear')"
          :title="t('pages.agents.searchClear')"
          @click="keyword = ''"
        >
          <XIcon class="size-3" />
        </button>
      </div>
      <Button
        size="icon-sm"
        variant="outline"
        class="shrink-0"
        :disabled="pending"
        :title="t('pages.agents.create')"
        :aria-label="t('pages.agents.create')"
        @click="emit('create')"
      >
        <PlusIcon />
      </Button>
    </div>

    <SbAsyncRegion
      class="min-h-0 flex-1 md:overflow-y-auto"
      block="cards"
      :pending="pending"
      :show-skeleton="showSkeleton"
      :show-empty="showEmpty && !keyword.trim()"
      :show-error="showError"
      :refreshing="refreshing"
      :error="error"
      @retry="emit('reload')"
    >
      <template #empty>
        <SbEmptyState
          :icon="BotIcon"
          :description="t('pages.agents.emptyListDesc')"
          :action-text="t('pages.agents.emptyListAction')"
          @action="emit('create')"
        />
      </template>
      <div class="flex gap-2 overflow-x-auto md:flex-col md:gap-1 md:overflow-visible">
        <AgentCard
          v-for="a in filtered"
          :key="a.id"
          class="shrink-0 md:w-full"
          :agent="a"
          :active="a.id === activeId"
          :schedule-summary="a.id in scheduleByAgent ? scheduleSummary(scheduleByAgent[a.id]) : undefined"
          :schedule-enabled="!!scheduleByAgent[a.id]?.enabled"
          :running="!!runningAgentIds?.has(a.id)"
          @select="emit('select', a.id)"
        />
      </div>
      <!-- 搜索无结果：不走 SbAsyncRegion 的空态（那是「一个助手都没有」），单独提示 -->
      <p v-if="!filtered.length && agents.length" class="px-1 py-3 text-xs text-muted-foreground">
        {{ t('pages.agents.searchNoMatch') }}
      </p>
    </SbAsyncRegion>
  </div>
</template>

<script setup lang="ts">
/**
 * AgentManager 左栏：搜索 + Agent 条目列表（从 pages/AgentManager.vue 拆出）。
 *
 * 只负责渲染与事件转发；取数状态由宿主传入，保持「展示归组件、编排归页面」。
 * 编辑/定时/删除不在这一层：它们收进宿主头部的「⋯」菜单（planv6.0 §3.3）。
 * 搜索是纯展示层过滤，不上报宿主——助手数量是每项目个位数，没必要进 URL。
 */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { BotIcon, PlusIcon, SearchIcon, XIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import SbEmptyState from '@/components/SbEmptyState.vue'
import AgentCard from './AgentCard.vue'
import type { CloudAgent } from '@/services/types'
import type { AgentSchedule } from '@/services/types'

const props = defineProps<{
  agents: CloudAgent[]
  activeId: string
  pending: boolean
  showSkeleton: boolean
  showEmpty: boolean
  showError: boolean
  refreshing: boolean
  error: string
  scheduleByAgent: Record<string, AgentSchedule>
  /** 定时任务摘要格式化（宿主注入，复用同一份预设文案）。 */
  scheduleSummary: (s: AgentSchedule) => string
  /** 正在回复的助手 id 集合（交互 + 定时任务）。 */
  runningAgentIds?: Set<string>
}>()

const emit = defineEmits<{
  reload: []
  create: []
  select: [id: string]
}>()

const { t } = useI18n()

const keyword = ref('')

/** 按名称与描述过滤；大小写不敏感。 */
const filtered = computed(() => {
  const q = keyword.value.trim().toLowerCase()
  if (!q) return props.agents
  return props.agents.filter(
    (a) => a.name.toLowerCase().includes(q) || (a.description || '').toLowerCase().includes(q)
  )
})
</script>
