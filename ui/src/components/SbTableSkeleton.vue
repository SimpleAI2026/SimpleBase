<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'
import { TableCell, TableRow } from '@/components/ui/table'

interface Props {
  columns: number
  rows?: number
  /** 安静等待：占住行高，但不脉冲。 */
  ghost?: boolean
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), {
  rows: 6,
  ghost: false
})

const widths = ['w-[92%]', 'w-[64%]', 'w-[40%]']

function widthAt(index: number) {
  return widths[index % widths.length]
}
</script>

<template>
  <TableRow
    v-for="row in rows"
    :key="row"
    aria-hidden="true"
    :class="cn('h-10', ghost && 'invisible', props.class)"
  >
    <TableCell v-for="col in columns" :key="col" class="py-2">
      <Skeleton class="h-4" :class="widthAt(col - 1)" />
    </TableCell>
  </TableRow>
</template>
