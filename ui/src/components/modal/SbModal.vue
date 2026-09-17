<template>
  <a-modal
    :open="open"
    :title="title"
    :width="width"
    :confirm-loading="confirmLoading"
    :destroy-on-close="destroyOnClose"
    :ok-text="okText"
    :cancel-text="cancelText"
    :ok-button-props="okButtonProps"
    :mask-closable="maskClosable"
    :centered="centered"
    :z-index="zIndex"
    wrap-class-name="sb-modal"
    @ok="emit('ok')"
    @cancel="onCancel"
    @update:open="(v: boolean) => emit('update:open', v)"
  >
    <slot />
    <template v-if="$slots.footer" #footer>
      <slot name="footer" />
    </template>
  </a-modal>
</template>

<script setup lang="ts">
/**
 * 统一弹窗：固定默认宽度、footer、destroyOnClose，业务弹窗都包这一层。
 */
withDefaults(
  defineProps<{
    open: boolean
    title?: string
    width?: number | string
    confirmLoading?: boolean
    destroyOnClose?: boolean
    okText?: string
    cancelText?: string
    okButtonProps?: Record<string, unknown>
    maskClosable?: boolean
    centered?: boolean
    zIndex?: number
  }>(),
  {
    width: 520,
    confirmLoading: false,
    destroyOnClose: true,
    okText: '确定',
    cancelText: '取消',
    maskClosable: true,
    centered: true
  }
)

const emit = defineEmits<{
  'update:open': [value: boolean]
  ok: []
  cancel: []
}>()

function onCancel() {
  emit('cancel')
  emit('update:open', false)
}
</script>
