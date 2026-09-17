<template>
  <div
    :class="cn(
      'flex size-7 shrink-0 items-center justify-center rounded-full border bg-muted text-muted-foreground',
      align === 'end' && 'border-primary text-primary',
    )"
  >
    <slot />
  </div>
</template>
<script setup lang="ts">
import { cn } from '@/lib/utils'

withDefaults(defineProps<{ align?: 'start' | 'end' }>(), { align: 'start' })
</script>
