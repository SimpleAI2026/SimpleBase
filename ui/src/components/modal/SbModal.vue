<template>
  <Dialog :open="open" @update:open="emit('update:open', $event)">
    <DialogContent :class="contentClass" :style="contentStyle" :show-close-button="true">
      <DialogHeader class="shrink-0 pr-8">
        <DialogTitle>{{ title }}</DialogTitle>
        <DialogDescription v-if="description">{{ description }}</DialogDescription>
      </DialogHeader>
      <!-- 标题和底栏留在视口内，长内容只在 body 里滚动；px-1 -mx-1 给 overflow 裁切下的焦点环让出空间 -->
      <div data-slot="sb-modal-body" class="-mx-1 min-h-0 min-w-0 max-w-full overflow-x-hidden overflow-y-auto px-1">
        <slot />
      </div>
      <DialogFooter v-if="!hideFooter" class="shrink-0">
        <slot name="footer">
          <Button variant="outline" @click="onCancel">{{ cancelText || t('common.cancel') }}</Button>
          <Button v-bind="okButtonProps" :disabled="okDisabled" @click="emit('ok')">
            <Spinner v-if="confirmLoading" data-icon="inline-start" />
            {{ okText || t('common.ok') }}
          </Button>
        </slot>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button, type ButtonVariants } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Spinner } from '@/components/ui/spinner'

/** 透传给确认按钮的 Button props（variant / size / class / disabled 等全部生效）。 */
export interface SbModalOkButtonProps {
  variant?: ButtonVariants['variant']
  size?: ButtonVariants['size']
  class?: string
  disabled?: boolean
}

const props = withDefaults(
  defineProps<{
    open: boolean
    title?: string
    description?: string
    /** 弹窗最大宽度（数字按 px 计）。与 maxWidth 等价，保留为向后兼容别名。 */
    width?: number | string
    maxWidth?: number | string
    minWidth?: number | string
    hideFooter?: boolean
    confirmLoading?: boolean
    okText?: string
    cancelText?: string
    /** 直接透传给确认按钮的 Button props */
    okButtonProps?: SbModalOkButtonProps
  }>(),
  {
    width: 520,
    hideFooter: false,
    confirmLoading: false
  }
)

const emit = defineEmits<{
  'update:open': [value: boolean]
  ok: []
  cancel: []
}>()

const { t } = useI18n()

const okDisabled = computed(
  () => Boolean(props.okButtonProps?.disabled) || props.confirmLoading
)

function cssSize(value: number | string): string {
  return typeof value === 'number' ? `${value}px` : value
}

/**
 * 宽度只有一条通道：CSS 变量 `--sb-modal-max-w` / `--sb-modal-min-w`。
 * `width` 是历史别名，换算成同样的变量，不再走 `sm:max-w-5xl` 之类的分档表
 * （两套机制并存时改一处不生效，见 planv5.0 ui-optimization-v3-plan.md §3.4）。
 */
const resolvedMaxWidth = computed(() => props.maxWidth ?? props.width)

const contentStyle = computed(() => {
  const style: Record<string, string> = {}
  if (resolvedMaxWidth.value != null) {
    style['--sb-modal-max-w'] = cssSize(resolvedMaxWidth.value)
  }
  if (props.minWidth != null) {
    style['--sb-modal-min-w'] = cssSize(props.minWidth)
  }
  return style
})

const contentClass = computed(() => {
  // flex 列 + 视口高度上限：覆盖 DialogContent 默认的 grid，短弹窗仍按内容高度居中
  const classes: string[] = ['flex max-h-[calc(100vh-2rem)] min-h-0 min-w-0 flex-col']
  if (resolvedMaxWidth.value != null) {
    classes.push('sm:max-w-[var(--sb-modal-max-w)]')
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
