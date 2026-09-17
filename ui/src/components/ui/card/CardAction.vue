<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <div
    data-slot="card-action"
    :class="cn('flex items-center gap-2 self-center shrink-0 ml-auto', props.class)"
  >
    <slot />
  </div>
</template>
