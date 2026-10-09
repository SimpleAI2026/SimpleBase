<script setup lang="ts">
import type { HTMLAttributes } from 'vue'

import { Loader2Icon } from '@lucide/vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <Loader2Icon role="status" aria-label="正在加载" :class="cn('size-4 animate-spin', props.class)" />
</template>
