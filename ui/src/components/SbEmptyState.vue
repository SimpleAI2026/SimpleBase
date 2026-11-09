<template>
  <Empty class="py-5">
    <EmptyHeader>
      <EmptyMedia variant="icon">
        <component :is="icon || InboxIcon" />
      </EmptyMedia>
      <EmptyTitle>{{ resolvedTitle }}</EmptyTitle>
      <EmptyDescription>{{ resolvedDescription }}</EmptyDescription>
    </EmptyHeader>
    <EmptyContent v-if="actionText">
      <Button @click="$emit('action')">
        <component :is="actionIcon || PlusIcon" data-icon="inline-start" />
        {{ actionText }}
      </Button>
    </EmptyContent>
  </Empty>
</template>
<script setup lang="ts">
import type { Component } from 'vue'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
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

const props = withDefaults(
  defineProps<{
    title?: string
    description?: string
    actionText?: string
    icon?: Component
    /**
     * 操作按钮的图标。默认 PlusIcon 只适合「新建」语义；
     * 「重试 / 刷新 / 查看」一类必须显式传入，否则图标与动作不符。
     */
    actionIcon?: Component
  }>(),
  {}
)

defineEmits<{ (e: 'action'): void }>()

const { t } = useI18n()

const resolvedTitle = computed(() => props.title || t('common.emptyTitle'))
const resolvedDescription = computed(() => props.description || t('common.emptyDescription'))
</script>
