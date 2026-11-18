<template>
  <Card>
    <CardHeader
      class="flex-wrap gap-3 border-b max-sm:[&_[data-slot=card-action]]:w-full max-sm:[&_[data-slot=card-action]]:justify-start"
    >
      <CardTitle>{{ title }}</CardTitle>
      <CardDescription v-if="description || $slots.description">
        <slot name="description">{{ description }}</slot>
      </CardDescription>
      <CardAction v-if="$slots.actions">
        <div class="flex items-center gap-2">
          <slot name="actions" />
        </div>
      </CardAction>
    </CardHeader>
    <CardContent v-if="$slots.toolbar" class="flex flex-wrap items-end gap-2.5 border-b py-4">
      <slot name="toolbar" />
    </CardContent>
    <div>
      <slot />
      <TablePager
        v-if="pager"
        variant="footer"
        v-bind="pager"
        @update:page="emit('update:page', $event)"
      />
    </div>
  </Card>
</template>

<script setup lang="ts">
/**
 * 列表页统一卡片骨架（planv6.7 ui-simplify-ux-plan S5）。
 * 收编三件样板：卡片头窄屏换行 class、筛选工具条 CardContent、TablePager footer。
 * actions / toolbar / footer 缺省不渲染；pager 传入后内嵌分页器。
 */
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import TablePager from './TablePager.vue'

defineProps<{
  title: string
  /** 卡片副标题；需要富文本（含 code）时用 description 插槽 */
  description?: string
  /** 内嵌 TablePager 的 props（page / pageSize / total / pageCount） */
  pager?: {
    page: number
    pageSize: number
    total: number
    pageCount: number
  }
}>()

const emit = defineEmits<{ 'update:page': [value: number] }>()
</script>
