<template>
  <div
    :class="cn(
      'rounded-lg border px-3 py-2',
      variant === 'muted' && 'bg-muted border-border',
      variant === 'default' && 'bg-primary/12 border-transparent',
    )"
  >
    <slot />
  </div>
</template>
<script setup lang="ts">
import { cn } from '@/lib/utils'

withDefaults(defineProps<{ variant?: 'default' | 'muted'; align?: 'start' | 'end' }>(), {
  variant: 'muted',
  align: 'start',
})
</script>
