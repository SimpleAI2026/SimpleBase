<template>
  <div
    data-slot="sb-error-state"
    :class="
      variant === 'block'
        ? 'flex flex-col items-start gap-3 py-6'
        : 'flex w-full max-w-lg flex-col items-center gap-3 text-left'
    "
  >
    <Alert variant="destructive" :class="variant === 'block' ? 'w-full' : undefined">
      <AlertTitle>{{ title || t('error.loadFailed') }}</AlertTitle>
      <AlertDescription>{{ description || t('error.loadFailed') }}</AlertDescription>
    </Alert>
    <Button size="sm" variant="outline" @click="emit('retry')">{{ retryText || t('common.retry') }}</Button>
  </div>
</template>

<script setup lang="ts">
/**
 * SbErrorState：统一报错框（plan/planv5.0/ui-optimization-v3-plan.md §3.4）。
 *
 * 此前 `SbAsyncRegion` 在 `as='tbody'` 与 `as='div'` 两个分支里各写了一遍
 * 「Alert + 标题 + 重试按钮」，任何文案或样式调整都要改两处。这里收敛成唯一实现。
 *
 * - variant='inline'：嵌在表格 `tbody` 的 `TableEmpty` 里，居中且限宽。
 * - variant='block'：普通区域，左对齐占满宽度。
 *
 * 语义约定沿用 plan/planv5.0/console-loading-states-plan.md：
 * 不使用 `aria-live`（避免骨架与错误反复朗读），由外层区域维护 `aria-busy`。
 */
import { useI18n } from 'vue-i18n'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import type { HTMLAttributes } from 'vue'

export interface SbErrorStateProps {
  /** 已翻译的标题；缺省用 error.loadFailed */
  title?: string
  /** 已翻译的详情；缺省用 error.loadFailed */
  description?: string
  /** 已翻译的重试按钮文案；缺省用 common.retry */
  retryText?: string
  /** inline：嵌在表格 tbody 里；block：普通区域 */
  variant?: 'inline' | 'block'
  class?: HTMLAttributes['class']
}

withDefaults(defineProps<SbErrorStateProps>(), {
  variant: 'block'
})

const emit = defineEmits<{ retry: [] }>()
const { t } = useI18n()
</script>
