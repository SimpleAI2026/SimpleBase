<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <span
    data-slot="command-shortcut"
    :class="cn('text-muted-foreground group-data-highlighted/command-item:text-foreground ml-auto font-mono text-xs', props.class)"
  >
    <slot />
  </span>
</template>
