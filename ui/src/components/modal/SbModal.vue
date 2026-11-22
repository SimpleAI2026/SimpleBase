<template>
  <Dialog :open="open" @update:open="emit('update:open', $event)">
    <DialogContent :class="contentClass" :style="contentStyle" :show-close-button="true">
      <DialogHeader class="shrink-0 pr-8">
        <DialogTitle>{{ title }}</DialogTitle>
        <!-- 始终渲染 Description：reka-ui 在缺失时会打 a11y 警告。
             没有描述时用标题兜底并视觉隐藏（仍为屏幕阅读器提供 aria-describedby）。 -->
        <DialogDescription :class="description ? undefined : 'sr-only'">{{ description || title }}</DialogDescription>
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
    /** 弹窗最大宽度（数字按 px 计）。 */
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
    maxWidth: 520,
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

/** 宽度只经 CSS 变量下发，不走 `sm:max-w-5xl` 之类的分档表。 */
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
  // flex 列 + 视口高度上限：覆盖 DialogContent 默认的 grid，短弹窗仍按内容高度居中
  const classes: string[] = ['flex max-h-[calc(100vh-2rem)] min-h-0 min-w-0 flex-col']
  if (props.maxWidth != null) {
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
