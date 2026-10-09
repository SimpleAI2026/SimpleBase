<script setup lang="ts">
import { computed } from 'vue'
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { TableCell, TableEmpty, TableRow } from '@/components/ui/table'
import SbBlockSkeleton from './SbBlockSkeleton.vue'
import SbEmptyState from './SbEmptyState.vue'
import SbTableSkeleton from './SbTableSkeleton.vue'

interface Props {
  /** `tbody` 时骨架和空行留在表格里，避免把表头换掉。 */
  as?: 'div' | 'tbody'
  pending?: boolean
  showSkeleton?: boolean
  showEmpty?: boolean
  showError?: boolean
  refreshing?: boolean
  error?: string
  columns?: number
  rows?: number
  block?: 'lines' | 'cards' | 'chart' | 'spinner'
  emptyTitle?: string
  emptyDescription?: string
  emptyActionText?: string
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), {
  as: 'div',
  pending: false,
  showSkeleton: false,
  showEmpty: false,
  showError: false,
  refreshing: false,
  error: '',
  columns: 1,
  rows: 6,
  block: 'lines'
})

const emit = defineEmits<{
  retry: []
  'empty-action': []
}>()

const reserving = computed(
  () =>
    props.pending &&
    !props.refreshing &&
    !props.showSkeleton &&
    !props.showEmpty &&
    !props.showError
)
const showPlaceholder = computed(() => props.showSkeleton || reserving.value)
const busy = computed(() => props.pending || props.showSkeleton || props.refreshing)
const rootClass = computed(() => cn(props.refreshing && 'opacity-80', props.class))
</script>

<template>
  <component
    :is="as"
    :data-slot="as === 'tbody' ? 'table-body' : 'async-region'"
    :aria-busy="busy ? 'true' : 'false'"
    :class="rootClass"
  >
    <tr v-if="as === 'tbody' && busy" class="sr-only">
      <td>正在加载</td>
    </tr>
    <p v-else-if="busy" class="sr-only">正在加载</p>

    <template v-if="showError">
      <TableEmpty v-if="as === 'tbody'" :colspan="columns">
        <div class="flex w-full max-w-lg flex-col items-center gap-3 text-left">
          <Alert variant="destructive">
            <AlertTitle>加载失败</AlertTitle>
            <AlertDescription>{{ error || '加载失败' }}</AlertDescription>
          </Alert>
          <Button size="sm" variant="outline" @click="emit('retry')">重试</Button>
        </div>
      </TableEmpty>
      <div v-else class="flex flex-col items-start gap-3 py-6">
        <Alert variant="destructive" class="w-full">
          <AlertTitle>加载失败</AlertTitle>
          <AlertDescription>{{ error || '加载失败' }}</AlertDescription>
        </Alert>
        <Button size="sm" variant="outline" @click="emit('retry')">重试</Button>
      </div>
    </template>

    <template v-else-if="showPlaceholder">
      <template v-if="as === 'tbody'">
        <slot name="skeleton" :ghost="reserving && !showSkeleton">
          <SbTableSkeleton :columns="columns" :rows="rows" :ghost="reserving && !showSkeleton" />
        </slot>
      </template>
      <div v-else aria-hidden="true" :class="reserving && !showSkeleton ? 'invisible' : ''">
        <slot name="skeleton">
          <SbBlockSkeleton :variant="block" />
        </slot>
      </div>
    </template>

    <template v-else-if="showEmpty">
      <TableEmpty v-if="as === 'tbody'" :colspan="columns">
        <slot name="empty">
          <SbEmptyState
            :title="emptyTitle"
            :description="emptyDescription"
            :action-text="emptyActionText"
            @action="emit('empty-action')"
          />
        </slot>
      </TableEmpty>
      <slot v-else name="empty">
        <SbEmptyState
          :title="emptyTitle"
          :description="emptyDescription"
          :action-text="emptyActionText"
          @action="emit('empty-action')"
        />
      </slot>
    </template>

    <slot v-else />
  </component>
</template>
