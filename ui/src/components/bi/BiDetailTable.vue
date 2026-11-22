<template>
  <Card data-testid="bi-detail">
    <CardContent class="flex flex-col gap-3 p-4 sm:p-5">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="flex min-w-0 flex-wrap items-baseline gap-2">
          <h3 class="text-sm font-medium text-foreground">{{ t('components.bi.detail.title') }}</h3>
          <span class="text-xs text-muted-foreground" data-testid="bi-detail-total">{{ t('components.bi.detail.rows', { n: total.toLocaleString() }) }}</span>
        </div>
        <Popover v-if="props.columns.length" v-model:open="colsOpen">
          <PopoverTrigger as-child>
            <Button size="sm" variant="outline" data-action="columns">
              <Columns3Icon data-icon="inline-start" />{{ t('components.bi.detail.columns') }}
            </Button>
          </PopoverTrigger>
          <PopoverContent align="end" class="max-h-72 w-56 overflow-y-auto">
            <label v-for="column in props.columns" :key="column" class="flex items-center gap-2 text-xs">
              <Switch :model-value="visible.includes(column)" :data-column="column" @update:model-value="toggle(column, Boolean($event))" />
              <span class="truncate">{{ column }}</span>
            </label>
          </PopoverContent>
        </Popover>
      </div>

      <div class="overflow-hidden rounded-lg border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead v-for="column in shown" :key="column">{{ column }}</TableHead>
            </TableRow>
          </TableHeader>
          <SbAsyncRegion
            as="tbody"
            :columns="Math.max(shown.length, 1)"
            :pending="loading"
            :show-empty="!loading && rows.length === 0 && !error"
            :show-error="Boolean(error)"
            :error="error"
            :empty-title="t('pages.dataBrowser.empty')"
            @retry="emit('retry')"
          >
            <TableRow v-for="(row, index) in rows" :key="index">
              <TableCell v-for="column in shown" :key="column" class="min-w-32 max-w-72 align-top">
                <!-- NULL 与空字符串、0、false 必须能区分：只把「没有值」显示成破折号并标出来 -->
                <span v-if="isNull(row[column])" class="text-xs text-muted-foreground" data-null data-testid="cell-null" :title="t('components.bi.detail.null')">—</span>
                <span v-else-if="mono.includes(column)" class="sb-mono text-xs break-all">{{ text(row[column]) }}</span>
                <div v-else class="flex items-start gap-1">
                  <span
                    class="min-w-0 text-sm"
                    :class="isExpanded(index, column, row[column]) ? 'whitespace-pre-wrap break-all' : 'block truncate'"
                    :title="text(row[column])"
                    :data-long="isLong(row[column]) ? true : undefined"
                    :data-expanded="isExpanded(index, column, row[column]) ? true : undefined"
                    @click="toggleExpand(index, column, row[column])"
                  >{{ text(row[column]) }}</span>
                  <Button
                    v-if="isLong(row[column])"
                    size="icon-sm"
                    variant="ghost"
                    class="shrink-0"
                    :aria-label="t('common.copy')"
                    data-action="copy-cell"
                    @click="copy(row[column])"
                  >
                    <CopyIcon />
                  </Button>
                </div>
              </TableCell>
            </TableRow>
          </SbAsyncRegion>
        </Table>
        <TablePager variant="footer" :page="page" :page-size="pageSize" :total="total" :page-count="pageCount" @update:page="emit('update:page', $event)" />
      </div>
    </CardContent>
  </Card>
</template>

<script setup lang="ts">
// BiDetailTable：只读明细表（planv6.7 §2.2，F8）。
// 原 DataBrowser 的行内编辑已移除（编辑统一走数据管理页的 DocumentListModal），
// 这里只做展示、分页、列显隐；数据由宿主随全局条件加载。
//
// 输入是「列名 + 归一化行」：集合文档与 SQL 表各自在页面层转成同一形状（见 utils/bi-source.ts），
// 表格不再关心数据源是什么。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Columns3Icon, CopyIcon } from '@lucide/vue'
import { copyText } from '@/utils/clipboard'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Switch } from '@/components/ui/switch'
import { Table, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import TablePager from '@/components/TablePager.vue'

const props = withDefaults(
  defineProps<{
    /** 列名（有序）。 */
    columns: string[]
    /** 每行一个「列名 → 值」对象。 */
    rows: Array<Record<string, unknown>>
    /** 用等宽字体展示的列（id / 时间戳这类机器值）。 */
    mono?: string[]
    loading: boolean
    error: string
    page: number
    pageSize: number
    total: number
    /** 列显隐的持久化键（按「项目 + 库 + 数据源」）：不同来源的列集合不同，不能共用一份偏好。 */
    storageKey?: string
  }>(),
  { mono: () => [], storageKey: '' }
)

const emit = defineEmits<{ 'update:page': [page: number]; retry: [] }>()
const { t } = useI18n()

/** 超过这个长度的单元格默认截断，点击展开并可以复制。 */
const LONG_CHARS = 60

const colsOpen = ref(false)
const visible = ref<string[]>([])
const expanded = ref<Set<string>>(new Set())

const pageCount = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)))
const shown = computed(() => props.columns.filter((c) => visible.value.includes(c)))

function readSaved(key: string): string[] | null {
  if (!key) return null
  try {
    const raw = window.localStorage.getItem(key)
    if (!raw) return null
    const parsed: unknown = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed.filter((x): x is string => typeof x === 'string') : null
  } catch {
    return null
  }
}

function saveVisible() {
  if (!props.storageKey) return
  try {
    window.localStorage.setItem(props.storageKey, JSON.stringify(visible.value))
  } catch {
    /* 隐私模式 / 配额满：只影响下次是否记住列，不影响当前展示 */
  }
}

// 列集合变化时保留用户已关掉的列，**只有新出现的列**默认显示；换数据源时改用该来源保存的偏好。
let lastKey = ''
let lastColumns: string[] = []
watch(() => [props.columns, props.storageKey] as const, ([next, key]) => {
  const switched = key !== lastKey
  const known = new Set(lastColumns)
  lastKey = key
  lastColumns = [...next]
  if (switched) {
    // 换来源：列集合完全不同，沿用上一个来源的显隐只会得到一张空表。
    const saved = readSaved(key)?.filter((c) => next.includes(c)) ?? []
    visible.value = saved.length > 0 ? saved : [...next]
    expanded.value = new Set()
    return
  }
  const kept = visible.value.filter((c) => next.includes(c))
  for (const c of next) if (!known.has(c) && !kept.includes(c)) kept.push(c)
  visible.value = kept
}, { immediate: true, deep: true })

watch(visible, saveVisible, { deep: true })

function toggle(column: string, on: boolean) {
  visible.value = on ? (visible.value.includes(column) ? visible.value : [...visible.value, column]) : visible.value.filter((c) => c !== column)
}

function isNull(value: unknown): boolean {
  return value === null || value === undefined
}

function text(value: unknown): string {
  if (isNull(value)) return ''
  if (typeof value === 'string') return value
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

function isLong(value: unknown): boolean {
  return text(value).length > LONG_CHARS
}

function cellKey(index: number, column: string): string {
  return `${index}:${column}`
}

function isExpanded(index: number, column: string, value: unknown): boolean {
  return isLong(value) && expanded.value.has(cellKey(index, column))
}

function toggleExpand(index: number, column: string, value: unknown) {
  if (!isLong(value)) return
  const key = cellKey(index, column)
  const next = new Set(expanded.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  expanded.value = next
}

async function copy(value: unknown) {
  await copyText(text(value), () => t('common.copied'), () => t('common.copyFailed'))
}
</script>
