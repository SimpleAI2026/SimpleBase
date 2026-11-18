<template>
  <SbModal
    :open="open"
    :title="isNew ? t('components.bi.editor.titleNew') : t('components.bi.editor.titleEdit')"
    :max-width="760"
    :ok-text="t('components.bi.editor.save')"
    :ok-button-props="{ disabled: !valid }"
    @update:open="emit('update:open', $event)"
    @ok="save"
  >
    <div class="flex flex-col gap-4" data-testid="bi-editor">
      <Field>
        <FieldLabel>{{ t('components.bi.editor.name') }}</FieldLabel>
        <Input v-model="draft.title" maxlength="40" data-field="title" />
        <FieldError v-if="titleMissing">{{ t('components.bi.editor.titleRequired') }}</FieldError>
      </Field>

      <Field>
        <FieldLabel>{{ t('components.bi.editor.type') }}</FieldLabel>
        <BiChartTypePicker v-model="draft.type" :widget="draft" :fields="fields" :inferred="inferred" :builtin-time="builtinTime" allow-scatter-setup />
      </Field>

      <!-- 散点：X / Y / 大小 / 系列，不聚合 -->
      <template v-if="scatterMode">
        <div class="flex flex-wrap gap-3">
          <Field class="min-w-40 flex-1">
            <FieldLabel>{{ t('components.bi.editor.x') }}</FieldLabel>
            <Select :model-value="draft.x || undefined" @update:model-value="setField('x', $event)">
              <SelectTrigger class="w-full" data-field="x"><SelectValue /></SelectTrigger>
              <SelectContent><SelectItem v-for="f in numberFields" :key="f.name" :value="f.name">{{ f.name }}</SelectItem></SelectContent>
            </Select>
          </Field>
          <Field class="min-w-40 flex-1">
            <FieldLabel>{{ t('components.bi.editor.y') }}</FieldLabel>
            <Select :model-value="draft.y || undefined" @update:model-value="setField('y', $event)">
              <SelectTrigger class="w-full" data-field="y"><SelectValue /></SelectTrigger>
              <SelectContent><SelectItem v-for="f in numberFields" :key="f.name" :value="f.name">{{ f.name }}</SelectItem></SelectContent>
            </Select>
          </Field>
        </div>
        <div class="flex flex-wrap gap-3">
          <Field class="min-w-40 flex-1">
            <FieldLabel>{{ t('components.bi.editor.size') }}</FieldLabel>
            <Select :model-value="draft.size || NONE" @update:model-value="setField('size', $event)">
              <SelectTrigger class="w-full" data-field="size"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem :value="NONE">{{ t('components.bi.editor.none') }}</SelectItem>
                <SelectItem v-for="f in numberFields" :key="f.name" :value="f.name">{{ f.name }}</SelectItem>
              </SelectContent>
            </Select>
          </Field>
          <Field class="min-w-40 flex-1">
            <FieldLabel>{{ t('components.bi.editor.series') }}</FieldLabel>
            <Select :model-value="draft.series || NONE" @update:model-value="setField('series', $event)">
              <SelectTrigger class="w-full" data-field="series"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem :value="NONE">{{ t('components.bi.editor.none') }}</SelectItem>
                <SelectItem v-for="f in categoryFields" :key="f.name" :value="f.name">{{ f.name }}</SelectItem>
              </SelectContent>
            </Select>
          </Field>
        </div>
      </template>

      <!-- 聚合：度量 + 维度 + 系列 -->
      <template v-else>
        <Field>
          <FieldLabel>{{ t('components.bi.editor.metrics') }}</FieldLabel>
          <div class="flex flex-col gap-2">
            <div v-for="(row, i) in metricRows" :key="i" class="flex flex-wrap items-center gap-2" data-testid="metric-row">
              <Select :model-value="row.kind" @update:model-value="setMetricKind(i, $event)">
                <SelectTrigger class="w-36 shrink-0" :data-metric-kind="i"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem v-for="kind in METRIC_KINDS" :key="kind" :value="kind">{{ t(`components.bi.kind.${kind}`) }}</SelectItem>
                </SelectContent>
              </Select>
              <Select v-if="row.kind !== 'count'" :model-value="row.field || undefined" @update:model-value="setMetricField(i, $event)">
                <SelectTrigger class="min-w-32 flex-1" :data-metric-field="i"><SelectValue :placeholder="t('components.bi.editor.metricField')" /></SelectTrigger>
                <SelectContent>
                  <SelectItem v-for="f in metricFieldsFor(row.kind)" :key="f.name" :value="f.name">{{ f.name }}</SelectItem>
                </SelectContent>
              </Select>
              <Button type="button" size="icon-sm" variant="ghost" :aria-label="t('components.bi.editor.removeMetric')" :disabled="metricRows.length <= 1" :data-remove-metric="i" @click="removeMetric(i)">
                <XIcon />
              </Button>
            </div>
            <div>
              <Button type="button" size="sm" variant="outline" :disabled="draft.metrics.length >= MAX_METRICS" data-action="add-metric" @click="addMetric">
                <PlusIcon data-icon="inline-start" />{{ t('components.bi.editor.addMetric') }}
              </Button>
            </div>
          </div>
        </Field>

        <div class="flex flex-wrap gap-3">
          <Field class="min-w-40 flex-1">
            <FieldLabel>{{ t('components.bi.editor.groupBy') }}</FieldLabel>
            <Select :model-value="groupField || NONE" @update:model-value="setGroupField($event)">
              <SelectTrigger class="w-full" data-field="group"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem :value="NONE">{{ t('components.bi.editor.none') }}</SelectItem>
                <SelectItem v-for="f in groupableFields" :key="f.name" :value="f.name">{{ f.name }}</SelectItem>
              </SelectContent>
            </Select>
          </Field>

          <Field v-if="groupIsDate" class="min-w-32 sm:w-40 sm:flex-none">
            <FieldLabel>{{ t('components.bi.editor.timeBucket') }}</FieldLabel>
            <Select :model-value="parsedGroup?.unit || 'day'" @update:model-value="setUnit($event)">
              <SelectTrigger class="w-full" data-field="unit"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem v-for="u in TIME_UNITS" :key="u" :value="u">{{ t(`components.bi.unit.${u}`) }}</SelectItem>
              </SelectContent>
            </Select>
          </Field>

          <Field v-if="groupIsNumber" class="min-w-32 sm:w-40 sm:flex-none">
            <FieldLabel>{{ t('components.bi.editor.binStep') }}</FieldLabel>
            <Input type="number" min="0" step="any" :model-value="binText" data-field="bin" @update:model-value="setBin(String($event))" />
          </Field>

          <Field class="min-w-40 flex-1">
            <FieldLabel>{{ t('components.bi.editor.series') }}</FieldLabel>
            <Select :model-value="draft.series || NONE" :disabled="!groupField" @update:model-value="setField('series', $event)">
              <SelectTrigger class="w-full" data-field="series"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem :value="NONE">{{ t('components.bi.editor.none') }}</SelectItem>
                <SelectItem v-for="f in seriesFields" :key="f.name" :value="f.name">{{ f.name }}</SelectItem>
              </SelectContent>
            </Select>
          </Field>
        </div>
      </template>

      <Field>
        <FieldLabel>{{ t('components.bi.editor.filter') }}</FieldLabel>
        <Input v-model="draft.filter" :placeholder="t('pages.dataBrowser.filter')" data-field="filter" />
      </Field>

      <div v-if="!scatterMode" class="flex flex-wrap gap-3">
        <Field class="min-w-40 flex-1">
          <FieldLabel>{{ t('components.bi.editor.sort') }}</FieldLabel>
          <Select :model-value="draft.sort ? sortKey : NONE" @update:model-value="setSortKey">
            <SelectTrigger class="w-full" data-field="sort"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem :value="NONE">{{ t('components.bi.editor.sortDefault') }}</SelectItem>
              <SelectItem v-for="o in sortOptions" :key="o.value" :value="o.value">{{ o.label }}</SelectItem>
            </SelectContent>
          </Select>
        </Field>
        <Field v-if="draft.sort" class="min-w-32 sm:w-40 sm:flex-none">
          <FieldLabel>{{ t('components.bi.editor.sortDirection') }}</FieldLabel>
          <Select :model-value="sortDesc ? 'desc' : 'asc'" @update:model-value="setSortDir">
            <SelectTrigger class="w-full" data-field="sort-dir"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="desc">{{ t('components.bi.editor.sortDesc') }}</SelectItem>
              <SelectItem value="asc">{{ t('components.bi.editor.sortAsc') }}</SelectItem>
            </SelectContent>
          </Select>
        </Field>
      </div>

      <div class="flex flex-col gap-3 border-t border-border pt-3">
        <p class="text-xs text-muted-foreground">{{ t('components.bi.editor.displayGroup') }}</p>
      <div class="flex flex-wrap gap-3">
        <Field class="min-w-32 flex-1">
          <FieldLabel>{{ t('components.bi.editor.limit') }}</FieldLabel>
          <Input type="number" min="1" :max="limitMax" :model-value="draft.limit ?? ''" data-field="limit" @update:model-value="setLimit(String($event))" />
        </Field>
        <Field class="min-w-32 flex-1">
          <FieldLabel>{{ t('components.bi.editor.span') }}</FieldLabel>
          <Select :model-value="String(draft.span)" @update:model-value="setSpan($event)">
            <SelectTrigger class="w-full" data-field="span"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="4">{{ t('components.bi.editor.spanQuarter') }}</SelectItem>
              <SelectItem value="6">{{ t('components.bi.editor.spanHalf') }}</SelectItem>
              <SelectItem value="8">{{ t('components.bi.editor.spanTwoThirds') }}</SelectItem>
              <SelectItem value="12">{{ t('components.bi.editor.spanFull') }}</SelectItem>
            </SelectContent>
          </Select>
        </Field>
      </div>

      <div v-if="showStyleSwitches" class="flex flex-wrap items-center gap-5">
        <label v-if="effectiveType === 'line'" class="flex items-center gap-2 text-sm">
          <Switch :model-value="Boolean(draft.area)" data-switch="area" @update:model-value="draft.area = $event" />
          {{ t('components.bi.editor.area') }}
        </label>
        <label v-if="effectiveType === 'pie'" class="flex items-center gap-2 text-sm">
          <Switch :model-value="Boolean(draft.donut)" data-switch="donut" @update:model-value="draft.donut = $event" />
          {{ t('components.bi.editor.donut') }}
        </label>
      </div>

      </div>

      <p v-if="problem" class="text-sm text-destructive" data-testid="editor-problem">
        {{ t('components.bi.editor.incompatible', { reason: t(`components.bi.reason.${problem}`) }) }}
      </p>
    </div>
  </SbModal>
</template>

<script setup lang="ts">
// BiWidgetEditor：图表配置弹窗（planv6.7 §2.2，F9）。
// 校验全部来自 bi-compat（与类型选择器、Agent 结果校验同一份口径）；
// 保存时只回传一个 BiWidget，由页面决定是新增还是更新。
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { PlusIcon, XIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import SbModal from '@/components/modal/SbModal.vue'
import BiChartTypePicker from '@/components/bi/BiChartTypePicker.vue'
import type { BiSpan, BiWidget, CollectionField } from '@/services/types'
import { resolveAutoType } from '@/utils/bi-auto'
import {
  AGGREGATE_MAX_LIMIT,
  MAX_METRICS,
  POINTS_MAX_LIMIT,
  TIME_UNITS,
  checkType,
  fieldMap,
  formatGroupBy,
  metricAlias,
  parseGroupBy,
  parseMetric
} from '@/utils/bi-compat'
import { newWidgetId } from '@/composables/useBiDashboard'

/** Select 不允许空串作为 value，用哨兵值表示「无」。 */
const NONE = '__none__'
const METRIC_KINDS = ['count', 'sum', 'avg', 'min', 'max', 'distinct'] as const

const props = withDefaults(defineProps<{
  open: boolean
  /** 编辑已有图表 / 调整 Agent 结果时传入；新增时为 null。 */
  widget: BiWidget | null
  fields: CollectionField[]
  /** 数据源是否有内置 created_at / updated_at：集合有，SQL 表没有。 */
  builtinTime?: boolean
}>(), { builtinTime: true })
const emit = defineEmits<{ 'update:open': [value: boolean]; save: [widget: BiWidget] }>()
const { t } = useI18n()

function blank(): BiWidget {
  return { id: newWidgetId(), title: '', type: 'auto', metrics: ['count'], span: 6 }
}

const draft = reactive<BiWidget>(blank())
const isNew = ref(true)

watch(
  () => props.open,
  (open) => {
    if (!open) return
    isNew.value = !props.widget
    const src = props.widget ? { ...props.widget, metrics: [...props.widget.metrics] } : blank()
    for (const key of Object.keys(draft)) delete (draft as unknown as Record<string, unknown>)[key]
    Object.assign(draft, src)
  },
  { immediate: true }
)

const fieldIndex = computed(() => fieldMap(props.fields, props.builtinTime))
const allFields = computed(() => [...fieldIndex.value.values()])
const numberFields = computed(() => allFields.value.filter((f) => f.type === 'number'))
const categoryFields = computed(() => allFields.value.filter((f) => ['text', 'select', 'bool', 'email', 'url'].includes(f.type)))
const groupableFields = computed(() => allFields.value.filter((f) => f.type !== 'json'))
const seriesFields = computed(() => allFields.value.filter((f) => f.type !== 'json' && f.name !== groupField.value))

const scatterMode = computed(() => draft.type === 'scatter' || Boolean(draft.x || draft.y))

const parsedGroup = computed(() => parseGroupBy(draft.groupBy))
const groupField = computed(() => parsedGroup.value?.field ?? '')
const groupType = computed(() => fieldIndex.value.get(groupField.value)?.type)
const groupIsDate = computed(() => groupType.value === 'date')
const groupIsNumber = computed(() => groupType.value === 'number')
const binText = computed(() => (parsedGroup.value?.bin !== undefined ? String(parsedGroup.value.bin) : ''))

/**
 * 把度量字符串拆成编辑行。必须容忍「已选聚合方式、还没选字段」的草稿态（`sum:`）：
 * parseMetric 把它判为非法并返回 null，若据此回落成 count，用户一选「求和」界面就弹回「记录数」，
 * 字段选择器永远出不来。保存前的合法性仍由 checkType 把关。
 */
function parseMetricRow(raw: string): { kind: string; field: string } {
  const parsed = parseMetric(raw)
  if (parsed) return { kind: parsed.kind, field: parsed.field ?? '' }
  const [kind, field = ''] = raw.split(':')
  return (METRIC_KINDS as readonly string[]).includes(kind) ? { kind, field } : { kind: 'count', field: '' }
}

const metricRows = computed(() => draft.metrics.map(parseMetricRow))

function metricFieldsFor(kind: string): CollectionField[] {
  if (kind === 'sum' || kind === 'avg') return numberFields.value
  if (kind === 'min' || kind === 'max') return allFields.value.filter((f) => f.type === 'number' || f.type === 'date')
  return groupableFields.value
}

function writeMetric(i: number, kind: string, field: string) {
  const next = [...draft.metrics]
  next[i] = kind === 'count' ? 'count' : field ? `${kind}:${field}` : `${kind}:`
  draft.metrics = next
}
function setMetricKind(i: number, kind: unknown) {
  const k = String(kind)
  const current = metricRows.value[i]
  const keep = metricFieldsFor(k).some((f) => f.name === current.field) ? current.field : ''
  writeMetric(i, k, keep)
}
function setMetricField(i: number, field: unknown) {
  writeMetric(i, metricRows.value[i].kind, String(field))
}
function addMetric() {
  if (draft.metrics.length < MAX_METRICS) draft.metrics = [...draft.metrics, 'count']
}
function removeMetric(i: number) {
  if (draft.metrics.length > 1) draft.metrics = draft.metrics.filter((_, idx) => idx !== i)
}

function setGroupField(value: unknown) {
  const name = String(value)
  if (name === NONE) {
    draft.groupBy = undefined
    draft.series = undefined
    return
  }
  const type = fieldIndex.value.get(name)?.type
  // 选中日期字段默认按天分桶；数值字段默认步长 10；其余直接分组。
  draft.groupBy = type === 'date' ? `${name}:day` : type === 'number' ? `${name}:bin=10` : name
  if (draft.series === name) draft.series = undefined
}
function setUnit(unit: unknown) {
  if (groupField.value) draft.groupBy = formatGroupBy({ field: groupField.value, unit: unit as (typeof TIME_UNITS)[number] })
}
function setBin(text: string) {
  const step = Number(text)
  if (!groupField.value) return
  // 非法步长保留原文，让校验给出明确提示，而不是悄悄改成别的值。
  draft.groupBy = Number.isFinite(step) && step > 0 ? formatGroupBy({ field: groupField.value, bin: step }) : `${groupField.value}:bin=${text || 0}`
}

type PlainKey = 'x' | 'y' | 'size' | 'series'
function setField(key: PlainKey, value: unknown) {
  const v = String(value)
  draft[key] = v === NONE || v === '' ? undefined : v
}
function setLimit(text: string) {
  const n = Number(text)
  draft.limit = text === '' || !Number.isFinite(n) ? undefined : n
}
function setSpan(value: unknown) {
  draft.span = Number(value) as BiSpan
}

const limitMax = computed(() => (scatterMode.value ? POINTS_MAX_LIMIT : AGGREGATE_MAX_LIMIT))

/** 可排序的列：维度 / 系列列 / 各度量别名（与后端允许的响应列一致）。 */
const sortOptions = computed(() => {
  const out: Array<{ value: string; label: string }> = []
  if (draft.groupBy) out.push({ value: 'x', label: t('components.bi.editor.sortByDimension', { field: groupField.value }) })
  if (draft.groupBy && draft.series) out.push({ value: 'series', label: t('components.bi.editor.sortBySeries', { field: draft.series }) })
  for (const raw of draft.metrics) {
    const m = parseMetric(raw)
    if (m) out.push({ value: metricAlias(m), label: metricLabel(metricAlias(m)) })
  }
  return out
})
const sortKey = computed(() => (draft.sort ?? '').replace(/^-/, ''))
const sortDesc = computed(() => (draft.sort ?? '').startsWith('-'))

function metricLabel(alias: string): string {
  if (alias === 'count') return t('components.bi.metric.count')
  const [kind, ...rest] = alias.split('_')
  const field = rest.join('_')
  return ['sum', 'avg', 'min', 'max', 'distinct'].includes(kind) ? t(`components.bi.metric.${kind}`, { field }) : alias
}
function setSortKey(value: unknown) {
  const key = String(value)
  if (key === NONE) {
    draft.sort = undefined
    return
  }
  draft.sort = sortDesc.value ? `-${key}` : key
}
function setSortDir(value: unknown) {
  const key = sortKey.value
  if (!key) return
  draft.sort = String(value) === 'desc' ? `-${key}` : key
}

// 度量 / 维度 / 系列变化后，指向已不存在列的排序必须清掉，否则保存成功但请求必然 400。
// immediate：打开一个带着陈旧 sort 的图表（如度量字段被删）时也先自愈，而不是卡住保存。
watch(
  () => sortOptions.value.map((o) => o.value).join(','),
  () => {
    if (draft.sort && !sortOptions.value.some((o) => o.value === sortKey.value)) draft.sort = undefined
  },
  { immediate: true }
)

/** 当前真正生效的类型：auto 要先按规则解析，用于决定显示哪些样式开关。 */
const resolved = computed(() => resolveAutoType(draft, fieldIndex.value))
const effectiveType = computed(() => (draft.type === 'auto' ? resolved.value : draft.type))
const inferred = computed(() => (draft.type === 'auto' ? resolved.value : null))
const showStyleSwitches = computed(() => effectiveType.value === 'line' || effectiveType.value === 'pie')

const titleMissing = computed(() => draft.title.trim() === '')
const check = computed(() => {
  // 切到散点时清掉聚合配置、切回聚合时清掉散点配置，由校验指出残留冲突。
  return checkType(draft.type, normalized(), fieldIndex.value)
})
const problem = computed(() => (check.value.ok ? '' : check.value.reason))
const valid = computed(() => !titleMissing.value && check.value.ok)

/** 选择散点类型时：保证 metrics 为空；离开散点时：丢掉 x / y / size。 */
function normalized(): BiWidget {
  const out: BiWidget = { ...draft, metrics: [...draft.metrics] }
  if (draft.type === 'scatter') {
    out.metrics = []
    out.groupBy = undefined
  } else if (!draft.x && !draft.y) {
    out.x = out.y = out.size = undefined
  }
  return out
}

watch(
  () => draft.type,
  (type, previous) => {
    if (type === 'scatter') {
      draft.metrics = []
      draft.groupBy = undefined
      return
    }
    // 离开散点：X / Y / 大小只对散点有意义，必须清掉，否则 auto / table 仍会把它当散点配置，
    // 界面停在散点表单、度量行消失，用户无路可退。
    if (previous === 'scatter') {
      draft.x = draft.y = draft.size = undefined
      if (draft.metrics.length === 0) draft.metrics = ['count']
    }
  }
)

function save() {
  if (!valid.value) return
  const out = normalized()
  out.title = out.title.trim()
  if (!out.filter?.trim()) out.filter = undefined
  else out.filter = out.filter.trim()
  // 去掉值为 undefined 的键，避免写进 localStorage 的 JSON 里留下噪音。
  const clean = Object.fromEntries(Object.entries(out).filter(([, v]) => v !== undefined)) as unknown as BiWidget
  emit('save', clean)
  emit('update:open', false)
}
</script>
