<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <div
    data-slot="card-footer"
    :class="cn('bg-muted/40 rounded-b-xl border-t border-border p-4 sm:px-5 group-data-[size=sm]/card:p-3 flex items-center', props.class)"
  >
    <slot />
  </div>
</template>
