<template>
  <!-- ThreadSwitcher：对话区头部的会话下拉切换器（planv4.1 §3.2：会话列表移入头部）。 -->
  <div class="flex min-w-0 flex-1 items-center gap-2">
    <Select :model-value="activeId" @update:model-value="(id: string) => $emit('select', id)">
      <SelectTrigger class="h-8 min-w-0 flex-1" aria-label="切换会话">
        <SelectValue placeholder="选择会话" />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          <SelectItem v-for="th in threads" :key="th.id" :value="th.id">
            {{ th.title }}
            <span class="ml-1 text-muted-foreground">{{ relativeTime(th.updated_at) }}</span>
          </SelectItem>
        </SelectGroup>
      </SelectContent>
    </Select>
    <Button size="sm" variant="outline" :disabled="busy" @click="$emit('create')">
      <PlusIcon data-icon="inline-start" />
      新会话
    </Button>
    <template v-if="activeThread">
      <ConfirmAction title="确认删除当前会话？" @confirm="$emit('remove', activeThread.id)">
        <Button size="sm" variant="ghost" :aria-label="`删除会话 ${activeThread.title}`">删除</Button>
      </ConfirmAction>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { PlusIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import ConfirmAction from '../ConfirmAction.vue'
import { relativeTime } from '@/utils/relative-time'
import type { AgentThread } from '@/services/types'

const props = defineProps<{ threads: AgentThread[]; activeId: string; busy?: boolean }>()
defineEmits<{
  (e: 'create'): void
  (e: 'select', id: string): void
  (e: 'remove', id: string): void
}>()

const activeThread = computed(() => props.threads.find((t) => t.id === props.activeId))
</script>
