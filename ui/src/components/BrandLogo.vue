<template>
  <span class="inline-flex min-w-0 items-center gap-2.5 text-foreground">
    <span
      aria-hidden="true"
      :class="[
        'flex shrink-0 items-center justify-center bg-primary font-medium text-primary-foreground',
        size === 'lg' ? 'size-9 rounded-lg text-lg' : 'size-8 rounded-lg text-base'
      ]"
    >S</span>
    <span
      v-if="!markOnly"
      :class="['truncate font-medium', size === 'lg' ? 'text-lg' : 'text-base', wordmarkClass]"
    >SimpleBase</span>
  </span>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'

interface BrandLogoProps {
  size?: 'md' | 'lg'
  markOnly?: boolean
  wordmarkClass?: HTMLAttributes['class']
}

withDefaults(defineProps<BrandLogoProps>(), {
  size: 'md',
  markOnly: false,
  wordmarkClass: undefined
})
</script>
