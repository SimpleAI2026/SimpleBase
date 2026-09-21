<template>
  <Dialog :open="open" @update:open="emit('update:open', $event)">
    <DialogContent :class="contentClass" :style="contentStyle" :show-close-button="true">
      <DialogHeader>
        <DialogTitle>{{ title }}</DialogTitle>
        <DialogDescription v-if="description" class="sr-only">{{ description }}</DialogDescription>
      </DialogHeader>
      <div class="min-w-0">
        <slot />
      </div>
      <DialogFooter v-if="!hideFooter">
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
    maxWidth?: number | string
    minWidth?: number | string
    hideFooter?: boolean
    confirmLoading?: boolean
    okText?: string
    cancelText?: string
    okButtonProps?: Record<string, unknown>
  }>(),
  {
    width: 520,
    hideFooter: false,
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

function cssSize(value: number | string): string {
  return typeof value === 'number' ? `${value}px` : value
}

const contentStyle = computed(() => {
  const style: Record<string, string> = {}
  if (props.maxWidth != null) {
    style['--sb-modal-max-w'] = cssSize(props.maxWidth)
  }
  if (props.minWidth != null) {
    style['--sb-modal-min-w'] = cssSize(props.minWidth)
  }
  return style
})

const contentClass = computed(() => {
  const classes: string[] = []
  if (props.maxWidth != null) {
    classes.push('sm:max-w-[var(--sb-modal-max-w)]')
  } else {
    const w = props.width
    if (typeof w === 'number') {
      if (w >= 900) classes.push('sm:max-w-5xl')
      else if (w >= 800) classes.push('sm:max-w-4xl')
      else if (w >= 700) classes.push('sm:max-w-3xl')
      else if (w >= 600) classes.push('sm:max-w-2xl')
      else if (w <= 480) classes.push('sm:max-w-md')
      else classes.push('sm:max-w-lg')
    } else {
      classes.push('sm:max-w-lg')
    }
  }
  if (props.minWidth != null) {
    classes.push('sm:min-w-[var(--sb-modal-min-w)]')
  }
  return classes.join(' ')
})

function onCancel() {
  emit('cancel')
  emit('update:open', false)
}
</script>
