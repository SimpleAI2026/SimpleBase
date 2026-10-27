<template>
  <!-- ThreadSwitcher：对话区头部的会话下拉切换器（planv4.1 §3.2：会话列表移入头部）。
       planv6.0 §3.6 #7：只留「下拉 + 新建」，删除会话收进宿主头部的「⋯」菜单。 -->
  <div class="flex min-w-32 flex-1 items-center gap-1.5">
    <Select :model-value="activeId" @update:model-value="(id: string) => $emit('select', id)">
      <SelectTrigger class="h-8 min-w-0 flex-1" :aria-label="t('components.threadSwitcher.switchAria')">
        <SelectValue :placeholder="triggerPlaceholder" />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          <SelectItem v-for="th in threads" :key="th.id" :value="th.id">
            <LoaderCircleIcon
              v-if="runningThreadIds?.has(th.id)"
              class="sb-spin size-3.5 shrink-0 text-primary"
              data-thread-running
              :aria-label="t('components.threadSwitcher.running')"
            />
            {{ th.title }}
            <span class="ml-1 text-muted-foreground">{{ relativeTime(th.updated_at) }}</span>
          </SelectItem>
        </SelectGroup>
      </SelectContent>
    </Select>
    <Button
      size="icon-sm"
      variant="outline"
      :disabled="busy"
      :title="t('components.threadSwitcher.newThread')"
      :aria-label="t('components.threadSwitcher.newThread')"
      @click="$emit('create')"
    >
      <PlusIcon />
    </Button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { LoaderCircleIcon, PlusIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { relativeTime } from '@/utils/relative-time'
import type { AgentThread } from '@/services/types'

interface Props {
  threads: AgentThread[]
  activeId: string
  busy?: boolean
  /** 会话历史尚未返回，且触发器还没有可显示的标题。 */
  pending?: boolean
  /** 正在回复的会话 id 集合。 */
  runningThreadIds?: Set<string>
}

const props = withDefaults(defineProps<Props>(), {
  busy: false,
  pending: false
})
defineEmits<{
  (e: 'create'): void
  (e: 'select', id: string): void
}>()

const { t } = useI18n()

const activeThread = computed(() => props.threads.find((t) => t.id === props.activeId))
const triggerPlaceholder = computed(() =>
  props.pending && !activeThread.value
    ? t('components.threadSwitcher.loadingThreads')
    : t('components.threadSwitcher.selectThread')
)
</script>
