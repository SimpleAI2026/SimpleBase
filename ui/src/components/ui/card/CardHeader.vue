<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <div
    data-slot="card-header"
    :class="cn('flex flex-col gap-1.5 p-4 group-data-[size=sm]/card:p-4 sm:p-5 [&.border-b]:border-border [&.border-b]:pb-4 group/card-header has-data-[slot=card-action]:flex-row has-data-[slot=card-action]:items-center has-data-[slot=card-action]:justify-between max-sm:has-data-[slot=card-action]:flex-col max-sm:has-data-[slot=card-action]:items-stretch', props.class)"
  >
    <slot />
  </div>
</template>
