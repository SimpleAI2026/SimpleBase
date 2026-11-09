<template>
  <AlertDialog :open="controlled ? open : undefined" @update:open="onOpenChange">
    <!-- 触发器模式才需要 slot 作为触发元素；受控模式由外部控制开关 -->
    <AlertDialogTrigger v-if="!controlled && !disabled" as-child>
      <slot />
    </AlertDialogTrigger>
    <AlertDialogContent v-if="controlled ? true : !disabled">
      <AlertDialogHeader>
        <AlertDialogTitle>{{ title }}</AlertDialogTitle>
        <AlertDialogDescription>{{ resolvedDescription }}</AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel @click="emit('cancel')">{{ t('common.cancel') }}</AlertDialogCancel>
        <AlertDialogAction @click="onConfirm">{{ t('common.ok') }}</AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>
  <!-- disabled 且非受控：只渲染插槽内容，不提供确认交互 -->
  <slot v-if="!controlled && disabled" />
</template>

<script setup lang="ts">
/**
 * ConfirmAction：破坏性操作的统一确认入口。
 *
 * v3 优化把原先「受控 / 触发器」两份几乎逐字重复的 AlertDialog 标记合并为一份
 * （plan/planv5.0/ui-optimization-v3-plan.md §3.4）：两者只是开合状态来源不同，
 * 标题、描述、按钮与事件语义完全一致，重复标记会让每次文案或样式调整都要改两处。
 *
 * - 默认（触发器模式）：`<slot />` 作为触发元素，点击后弹出确认。
 * - `controlled`：由外部通过 `open` 控制开合，不再需要触发元素（如对话确认）。
 * - `disabled`：非受控下仅渲染插槽内容，不提供确认交互。
 */
import { computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'

const props = withDefaults(
  defineProps<{
    title: string
    description?: string
    disabled?: boolean
    /** 为 true 时改为受控弹窗，不再依赖触发器插槽。缺省保持原来的触发器模式。 */
    controlled?: boolean
    open?: boolean
  }>(),
  {}
)

const emit = defineEmits<{
  (e: 'confirm'): void
  (e: 'cancel'): void
  (e: 'update:open', value: boolean): void
}>()

const { t } = useI18n()

/** 未显式传 description 时的默认提示（走 i18n）。 */
const resolvedDescription = computed(() => props.description || t('common.irreversible'))

// 确认后再次打开时重置标记，允许下一次取消事件照常上报。
let confirmed = false

function onConfirm() {
  confirmed = true
  emit('confirm')
  if (props.controlled) emit('update:open', false)
}

function onOpenChange(value: boolean) {
  emit('update:open', value)
  if (!value && !confirmed) emit('cancel')
  if (value) confirmed = false
}

onMounted(() => {
  confirmed = false
})

watch(
  () => props.open,
  (value) => {
    if (value) confirmed = false
  }
)
</script>
