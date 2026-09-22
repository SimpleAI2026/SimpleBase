<template>
  <Empty class="py-5">
    <EmptyHeader>
      <EmptyMedia variant="icon">
        <component :is="icon || InboxIcon" />
      </EmptyMedia>
      <EmptyTitle>{{ title }}</EmptyTitle>
      <EmptyDescription>{{ description }}</EmptyDescription>
    </EmptyHeader>
    <EmptyContent v-if="actionText">
      <Button @click="$emit('action')">
        <PlusIcon data-icon="inline-start" />
        {{ actionText }}
      </Button>
    </EmptyContent>
  </Empty>
</template>
<script setup lang="ts">
import type { Component } from 'vue'
import { InboxIcon, PlusIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'

withDefaults(
  defineProps<{
    title?: string
    description?: string
    actionText?: string
    icon?: Component
  }>(),
  { title: '暂无数据', description: '没有可展示的内容' }
)

defineEmits<{ (e: 'action'): void }>()
</script>
