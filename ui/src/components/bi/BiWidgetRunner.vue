<template>
  <BiWidgetCard
    :widget="widget"
    :result="query.result.value"
    :render="render"
    :resolved="resolved"
    :pending="query.pending.value"
    :error="query.error.value"
    :readonly="readonly"
    @edit="emit('edit', widget)"
    @duplicate="emit('duplicate', widget.id)"
    @remove="emit('remove', widget)"
    @retry="load"
    @select="onSelect"
  />
</template>

<script setup lang="ts">
// BiWidgetRunner：一个 widget 的「请求 + 渲染」运行单元（planv6.7 §4.2）。
// 页面只传 widget 与全局条件，这里持有自己的 useBiQuery；
// 全局条件 / widget 配置 / 刷新信号任何一个变化都会重新请求，旧响应由 useBiQuery 的令牌丢弃。
import { computed, onBeforeUnmount, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import BiWidgetCard from '@/components/bi/BiWidgetCard.vue'
import { useBiQuery, type BiQueryTarget } from '@/composables/useBiQuery'
import { useBiTheme } from '@/composables/useBiTheme'
import type { AggregateQuery, BiWidget, CollectionField } from '@/services/types'
import { hintsFromRows, resolveAutoType, type ResolvedType } from '@/utils/bi-auto'
import { buildRender, type BiRender } from '@/utils/bi-chart'
import { AGGREGATE_MAX_LIMIT, dimensionKind, fieldMap, isAdditiveMetrics, metricAlias, parseGroupBy, parseMetric } from '@/utils/bi-compat'
import { binChip, bucketChip, chipFor, combine, type FilterChip } from '@/utils/bi-filter'

const props = withDefaults(
  defineProps<{
    widget: BiWidget
    target: BiQueryTarget
    fields: CollectionField[]
    /** 页面级条件（时间范围 + 全局 filter + 联动 chip 合成后的表达式）。 */
    globalFilter: string
    /** 递增的刷新信号（手动 / 自动刷新）。 */
    refreshTick: number
    readonly?: boolean
    /** 数据源是否有内置 created_at / updated_at：集合有，SQL 表没有。 */
    builtinTime?: boolean
  }>(),
  { readonly: false, builtinTime: true }
)

const emit = defineEmits<{
  edit: [widget: BiWidget]
  duplicate: [id: string]
  remove: [widget: BiWidget]
  chip: [chip: FilterChip]
  /** 聚合结果就绪：页面用它给「自动」类型回填类目数（不重复请求）。 */
  result: [id: string]
  /** 请求结果（成功或带错误码的失败）：页面据此判断「所有卡片是否都因条件而失败」。 */
  outcome: [payload: BiOutcome]
}>()

/** 单张卡片的请求结果；页面只关心成败与错误码，不关心具体文案。 */
export interface BiOutcome {
  ok: boolean
  code: string
  message: string
}

const { t } = useI18n()
const { theme } = useBiTheme()
const query = useBiQuery(t('pages.dataBrowser.loadFailed'))
const index = computed(() => fieldMap(props.fields, props.builtinTime))

/** 饼图 / 条形图的取数上限：要在前端算出 Top N 与「其他」，必须多取一些，而不是只取 N 个。 */
const RANKED_FETCH_LIMIT = 100

/**
 * 后端请求：widget 自己的 filter 与页面级条件取 AND。
 * 饼图 / 条形图是「排名」语义：按第一个度量降序取数，且取数上限与展示的 Top N 分开——
 * 否则服务端缺省按名称升序截断，前端拿到的只是名称靠前的几类，占比与排名都是错的。
 */
const request = computed<AggregateQuery>(() => {
  const w = props.widget
  const filter = combine([props.globalFilter, w.filter]) || undefined
  if (w.x && w.y) {
    return { mode: 'points', x: w.x, y: w.y, size: w.size, series: w.series, filter, limit: w.limit }
  }
  let sort = w.sort
  let limit = w.limit
  // 占比要真实分母：只有单个可累加度量（count / sum）才能把分组结果相加，
  // avg / min / max / distinct 相加没有意义，后端也不会给总量。
  let withTotal = false
  const ranked = w.type === 'pie' || w.type === 'bar'
  if (ranked && !w.series && w.metrics.length > 0 && dimensionKind(w.groupBy) === 'category') {
    const first = parseMetric(w.metrics[0])
    if (!sort && first) sort = `-${metricAlias(first)}`
    limit = Math.min(Math.max(w.limit ?? 0, RANKED_FETCH_LIMIT), AGGREGATE_MAX_LIMIT)
    withTotal = w.type === 'pie' && w.metrics.length === 1 && isAdditiveMetrics(w.metrics)
  }
  // 只在真的要总量时才带这个键：其它图表不额外多一次聚合查询。
  return {
    metrics: w.metrics,
    groupBy: w.groupBy,
    series: w.series,
    filter,
    sort,
    limit,
    ...(withTotal ? { withTotal: true } : {})
  }
})

function load() {
  void query.run(props.target, request.value)
}

// 请求落定后上报结果：失败的错误码是页面区分「条件写错了」与「数据/网络问题」的唯一依据。
watch(
  () => [query.pending.value, query.error.value, query.errorCode.value] as const,
  ([pending]) => {
    if (pending) return
    emit('outcome', {
      ok: !query.error.value,
      code: query.errorCode.value,
      message: query.error.value
    })
  }
)

watch(
  () => [JSON.stringify(request.value), props.target.projectId, props.target.databaseId, props.target.collection, props.refreshTick],
  load,
  { immediate: true }
)
onBeforeUnmount(() => query.cancel())

const hints = computed(() => (query.result.value ? hintsFromRows(query.result.value.columns, query.result.value.rows) : {}))

/** 「自动」按规则解析：结果回来后用真实类目数重新解析，不重复请求。 */
const resolved = computed<ResolvedType>(() =>
  props.widget.type === 'auto' ? resolveAutoType(props.widget, index.value, hints.value) : props.widget.type
)

const labels = computed(() => ({
  other: t('components.bi.chart.other'),
  empty: t('components.bi.chart.emptyValue'),
  metricName: (alias: string) => {
    const [kind, ...rest] = alias.split('_')
    const field = rest.join('_')
    if (alias === 'count') return t('components.bi.metric.count')
    if (['sum', 'avg', 'min', 'max', 'distinct'].includes(kind)) return t(`components.bi.metric.${kind}`, { field })
    return alias
  }
}))

const render = computed<BiRender | null>(() => {
  const result = query.result.value
  if (!result) return null
  return buildRender({ widget: props.widget, type: resolved.value, result, theme: theme.value, labels: labels.value })
})

/**
 * 点击图元 → 联动筛选 chip（时间桶 / 分箱 / 分类各有自己的区间或等值表达式）。
 * 点击事件里的 name 是**展示名**：合并出来的「其他」、NULL / 空串、以及与别的类目重名的类目，
 * 都不对应「某一个真实值」，不能拿展示名去拼 `field="其他"` 这样会筛错的条件。
 * 这些类目由 render.drill 标出，这里提示原因并放弃下钻。系列（seriesName）暂不参与下钻，只筛主维度。
 */
function onSelect(payload: { name: string; seriesName: string; value: unknown }) {
  const group = parseGroupBy(props.widget.groupBy)
  if (!group) return
  const drill = render.value?.kind === 'chart' ? render.value.drill?.get(payload.name) : undefined
  if (drill && drill !== 'ok') {
    toast.info(t(`components.bi.drill.${drill}`))
    return
  }
  const type = index.value.get(group.field)?.type
  let chip: FilterChip | null = null
  if (group.unit) chip = bucketChip(group.field, group.unit, payload.name)
  else if (group.bin !== undefined) chip = binChip(group.field, payload.name, group.bin)
  else chip = chipFor(group.field, payload.name, type)
  if (chip) emit('chip', chip)
}

// 暴露给页面：集成测试与看板可读取查询状态和解析后的图表类型。
defineExpose({ query, reload: load, resolved })
</script>
