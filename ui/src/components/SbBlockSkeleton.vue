<script setup lang="ts">
import { computed } from 'vue'
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'

interface Props {
  variant?: 'lines' | 'cards' | 'chart' | 'spinner'
  count?: number
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'lines',
  count: 0
})

const lineWidths = ['w-full', 'w-[92%]', 'w-4/5', 'w-2/3', 'w-1/2']

const lines = computed(() => lineWidths.slice(0, props.count > 0 ? props.count : lineWidths.length))
const cards = computed(() => Array.from({ length: props.count > 0 ? props.count : 3 }, (_, i) => i))
</script>

<template>
  <div
    aria-hidden="true"
    :class="
      cn(
        props.variant === 'spinner' && 'flex min-h-28 items-center justify-center',
        props.variant === 'cards' && 'grid grid-cols-1 gap-3',
        props.variant === 'lines' && 'flex flex-col gap-3 py-2',
        props.class
      )
    "
  >
    <template v-if="variant === 'lines'">
      <Skeleton v-for="(width, index) in lines" :key="index" class="h-4" :class="width" />
    </template>
    <template v-else-if="variant === 'cards'">
      <Skeleton v-for="index in cards" :key="index" class="h-24 w-full rounded-xl" />
    </template>
    <Skeleton v-else-if="variant === 'chart'" class="h-44 w-full rounded-xl" />
    <Spinner v-else />
  </div>
</template>
