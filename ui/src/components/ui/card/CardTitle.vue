<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <div
    data-slot="card-title"
    :class="cn('text-base font-semibold leading-tight tracking-tight text-foreground group-data-[size=sm]/card:text-sm font-sans', props.class)"
  >
    <slot />
  </div>
</template>
