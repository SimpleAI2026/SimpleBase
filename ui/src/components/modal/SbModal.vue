<template>
  <Dialog :open="open" @update:open="emit('update:open', $event)">
    <DialogContent :class="contentClass" :show-close-button="true">
      <DialogHeader>
        <DialogTitle>{{ title }}</DialogTitle>
        <DialogDescription v-if="description" class="sr-only">{{ description }}</DialogDescription>
      </DialogHeader>
      <div class="min-w-0">
        <slot />
      </div>
      <DialogFooter>
        <slot name="footer">
          <Button variant="outline" @click="onCancel">{{ cancelText }}</Button>
          <Button :disabled="okDisabled || confirmLoading" @click="emit('ok')">
            <Spinner v-if="confirmLoading" data-icon="inline-start" />
            {{ okText }}
          </Button>
        </slot>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Spinner } from '@/components/ui/spinner'

const props = withDefaults(
  defineProps<{
    open: boolean
    title?: string
    description?: string
    width?: number | string
    confirmLoading?: boolean
    okText?: string
    cancelText?: string
    okButtonProps?: Record<string, unknown>
  }>(),
  {
    width: 520,
    confirmLoading: false,
    okText: '确定',
    cancelText: '取消',
  }
)

const emit = defineEmits<{
  'update:open': [value: boolean]
  ok: []
  cancel: []
}>()

const okDisabled = computed(() => Boolean(props.okButtonProps?.disabled))

const contentClass = computed(() => {
  const w = props.width
  if (typeof w === 'number') {
    if (w >= 900) return 'sm:max-w-5xl'
    if (w >= 800) return 'sm:max-w-4xl'
    if (w >= 700) return 'sm:max-w-3xl'
    if (w >= 600) return 'sm:max-w-2xl'
    if (w <= 480) return 'sm:max-w-md'
  }
  return 'sm:max-w-lg'
})

function onCancel() {
  emit('cancel')
  emit('update:open', false)
}
</script>
