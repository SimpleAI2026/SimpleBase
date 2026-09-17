<template>
  <div :class="cn('flex items-center gap-3 py-2', variant === 'separator' && 'text-xs text-muted-foreground')">
    <Separator v-if="variant === 'separator'" class="flex-1" />
    <span><slot /></span>
    <Separator v-if="variant === 'separator'" class="flex-1" />
  </div>
</template>
<script setup lang="ts">
import { Separator } from '@/components/ui/separator'
import { cn } from '@/lib/utils'

withDefaults(defineProps<{ variant?: 'default' | 'separator' }>(), { variant: 'default' })
</script>
