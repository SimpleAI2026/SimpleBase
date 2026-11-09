<template>
  <!-- 查询：结果表 + 分页 -->
  <template v-if="mode === 'query'">
    <template v-if="queryResult">
      <div class="mb-3 flex flex-wrap items-center gap-2">
        <Badge variant="outline">{{ t('components.sqlWork.rowCount', { n: queryResult.rowCount }) }}</Badge>
        <Badge variant="secondary">{{ queryResult.durationMs }} ms</Badge>
        <span class="font-mono text-xs text-muted-foreground">request_id: {{ queryResult.requestId }}</span>
      </div>
      <div v-if="queryResult.columns.length && queryResult.rowCount > 0" class="overflow-x-auto rounded-lg border border-border">
        <Table class="[&_td]:text-left [&_th]:text-left">
          <TableHeader>
            <TableRow>
              <TableHead v-for="c in queryResult.columns" :key="c" class="min-w-32">{{ c }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="row in queryPaged" :key="row.__idx" class="font-mono text-xs">
              <TableCell v-for="c in queryResult.columns" :key="c" class="max-w-60 truncate">
                {{ formatCell(row[c]) }}
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>
      <SbEmptyState v-else :description="t('components.sqlWork.emptyDesc')" />
      <TablePager
        v-if="queryResult.rowCount > 0"
        :page="queryPage"
        :page-size="pageSize"
        :total="queryTotal"
        :page-count="queryPageCount"
        @update:page="emit('update:page', $event)"
      />
    </template>
    <SbEmptyState v-else :description="t('components.sqlWork.emptyAfterRun')" />
  </template>

  <!-- 执行：受影响行数 -->
  <template v-else-if="mode === 'execute'">
    <template v-if="executeResult">
      <div class="flex flex-wrap items-center gap-2">
        <Badge variant="success">{{ t('components.sqlWork.rowsAffected', { n: executeResult.rowsAffected }) }}</Badge>
        <Badge variant="secondary">{{ executeResult.durationMs }} ms</Badge>
        <Badge variant="warning">{{ executeResult.durability }}</Badge>
        <span class="font-mono text-xs text-muted-foreground">request_id: {{ executeResult.requestId }}</span>
      </div>
    </template>
    <SbEmptyState v-else :description="t('components.sqlWork.emptyAfterRun')" />
  </template>

  <!-- 批量：逐条结果 -->
  <template v-else>
    <template v-if="batchResult">
      <div class="mb-3 flex flex-wrap items-center gap-2">
        <Badge variant="outline">{{ t('components.sqlWork.statementCount', { n: batchResult.results.length }) }}</Badge>
        <Badge variant="secondary">{{ batchResult.durationMs }} ms</Badge>
        <Badge variant="warning">{{ batchResult.durability }}</Badge>
      </div>
      <Alert v-if="batchResult.error" variant="destructive" class="mb-3">
        <AlertTitle>{{ t('components.sqlWork.rollbackTitle', { index: batchResult.error.failedIndex + 1, code: batchResult.error.code }) }}</AlertTitle>
        <AlertDescription>{{ batchResult.error.message }}</AlertDescription>
      </Alert>
      <div class="overflow-x-auto rounded-lg border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="w-14">#</TableHead>
              <TableHead class="w-20">{{ t('components.sqlWork.colStatus') }}</TableHead>
              <TableHead class="w-28">{{ t('components.sqlWork.colAffected') }}</TableHead>
              <TableHead class="w-24">{{ t('components.sqlWork.colDuration') }}</TableHead>
              <TableHead class="text-left">{{ t('components.sqlWork.colError') }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="(record, index) in batchResult.results" :key="index" class="text-xs">
              <TableCell class="font-mono">{{ index + 1 }}</TableCell>
              <TableCell>
                <Badge :variant="record.errorCode ? 'destructive' : 'success'">
                  {{ record.errorCode ? t('components.sqlWork.failed') : t('components.sqlWork.success') }}
                </Badge>
              </TableCell>
              <TableCell class="font-mono">{{ record.rowsAffected ?? '-' }}</TableCell>
              <TableCell class="font-mono">{{ (record.durationMs ?? '-') + ' ms' }}</TableCell>
              <TableCell class="text-left">
                <span v-if="record.errorMessage" class="text-xs text-destructive font-mono">{{ record.errorMessage }}</span>
                <span v-else>-</span>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>
    </template>
    <SbEmptyState v-else :description="t('components.sqlWork.emptyAfterRun')" />
  </template>
</template>

<script setup lang="ts">
/**
 * SQL 工作台的结果区（从 SqlWorkModal 拆出，planv5.0 ui-optimization-v3 §3.6）。
 *
 * 三个模式的结果形态互不相同但共用同一套「徽标摘要 + 表格/空态」骨架，
 * 单独成件后可独立测试，宿主只负责取数与模式切换。
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import SbEmptyState from '../SbEmptyState.vue'
import TablePager from '../TablePager.vue'
import { usePagination } from '../../composables/usePagination'
import type { SqlBatchResult, SqlExecuteResult, SqlQueryResult } from '../../services/api'

export type SqlMode = 'query' | 'execute' | 'batch'

const props = defineProps<{
  mode: SqlMode
  queryResult: SqlQueryResult | null
  executeResult: SqlExecuteResult | null
  batchResult: SqlBatchResult | null
}>()

const emit = defineEmits<{ 'update:page': [value: number] }>()

const { t } = useI18n()
const queryRows = computed(() => {
  const r = props.queryResult
  if (!r) return []
  return r.rows.map((row, i) => {
    const obj: Record<string, unknown> = { __idx: i }
    r.columns.forEach((col, j) => {
      obj[col] = row[j]
    })
    return obj
  })
})

const { page: queryPage, pageSize, total: queryTotal, pageCount: queryPageCount, items: queryPaged } =
  usePagination(queryRows)

function formatCell(v: unknown): string {
  if (v === null || v === undefined) return 'NULL'
  if (typeof v === 'string') return v
  return JSON.stringify(v)
}
</script>
