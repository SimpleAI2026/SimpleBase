<template>
  <div class="flex flex-wrap items-center gap-2" role="group" :aria-label="t('pages.dataBrowser.chartType')">
    <TooltipProvider :delay-duration="150">
      <Tooltip v-for="item in items" :key="item.type">
        <TooltipTrigger as-child>
          <!-- 置灰按钮不能直接作为 tooltip 触发器（disabled 元素不触发指针事件），包一层 span -->
          <span class="inline-flex">
            <Button
              type="button"
              size="sm"
              :variant="item.selected ? 'default' : 'outline'"
              :disabled="Boolean(item.reason)"
              :data-type="item.type"
              :data-state="item.selected ? 'on' : 'off'"
              :aria-pressed="item.selected"
              class="h-8 px-3 text-xs"
              @click="emit('update:modelValue', item.type)"
            >
              {{ item.label }}
            </Button>
          </span>
        </TooltipTrigger>
        <TooltipContent v-if="item.reason">{{ item.reasonText }}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
    <span v-if="inferredLabel" class="text-xs text-muted-foreground" data-testid="inferred">{{ inferredLabel }}</span>
  </div>
</template>

<script setup lang="ts">
// BiChartTypePicker：图表类型选择器（planv6.7 §4.3，F4 / F4a / F4b）。
// 固定 9 项，顺序与产品原型一致；不兼容当前字段配置的类型置灰并给出原因。
// 兼容判定来自 bi-compat（与编辑器、Agent 结果校验同一份口径）。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import type { BiChartType, BiWidget, CollectionField } from '@/services/types'
import { ALL_TYPES, checkType, fieldMap } from '@/utils/bi-compat'
import type { ResolvedType } from '@/utils/bi-auto'

const props = withDefaults(defineProps<{
  modelValue: BiChartType
  /** 当前正在配置的 widget；无 widget 时（页面级选择器）所有类型都可选。 */
  widget?: BiWidget | null
  fields?: CollectionField[]
  /** 「自动」当前解析出的类型，用于显示「已推断为：xx」。 */
  inferred?: ResolvedType | null
  /**
   * 编辑器里的选择器：类型切换本身就是配置的入口，不能被「当前还没配好」卡死。
   * - 选「散点图」才会出现 X / Y 输入，若按「还没配 X / Y」置灰，永远切不过去；
   * - 当前已是散点时，切走（auto / 表格 / 其他）也必须可用：切换时编辑器会清掉散点配置并补回默认度量。
   * 开启后这两种过渡不按配置置灰，真正的合法性校验留到保存阶段。
   */
  allowScatterSetup?: boolean
  /** 数据源是否有内置 created_at / updated_at；缺省有（集合）。 */
  builtinTime?: boolean
}>(), { builtinTime: true })

const emit = defineEmits<{ 'update:modelValue': [type: BiChartType] }>()
const { t } = useI18n()

const fieldIndex = computed(() => fieldMap(props.fields ?? [], props.builtinTime))

const items = computed(() =>
  ALL_TYPES.map((type) => {
    const transition = props.allowScatterSetup && (type === 'scatter' || props.modelValue === 'scatter')
    const skip = !props.widget || transition
    const checked = skip ? ({ ok: true } as const) : checkType(type, props.widget!, fieldIndex.value)
    const reason = checked.ok ? '' : checked.reason
    return {
      type,
      label: t(`components.bi.types.${type}`),
      selected: props.modelValue === type,
      reason,
      reasonText: reason ? t(`components.bi.reason.${reason}`) : ''
    }
  })
)

const inferredLabel = computed(() =>
  props.modelValue === 'auto' && props.inferred
    ? t('components.bi.inferredAs', { type: t(`components.bi.types.${props.inferred}`) })
    : ''
)
</script>
