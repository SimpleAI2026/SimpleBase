<script setup lang="ts">
import { computed, unref } from 'vue'
import type { MaybeRef } from 'vue'
import { useI18n } from 'vue-i18n'
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import { TableEmpty } from '@/components/ui/table'
import SbBlockSkeleton from '@/components/SbBlockSkeleton.vue'
import SbEmptyState from '@/components/SbEmptyState.vue'
import SbErrorState from '@/components/SbErrorState.vue'
import SbTableSkeleton from '@/components/SbTableSkeleton.vue'
import type { AsyncRegionState } from '@/components/async-region-state'

interface Props {
  /** `tbody` 时骨架和空行留在表格里，避免把表头换掉。 */
  as?: 'div' | 'tbody'
  /** 直接传 `useLoadState()` 的结果；传了就忽略下面散装的 pending / show* / refreshing / error。 */
  state?: AsyncRegionState
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

const { t } = useI18n()

/** 六个状态的唯一读取入口：有 state 以它为准，否则用散装 props。 */
const view = computed(() => {
  const s = props.state
  return s
    ? {
        pending: unref(s.pending),
        showSkeleton: unref(s.showSkeleton),
        showEmpty: unref(s.showEmpty),
        showError: unref(s.showError),
        refreshing: unref(s.refreshing),
        error: unref(s.error)
      }
    : {
        pending: props.pending,
        showSkeleton: props.showSkeleton,
        showEmpty: props.showEmpty,
        showError: props.showError,
        refreshing: props.refreshing,
        error: props.error
      }
})

const reserving = computed(
  () =>
    view.value.pending &&
    !view.value.refreshing &&
    !view.value.showSkeleton &&
    !view.value.showEmpty &&
    !view.value.showError
)
const showPlaceholder = computed(() => view.value.showSkeleton || reserving.value)
const busy = computed(() => view.value.pending || view.value.showSkeleton || view.value.refreshing)
const rootClass = computed(() => cn(view.value.refreshing && 'opacity-80', props.class))
</script>

<template>
  <component
    :is="as"
    :data-slot="as === 'tbody' ? 'table-body' : 'async-region'"
    :aria-busy="busy ? 'true' : 'false'"
    :class="rootClass"
  >
    <tr v-if="as === 'tbody' && busy" class="sr-only">
      <td>{{ t('common.loading') }}</td>
    </tr>
    <p v-else-if="busy" class="sr-only">{{ t('common.loading') }}</p>

    <!-- 错误态唯一实现：SbErrorState -->
    <template v-if="view.showError">
      <TableEmpty v-if="as === 'tbody'" :colspan="columns">
        <SbErrorState variant="inline" :description="view.error" @retry="emit('retry')" />
      </TableEmpty>
      <SbErrorState v-else variant="block" :description="view.error" @retry="emit('retry')" />
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

    <template v-else-if="view.showEmpty">
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
