<template>
  <Dialog :open="open" @update:open="emit('update:open', $event)">
    <DialogContent
      :class="contentClass"
      :style="contentStyle"
      :data-max-width="cssSize(maxWidth) || undefined"
      :data-min-width="cssSize(minWidth) || undefined"
      :show-close-button="true"
    >
      <DialogHeader>
        <DialogTitle>{{ title }}</DialogTitle>
        <DialogDescription v-if="description" class="sr-only">{{ description }}</DialogDescription>
      </DialogHeader>
      <div :class="bodyClass || 'min-w-0'">
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
import { computed, type StyleValue } from 'vue'
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
    /** Exact max-width (px if number). Overrides the `width` breakpoint mapping. */
    maxWidth?: number | string
    /** Exact min-width (px if number). */
    minWidth?: number | string
    hideFooter?: boolean
    bodyClass?: string
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

function cssSize(v: number | string | undefined): string | undefined {
  if (v == null || v === '') return undefined
  if (typeof v === 'number') return `${v}px`
  if (/^\d+(\.\d+)?$/.test(v.trim())) return `${v.trim()}px`
  return v
}

const contentStyle = computed<StyleValue>(() => {
  const style: Record<string, string> = {}
  const maxW = cssSize(props.maxWidth)
  const minW = cssSize(props.minWidth)
  if (maxW) style['--sb-modal-max-w'] = maxW
  if (minW) style['--sb-modal-min-w'] = minW
  return style
})

const contentClass = computed(() => {
  const classes: string[] = []
  if (props.maxWidth != null && props.maxWidth !== '') {
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
  if (props.minWidth != null && props.minWidth !== '') {
    classes.push('sm:min-w-[var(--sb-modal-min-w)]')
  }
  return classes.join(' ')
})

function onCancel() {
  emit('cancel')
  emit('update:open', false)
}
</script>
