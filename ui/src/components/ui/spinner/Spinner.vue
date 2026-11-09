<script setup lang="ts">
import type { HTMLAttributes } from 'vue'

import { Loader2Icon } from '@lucide/vue'
import { useI18n } from 'vue-i18n'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()

// 无障碍标签会随界面语言变化（plan/planv5.0/ui-optimization-v3-plan.md §3.2）：
// 这是文案而非样式约定，故在 fork 层也接 i18n。
const { t } = useI18n()
</script>

<template>
  <Loader2Icon role="status" :aria-label="t('common.loading')" :class="cn('size-4 animate-spin', props.class)" />
</template>
