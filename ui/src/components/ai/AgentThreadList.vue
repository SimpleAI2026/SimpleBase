<template>
  <div class="flex min-w-0 flex-col gap-2">
    <div class="flex items-center justify-between gap-2">
      <span class="text-xs font-semibold text-muted-foreground">会话</span>
      <Button size="sm" variant="outline" :disabled="busy" @click="$emit('create')">新会话</Button>
    </div>
    <Select class="md:hidden" :model-value="activeId" @update:model-value="(id: string) => $emit('select', id)">
      <SelectTrigger aria-label="切换会话"><SelectValue placeholder="选择会话" /></SelectTrigger>
      <SelectContent><SelectGroup><SelectItem v-for="th in threads" :key="th.id" :value="th.id">{{ th.title }}</SelectItem></SelectGroup></SelectContent>
    </Select>
    <div class="hidden max-h-[480px] flex-col gap-1 overflow-y-auto md:flex">
      <div v-for="th in threads" :key="th.id" class="rounded-md border p-2" :class="activeId === th.id ? 'border-primary bg-primary/8' : 'border-border'">
        <div v-if="editingId === th.id" class="flex gap-1">
          <Input v-model="draft" :aria-label="`重命名 ${th.title}`" maxlength="80" @keydown.enter="save(th.id)" @keydown.esc="editingId = ''" />
          <Button size="sm" variant="outline" @click="save(th.id)">保存</Button>
        </div>
        <button v-else type="button" class="w-full min-w-0 text-left" :aria-pressed="activeId === th.id" @click="$emit('select', th.id)">
          <span class="block truncate text-sm">{{ th.title }}</span>
          <span v-if="th.last_message_preview" class="block truncate text-xs text-muted-foreground">{{ th.last_message_preview }}</span>
          <span class="text-xs text-muted-foreground">{{ relativeTime(th.updated_at) }}</span>
        </button>
        <div class="flex justify-end gap-1">
          <Button size="xs" variant="ghost" :aria-label="`重命名会话 ${th.title}`" @click="editingId = th.id; draft = th.title">重命名</Button>
          <ConfirmAction title="确认删除会话？" @confirm="$emit('remove', th.id)">
            <Button size="xs" variant="destructiveGhost" :aria-label="`删除会话 ${th.title}`">删除</Button>
          </ConfirmAction>
        </div>
      </div>
    </div>
    <Button v-if="nextCursor" size="sm" variant="ghost" class="hidden md:inline-flex" @click="$emit('more')">加载更多</Button>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import ConfirmAction from '../ConfirmAction.vue'
import type { AgentThread } from '@/services/types'

defineProps<{ threads: AgentThread[]; activeId: string; nextCursor?: string; busy?: boolean }>()
const emit = defineEmits<{
  (e: 'create'): void
  (e: 'select', id: string): void
  (e: 'rename', id: string, title: string): void
  (e: 'remove', id: string): void
  (e: 'more'): void
}>()
const editingId = ref('')
const draft = ref('')
function save(id: string) {
  const title = draft.value.trim()
  if (!title || title.length > 80) return
  emit('rename', id, title)
  editingId.value = ''
}
function relativeTime(value: string) {
  const elapsed = Date.now() - new Date(value).getTime()
  if (!Number.isFinite(elapsed)) return ''
  if (elapsed < 60_000) return '刚刚'
  if (elapsed < 3_600_000) return `${Math.floor(elapsed / 60_000)} 分钟前`
  if (elapsed < 86_400_000) return `${Math.floor(elapsed / 3_600_000)} 小时前`
  return `${Math.floor(elapsed / 86_400_000)} 天前`
}
</script>
