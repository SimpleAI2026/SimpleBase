<template>
  <!-- AgentWorkspaceSidebar：工作台左栏（planv6.7 v2 §4.1）。
       顶部搜索 + [+ 新建协作群]；所有群（含 #通用 / 告警 / 统计 / 定时）都可展开，
       展开后是各自的会话列表；所有群都可管理成员与群设置。
       搜索只过滤导航，不改变选中会话。 -->
  <div class="flex min-h-0 flex-col gap-2 md:h-full" data-agent-workspace-sidebar>
    <div class="flex shrink-0 items-center gap-1">
      <div class="relative min-w-0 flex-1">
        <SearchIcon
          aria-hidden="true"
          class="pointer-events-none absolute top-1/2 left-2 size-3.5 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          v-model="keyword"
          class="h-8 pl-7 pr-6 text-xs"
          :placeholder="t('components.agentWorkspace.search')"
          :aria-label="t('components.agentWorkspace.search')"
        />
        <button
          v-if="keyword"
          type="button"
          class="absolute top-1/2 right-1.5 flex size-4 -translate-y-1/2 items-center justify-center rounded-full text-muted-foreground hover:text-foreground"
          :aria-label="t('pages.agents.searchClear')"
          @click="keyword = ''"
        >
          <XIcon class="size-3" />
        </button>
      </div>
      <Button
        size="icon-sm"
        variant="outline"
        class="shrink-0"
        :aria-label="t('components.agentWorkspace.newTag')"
        :title="t('components.agentWorkspace.newTag')"
        data-sidebar-new-tag
        @click="emit('create-tag')"
      >
        <PlusIcon />
      </Button>
    </div>

    <nav class="min-h-0 flex-1 overflow-y-auto" :aria-label="t('components.agentWorkspace.openNav')">
      <div
        v-for="group in visibleGroups"
        :key="group.id"
        class="flex flex-col"
        :data-sidebar-group="group.id"
      >
        <!-- 群行：#tag 直接切换分区；会话常显，不做折叠。 -->
        <div class="group/row flex items-center gap-1">
          <button
            type="button"
            class="flex min-w-0 flex-1 items-center gap-1.5 rounded-md px-1.5 py-1.5 text-left text-xs font-medium hover:bg-muted"
            :class="group.id === section ? 'bg-muted' : ''"
            :aria-current="group.id === section ? 'page' : undefined"
            :data-sidebar-section="group.id"
            @click="selectGroup(group.id)"
          >
            <component :is="groupIcon()" class="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
            <span class="min-w-0 flex-1 truncate">{{ groupName(group) }}</span>
            <span
              v-if="pendingCount(group.id)"
              class="shrink-0 rounded-full bg-primary/15 px-1.5 text-[10px] font-medium text-primary"
              :aria-label="t('components.agentWorkspace.pendingHuman', { n: pendingCount(group.id) })"
              :data-sidebar-pending="group.id"
            >{{ pendingCount(group.id) }}</span>
            <span
              v-else-if="threadsOf(group.id).length"
              class="hidden shrink-0 text-[10px] text-muted-foreground/70 xl:inline"
              data-sidebar-count
            >{{ threadsOf(group.id).length }}</span>
          </button>
          <!-- 群行 +：加已有助手 / 新建助手并加入 / 新建会话。 -->
          <Popover>
            <PopoverTrigger as-child>
              <button
                type="button"
                class="flex size-6 shrink-0 items-center justify-center rounded text-muted-foreground opacity-0 hover:text-foreground group-hover/row:opacity-100"
                :aria-label="t('components.agentWorkspace.groupActions')"
                :data-sidebar-group-add="group.id"
                @click.stop
              >
                <PlusIcon class="size-3.5" />
              </button>
            </PopoverTrigger>
            <PopoverContent align="end" class="w-52 p-1">
              <button type="button" :class="MENU_ITEM" :data-group-add-existing="group.id" @click="emit('add-members', group.id)">
                {{ t('components.agentWorkspace.addExistingAgent') }}
              </button>
              <button type="button" :class="MENU_ITEM" :data-group-create-agent="group.id" @click="emit('create-member-agent', group.id)">
                {{ t('components.agentWorkspace.createAgentAndJoin') }}
              </button>
              <button type="button" :class="MENU_ITEM" :data-group-new-thread="group.id" @click="emit('create-thread', group.id)">
                {{ t('components.agentWorkspace.newThread') }}
              </button>
            </PopoverContent>
          </Popover>
          <!-- 群行 ⋯：群设置 / 归档。 -->
          <Popover>
            <PopoverTrigger as-child>
              <button
                type="button"
                class="flex size-6 shrink-0 items-center justify-center rounded text-muted-foreground opacity-0 hover:text-foreground group-hover/row:opacity-100"
                :aria-label="t('pages.agents.moreActions')"
                :data-sidebar-group-more="group.id"
                @click.stop
              >
                <EllipsisIcon class="size-3.5" />
              </button>
            </PopoverTrigger>
            <PopoverContent align="end" class="w-44 p-1">
              <button type="button" :class="MENU_ITEM" @click="emit('edit-tag', group.id)">{{ t('components.agentWorkspace.editTag') }}</button>
              <ConfirmAction
                v-if="!group.builtinKey"
                :title="t('components.agentWorkspace.archiveTagConfirm', { name: groupName(group) })"
                @confirm="emit('archive-tag', group.id)"
              >
                <button type="button" :class="[MENU_ITEM, 'text-destructive']">{{ t('components.agentWorkspace.archiveTag') }}</button>
              </ConfirmAction>
            </PopoverContent>
          </Popover>
        </div>

        <!-- 会话列表：常显，独立分页。 -->
        <div class="flex flex-col pl-5">
          <button
            v-for="th in visibleThreads(group.id)"
            :key="th.id"
            type="button"
            class="group flex min-h-8 items-center gap-1.5 rounded-md px-2 py-1.5 text-left text-xs hover:bg-muted"
            :class="th.id === threadId ? 'bg-muted font-medium' : 'text-muted-foreground'"
            :aria-current="th.id === threadId ? 'page' : undefined"
            :data-sidebar-thread="th.id"
            @click="emit('select-thread', th.id, group.id)"
          >
            <LoaderCircleIcon
              v-if="runningThreadIds?.has(th.id)"
              class="sb-spin size-3.5 shrink-0 text-primary"
              :aria-label="t('components.threadSwitcher.running')"
            />
            <component v-else :is="kindIcon(th.thread_kind)" class="size-3.5 shrink-0 opacity-70" aria-hidden="true" />
            <span class="min-w-0 flex-1 truncate">{{ threadTitle(th) }}</span>
            <span class="hidden shrink-0 text-[10px] text-muted-foreground/70 xl:inline">{{ relativeTime(th.updated_at) }}</span>
            <Popover>
              <PopoverTrigger as-child>
                <button
                  type="button"
                  class="hidden size-5 shrink-0 items-center justify-center rounded text-muted-foreground hover:text-foreground group-hover:flex"
                  :aria-label="t('pages.agents.moreActions')"
                  @click.stop
                >
                  <EllipsisIcon class="size-3.5" />
                </button>
              </PopoverTrigger>
              <PopoverContent align="end" class="w-40 p-1">
                <button type="button" :class="MENU_ITEM" @click="emit('rename-thread', th.id)">{{ t('components.agentWorkspace.renameThread') }}</button>
                <ConfirmAction
                  :title="t('components.threadSwitcher.deleteConfirm')"
                  @confirm="emit('remove-thread', th.id, group.id)"
                >
                  <button type="button" :class="[MENU_ITEM, 'text-destructive']">{{ t('components.agentWorkspace.archiveThread') }}</button>
                </ConfirmAction>
              </PopoverContent>
            </Popover>
          </button>
          <p v-if="pageOf(group.id).loading" class="px-2 py-1.5 text-xs text-muted-foreground">{{ t('common.loading') }}</p>
          <p v-else-if="!visibleThreads(group.id).length" class="px-2 py-1.5 text-xs text-muted-foreground">
            {{ t('components.agentWorkspace.emptyGroup') }}
          </p>
          <button
            v-if="pageOf(group.id).hasMore && !keyword.trim()"
            type="button"
            class="rounded-md px-2 py-1.5 text-left text-xs text-muted-foreground hover:text-foreground"
            :data-sidebar-load-more="group.id"
            @click="emit('load-more', group.id)"
          >{{ t('components.agentWorkspace.loadMore') }}</button>
        </div>
      </div>
      <p v-if="q && !visibleGroups.length" class="px-2 py-3 text-xs text-muted-foreground">
        {{ t('pages.agents.searchNoMatch') }}
      </p>
    </nav>

    <SbAsyncRegion
      class="shrink-0"
      block="lines"
      :pending="pending"
      :show-skeleton="pending && !tags.length"
      :show-empty="false"
      :show-error="!!error"
      :refreshing="false"
      :error="error"
      @retry="emit('reload')"
    />
  </div>
</template>

<script setup lang="ts">
/**
 * 工作台左栏：群树 + 每群会话（planv6.7 v2 §4.1）。
 *
 * 只负责渲染与事件转发；取数与选中态由宿主（AgentManager）持有。
 * 所有标签都可展开并展示各自的会话；群上显示待办角标，便于发现待处理事项。
 */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  BarChart3Icon,
  BellIcon,
  ClockIcon,
  EllipsisIcon,
  HashIcon,
  LoaderCircleIcon,
  PlusIcon,
  SearchIcon,
  XIcon
} from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import ConfirmAction from '@/components/ConfirmAction.vue'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import { relativeTime } from '@/utils/relative-time'
import type { AgentTag, AgentThread, AgentThreadKind } from '@/services/types'

/** 每个群的会话分页（宿主传入，保持单一数据源）。 */
export interface SidebarGroupPage {
  threads: AgentThread[]
  hasMore: boolean
  loading: boolean
}

const props = defineProps<{
  tags: AgentTag[]
  /** 每个群的分页数据；缺省时按空列表渲染。 */
  groupPages: Record<string, SidebarGroupPage>
  /** 当前分区：默认 'general'，也可以是其他标签 ID。 */
  section: string
  threadId: string
  runningThreadIds?: Set<string>
  /** 每个群的待办数（群行角标）。 */
  pendingByGroup?: Record<string, number>
  pending?: boolean
  error?: string
}>()

const emit = defineEmits<{
  (e: 'select-section', id: string): void
  (e: 'select-thread', id: string, groupId: string): void
  (e: 'create-tag'): void
  (e: 'create-thread', groupId: string): void
  (e: 'load-more', groupId: string): void
  (e: 'rename-thread', id: string): void
  (e: 'remove-thread', id: string, groupId: string): void
  (e: 'edit-tag', id: string): void
  (e: 'archive-tag', id: string): void
  (e: 'add-members', groupId: string): void
  (e: 'create-member-agent', groupId: string): void
  (e: 'reload'): void
}>()

const { t } = useI18n()

const MENU_ITEM = 'flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs hover:bg-muted disabled:pointer-events-none disabled:opacity-50'

const keyword = ref('')

/** 自动命名豁免名单：这些标题显示为本地化的「未命名会话」。 */
const UNTITLED_TITLES = new Set(['New thread', '云 Agent', '云助手', '云助手'])

function threadTitle(th: AgentThread): string {
  const title = th.title?.trim() || ''
  return UNTITLED_TITLES.has(title) ? t('components.agentWorkspace.untitled') : title || t('components.agentWorkspace.untitled')
}

function groupName(tag: AgentTag): string {
  if (tag.builtinKey) return t(`components.agentWorkspace.builtinTags.${tag.builtinKey}`)
  return tag.name
}

/** 群行统一用 # 前缀（planv6.7 v2 UI 定稿：#tag 就是群）。 */
function groupIcon() {
  return HashIcon
}

/** 会话类型图标（普通 / 告警 / 统计 / 定时）。 */
function kindIcon(kind?: AgentThreadKind) {
  switch (kind) {
    case 'alert':
      return BellIcon
    case 'statistics':
      return BarChart3Icon
    case 'scheduled':
      return ClockIcon
    default:
      return HashIcon
  }
}

const q = computed(() => keyword.value.trim().toLowerCase())

function threadsOf(groupId: string): AgentThread[] {
  return props.groupPages[groupId]?.threads ?? []
}

function pageOf(groupId: string): SidebarGroupPage {
  return props.groupPages[groupId] ?? { threads: [], hasMore: false, loading: false }
}

/** 搜索同时过滤群名与会话标题：群名命中时显示其全部会话。 */
const visibleGroups = computed(() => {
  if (!q.value) return props.tags
  return props.tags.filter((tag) => {
    if (groupName(tag).toLowerCase().includes(q.value)) return true
    return threadsOf(tag.id).some((th) => threadTitle(th).toLowerCase().includes(q.value))
  })
})

function visibleThreads(groupId: string): AgentThread[] {
  if (!q.value) return threadsOf(groupId)
  return threadsOf(groupId).filter((th) => threadTitle(th).toLowerCase().includes(q.value))
}

function pendingCount(groupId: string): number {
  return props.pendingByGroup?.[groupId] ?? 0
}

function selectGroup(id: string) {
  if (keyword.value.trim()) keyword.value = ''
  emit('select-section', id)
}
</script>
