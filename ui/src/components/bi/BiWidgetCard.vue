<template>
  <Card :class="preview ? 'border-dashed border-primary/50' : ''" :data-widget="widget.id" :data-mode="preview ? 'preview' : 'board'">
    <CardContent class="flex flex-col gap-3 p-4 sm:p-5">
      <div class="flex items-start justify-between gap-2">
        <div class="min-w-0">
          <h3 class="truncate text-sm font-medium text-foreground" :title="widget.title">{{ widget.title }}</h3>
          <p v-if="meta" class="mt-0.5 truncate text-xs text-muted-foreground">{{ meta }}</p>
        </div>

        <!-- 预览模式：加入看板 / 调整 / 丢弃 -->
        <div v-if="preview" class="flex shrink-0 items-center gap-1.5">
          <Button size="sm" :disabled="Boolean(error) || pending" data-action="add" @click="emit('add')">
            <PlusIcon data-icon="inline-start" />{{ t('components.bi.agent.addToDashboard') }}
          </Button>
          <Button size="sm" variant="outline" data-action="adjust" @click="emit('edit')">{{ t('components.bi.agent.adjust') }}</Button>
          <Button size="sm" variant="ghost" data-action="discard" @click="emit('discard')">{{ t('components.bi.agent.discard') }}</Button>
        </div>

        <!-- 看板模式：更多菜单（Popover 内放按钮列表，不引入 dropdown-menu） -->
        <Popover v-else-if="!readonly" v-model:open="menuOpen">
          <PopoverTrigger as-child>
            <Button size="icon-sm" variant="ghost" :aria-label="t('components.bi.widget.edit')" data-action="menu">
              <EllipsisIcon />
            </Button>
          </PopoverTrigger>
          <PopoverContent align="end" class="w-40 gap-1 p-1.5">
            <Button size="sm" variant="ghost" class="justify-start" data-action="edit" @click="act('edit')">
              <PencilIcon data-icon="inline-start" />{{ t('components.bi.widget.edit') }}
            </Button>
            <Button size="sm" variant="ghost" class="justify-start" data-action="duplicate" @click="act('duplicate')">
              <CopyIcon data-icon="inline-start" />{{ t('components.bi.widget.duplicate') }}
            </Button>
            <Button size="sm" variant="ghost" class="justify-start text-destructive" data-action="remove" @click="act('remove')">
              <Trash2Icon data-icon="inline-start" />{{ t('components.bi.widget.remove') }}
            </Button>
          </PopoverContent>
        </Popover>
      </div>

      <SbAsyncRegion
        :pending="pending && !render"
        :refreshing="pending && Boolean(render)"
        :show-error="Boolean(error)"
        :error="error"
        :show-empty="!pending && !error && render?.kind === 'empty'"
        block="chart"
        :empty-title="t('pages.dataBrowser.empty')"
        @retry="emit('retry')"
      >
        <template v-if="render?.kind === 'chart'">
          <BiChart
            :option="render.option"
            :height-class="heightClass"
            :aria-label="t('components.bi.chart.ariaLabel', { title: widget.title, type: typeLabel })"
            @select="(p) => emit('select', p)"
          />
        </template>

        <div v-else-if="render?.kind === 'kpi'" class="flex items-baseline gap-2">
          <span class="text-3xl font-medium tabular-nums text-foreground" data-testid="kpi-value">{{ kpiText }}</span>
          <span class="truncate text-xs text-muted-foreground">{{ render.label }}</span>
        </div>

        <div v-else-if="render?.kind === 'table'" class="max-h-72 overflow-auto rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead v-for="(column, i) in render.table.columns" :key="i">{{ column }}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="(row, r) in render.table.rows" :key="r">
                <TableCell v-for="(cell, c) in row" :key="c" class="tabular-nums">{{ cellText(cell) }}</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>
      </SbAsyncRegion>

      <!-- 预览查询失败：让助手修正 -->
      <div v-if="preview && error" class="flex justify-end">
        <Button size="sm" variant="outline" data-action="fix" @click="emit('fix', error)">{{ t('components.bi.agent.fixWithAgent') }}</Button>
      </div>

      <p v-if="result?.truncated" class="text-xs text-amber-600 dark:text-amber-400" data-testid="bi-truncated">
        {{ result.columns.includes('y') ? t('components.bi.chart.truncatedPoints', { n: result.rows.length }) : t('components.bi.chart.truncated', { n: result.rows.length }) }}
      </p>
    </CardContent>
  </Card>
</template>

<script setup lang="ts">
// BiWidgetCard：单个图表卡（planv6.7 §2.2）。
// 看板内与 Agent 预览共用同一个组件（preview 模式只换右上角按钮），
// 因此「预览看到的」与「加入看板后看到的」是同一套渲染。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { CopyIcon, EllipsisIcon, PencilIcon, PlusIcon, Trash2Icon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import BiChart from '@/components/bi/BiChart.vue'
import type { AggregateResult, BiWidget } from '@/services/types'
import type { BiRender } from '@/utils/bi-chart'
import { formatKpi } from '@/utils/bi-chart'
import type { ResolvedType } from '@/utils/bi-auto'

const props = withDefaults(
  defineProps<{
    widget: BiWidget
    result: AggregateResult | null
    render: BiRender | null
    resolved: ResolvedType
    pending?: boolean
    error?: string
    /** Agent 预览模式。 */
    preview?: boolean
    /** 只读（系统项目 / 无写权限）：隐藏编辑菜单。 */
    readonly?: boolean
  }>(),
  { pending: false, error: '', preview: false, readonly: false }
)

const emit = defineEmits<{
  edit: []
  duplicate: []
  remove: []
  retry: []
  add: []
  discard: []
  fix: [error: string]
  select: [payload: { name: string; seriesName: string; value: unknown }]
}>()

const { t } = useI18n()
const menuOpen = ref(false)

function act(action: 'edit' | 'duplicate' | 'remove') {
  menuOpen.value = false
  // 联合类型直接传给 emit 会匹配不到重载，逐个分支触发。
  if (action === 'edit') emit('edit')
  else if (action === 'duplicate') emit('duplicate')
  else emit('remove')
}

const typeLabel = computed(() => t(`components.bi.types.${props.resolved}`))

const kpiText = computed(() => (props.render?.kind === 'kpi' ? formatKpi(props.render.value) : ''))

/** 横向条形类类目多时增高：每类目约 28px，封顶 28rem；其余固定高度。 */
const heightClass = computed(() => {
  const horizontal = props.resolved === 'bar' || props.resolved === 'grouped_bar' || props.resolved === 'stacked_bar'
  if (!horizontal || !props.result) return 'h-64 sm:h-72'
  const categories = props.result.rows.length
  return categories > 8 ? 'h-[28rem]' : 'h-64 sm:h-72'
})

const meta = computed(() => {
  if (!props.result) return ''
  return t('components.bi.chart.scanned', { n: props.result.scanned.toLocaleString() })
})

function cellText(v: unknown): string {
  if (v === null || v === undefined) return ''
  return typeof v === 'object' ? JSON.stringify(v) : String(v)
}
</script>
