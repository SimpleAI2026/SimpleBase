<template>
  <div :class="cn('flex items-start gap-2', align === 'end' && 'flex-row-reverse')">
    <slot />
  </div>
</template>
<script setup lang="ts">
import { cn } from '@/lib/utils'

withDefaults(defineProps<{ align?: 'start' | 'end' }>(), { align: 'start' })
</script>
