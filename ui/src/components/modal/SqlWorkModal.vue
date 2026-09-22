<template>
  <SbModal
    :open="open"
    :title="modalTitle"
    :width="880"
    @update:open="onOpenChange"
  >
    <div class="flex flex-col gap-3">
      <ToggleGroup v-model="mode" type="single" variant="outline" spacing="0">
        <ToggleGroupItem value="query">查询</ToggleGroupItem>
        <ToggleGroupItem v-if="!readonly" value="execute">执行</ToggleGroupItem>
        <ToggleGroupItem v-if="!readonly" value="batch">批量</ToggleGroupItem>
      </ToggleGroup>
      <p v-if="readonly" class="m-0 text-xs text-muted-foreground">系统库只读：仅支持 SELECT 查询</p>

      <Textarea
        v-model="sqlText"
        :rows="mode === 'batch' ? 8 : 6"
        class="font-mono text-sm"
        :placeholder="sqlPlaceholder"
      />
      <div v-if="mode === 'query'" class="flex flex-wrap items-center gap-2">
        <span class="text-sm text-muted-foreground">参数（JSON 数组，可空）</span>
        <Input v-model="argsText" class="max-w-70" placeholder='["u-1001"]' />
        <Input
          v-model="maxRowsText"
          type="number"
          class="w-30"
          placeholder="行数上限"
          min="1"
          max="1000"
        />
      </div>
      <div v-else-if="mode === 'execute'" class="flex flex-wrap items-center gap-2">
        <span class="text-sm text-muted-foreground">参数（JSON 数组，可空）</span>
        <Input v-model="argsText" class="max-w-70" placeholder='["u-1001"]' />
      </div>
      <div v-else class="flex flex-wrap items-center gap-2">
        <span class="text-sm text-muted-foreground">每行一条 SQL（可带 -- args=[...] 注释）</span>
        <div class="flex items-center gap-2">
          <Switch :checked="transactional" @update:checked="transactional = $event" />
          <span class="text-sm">{{ transactional ? '事务' : '独立' }}</span>
        </div>
      </div>
      <p v-if="sqlHint" class="text-xs text-warning">{{ sqlHint }}</p>

      <Separator />

      <div class="min-h-30">
        <template v-if="mode === 'query'">
          <template v-if="queryResult">
            <div class="mb-3 flex flex-wrap items-center gap-2">
              <Badge variant="outline">{{ queryResult.rowCount }} 行</Badge>
              <Badge variant="secondary">{{ queryResult.durationMs }} ms</Badge>
              <span class="font-mono text-xs text-muted-foreground">request_id: {{ queryResult.requestId }}</span>
            </div>
            <div v-if="queryResult.columns.length && queryResult.rowCount > 0" class="overflow-x-auto rounded-lg border border-border">
              <Table>
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
            <SbEmptyState v-else description="暂时未查询到数据" />
            <TablePager
              v-if="queryResult.rowCount > 0"
              :page="queryPage"
              :page-size="pageSize"
              :total="queryTotal"
              :page-count="queryPageCount"
              @update:page="queryPage = $event"
            />
          </template>
          <SbEmptyState v-else description="执行后结果将显示在这里" />
        </template>

        <template v-else-if="mode === 'execute'">
          <template v-if="executeResult">
            <div class="flex flex-wrap items-center gap-2">
              <Badge variant="success">{{ executeResult.rowsAffected }} 行受影响</Badge>
              <Badge variant="secondary">{{ executeResult.durationMs }} ms</Badge>
              <Badge variant="warning">{{ executeResult.durability }}</Badge>
              <span class="font-mono text-xs text-muted-foreground">request_id: {{ executeResult.requestId }}</span>
            </div>
          </template>
          <SbEmptyState v-else description="执行后结果将显示在这里" />
        </template>

        <template v-else>
          <template v-if="batchResult">
            <div class="mb-3 flex flex-wrap items-center gap-2">
              <Badge variant="outline">{{ batchResult.results.length }} 条语句</Badge>
              <Badge variant="secondary">{{ batchResult.durationMs }} ms</Badge>
              <Badge variant="warning">{{ batchResult.durability }}</Badge>
            </div>
            <Alert v-if="batchResult.error" variant="destructive" class="mb-3">
              <AlertTitle>事务回滚：第 {{ batchResult.error.failedIndex + 1 }} 条失败（{{ batchResult.error.code }}）</AlertTitle>
              <AlertDescription>{{ batchResult.error.message }}</AlertDescription>
            </Alert>
            <div class="overflow-x-auto rounded-lg border border-border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead class="w-14">#</TableHead>
                    <TableHead class="w-20">状态</TableHead>
                    <TableHead class="w-28">受影响行数</TableHead>
                    <TableHead class="w-24">耗时</TableHead>
                    <TableHead>错误详情</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <TableRow v-for="(record, index) in batchResult.results" :key="index" class="text-xs">
                    <TableCell class="font-mono">{{ index + 1 }}</TableCell>
                    <TableCell>
                      <Badge :variant="record.errorCode ? 'destructive' : 'success'">
                        {{ record.errorCode ? '失败' : '成功' }}
                      </Badge>
                    </TableCell>
                    <TableCell class="font-mono">{{ record.rowsAffected ?? '-' }}</TableCell>
                    <TableCell class="font-mono">{{ (record.durationMs ?? '-') + ' ms' }}</TableCell>
                    <TableCell>
                      <span v-if="record.errorMessage" class="text-xs text-destructive font-mono">{{ record.errorMessage }}</span>
                      <span v-else>-</span>
                    </TableCell>
                  </TableRow>
                </TableBody>
              </Table>
            </div>
          </template>
          <SbEmptyState v-else description="执行后结果将显示在这里" />
        </template>
      </div>
    </div>

    <template #footer>
      <Button variant="outline" @click="onOpenChange(false)">关闭</Button>
      <Button :disabled="!canRun || running" @click="run">
        <Spinner v-if="running" data-icon="inline-start" />
        {{ modeLabel }}
      </Button>
    </template>
  </SbModal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { api } from '../../services/api'
import type { DatabaseItem, SqlBatchResult, SqlExecuteResult, SqlQueryResult } from '../../services/api'
import { usePagination } from '../../composables/usePagination'
import SbEmptyState from '../SbEmptyState.vue'
import TablePager from '../TablePager.vue'
import SbModal from './SbModal.vue'

const props = defineProps<{
  open: boolean
  projectId: string
  database: DatabaseItem | null
  /** 只读模式（admin 系统库）：隐藏执行/批量，仅保留查询。 */
  readonly?: boolean
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

type Mode = 'query' | 'execute' | 'batch'
const mode = ref<Mode>('query')
watch(
  () => props.readonly,
  (v) => {
    if (v) mode.value = 'query'
  },
  { immediate: true }
)
const sqlText = ref('')
const argsText = ref('')
const maxRowsText = ref('')
const transactional = ref(true)
const running = ref(false)
const queryResult = ref<SqlQueryResult | null>(null)
const executeResult = ref<SqlExecuteResult | null>(null)
const batchResult = ref<SqlBatchResult | null>(null)

const queryRows = computed(() => {
  const r = queryResult.value
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

const modalTitle = computed(() =>
  props.database ? `SQL 工作台 · ${props.database.name}` : 'SQL 工作台'
)

const modeLabel = computed(
  () => ({ query: '执行查询', execute: '执行语句', batch: '执行批量' })[mode.value]
)

const sqlPlaceholder = computed(() => {
  if (mode.value === 'batch') return '每行一条 SQL，例如：\nINSERT INTO users (id) VALUES (?)\n-- args=["u-1"]'
  return 'SELECT id, data FROM "users" WHERE id = ?'
})

const canRun = computed(() => {
  if (!props.database || running.value) return false
  return sqlText.value.trim().length > 0
})

const sqlHint = computed(() => {
  if (mode.value === 'query' && /^\s*(insert|update|delete|drop|alter|create|truncate)\b/i.test(sqlText.value.trim())) {
    return '检测到写语句：query 是只读意图，服务端会拒绝（write_in_read_only）。请切换到「执行」模式。'
  }
  return ''
})

function formatCell(v: unknown): string {
  if (v === null || v === undefined) return 'NULL'
  if (typeof v === 'string') return v
  return JSON.stringify(v)
}

function parseArgs(): unknown[] {
  const t = argsText.value.trim()
  if (!t) return []
  try {
    const parsed = JSON.parse(t)
    if (!Array.isArray(parsed)) throw new Error('参数必须是 JSON 数组')
    return parsed
  } catch (e) {
    throw new Error(`参数解析失败：${e instanceof Error ? e.message : String(e)}`)
  }
}

function parseBatch(): { sql: string; args?: unknown[] }[] {
  return sqlText.value
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l && !l.startsWith('--'))
    .map((l) => {
      const m = l.match(/--\s*args\s*=\s*(\[[\s\S]*?\])\s*$/)
      if (m) {
        try {
          return { sql: l.slice(0, m.index).trim(), args: JSON.parse(m[1]) as unknown[] }
        } catch {
          return { sql: l }
        }
      }
      return { sql: l }
    })
}

function resetResults() {
  queryResult.value = null
  executeResult.value = null
  batchResult.value = null
}

function resetAll() {
  mode.value = 'query'
  sqlText.value = ''
  argsText.value = ''
  maxRowsText.value = ''
  transactional.value = true
  running.value = false
  resetResults()
}

function onOpenChange(v: boolean) {
  emit('update:open', v)
}

watch(mode, resetResults)

watch(
  () => [props.open, props.database?.id],
  () => {
    if (props.open) resetAll()
  }
)

async function run() {
  if (!props.database) {
    toast.warning('请先选择数据库')
    return
  }
  const pid = props.projectId
  const dbId = props.database.id
  try {
    running.value = true
    const maxRows = maxRowsText.value ? Number(maxRowsText.value) : undefined
    if (mode.value === 'query') {
      const result = await api.sql.query(pid, dbId, {
        sql: sqlText.value.trim(),
        args: parseArgs(),
        maxRows
      })
      queryResult.value = result
    } else if (mode.value === 'execute') {
      executeResult.value = await api.sql.execute(pid, dbId, {
        sql: sqlText.value.trim(),
        args: parseArgs()
      })
      toast.success(`执行成功：${executeResult.value.rowsAffected} 行受影响`)
    } else {
      const statements = parseBatch()
      if (!statements.length) {
        toast.warning('请输入至少一条 SQL')
        return
      }
      batchResult.value = await api.sql.batch(pid, dbId, { statements, transactional: transactional.value })
      const failed = batchResult.value.results.filter((r) => r.errorCode).length
      if (batchResult.value.error) {
        toast.warning(`事务回滚：第 ${batchResult.value.error.failedIndex + 1} 条失败`)
      } else if (failed) {
        toast.warning(`执行完成：${failed} 条失败`)
      } else {
        toast.success(`批量执行成功（${statements.length} 条）`)
      }
    }
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '执行失败')
  } finally {
    running.value = false
  }
}
</script>
