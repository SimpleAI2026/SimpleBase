<template>
  <SbModal
    :open="open"
    :title="modalTitle"
    :width="880"
    @update:open="onOpenChange"
  >
    <div class="flex flex-col gap-3">
      <ToggleGroup v-model="mode" type="single" variant="outline" spacing="0">
        <ToggleGroupItem value="query">{{ t('components.sqlWork.modeQuery') }}</ToggleGroupItem>
        <ToggleGroupItem v-if="!readonly" value="execute">{{ t('components.sqlWork.modeExecute') }}</ToggleGroupItem>
        <ToggleGroupItem v-if="!readonly" value="batch">{{ t('components.sqlWork.modeBatch') }}</ToggleGroupItem>
      </ToggleGroup>
      <p v-if="readonly" class="m-0 text-xs text-muted-foreground">{{ t('components.sqlWork.readonlyHint') }}</p>

      <Textarea
        v-model="sqlText"
        :rows="mode === 'batch' ? 8 : 6"
        class="font-mono text-sm"
        :placeholder="sqlPlaceholder"
      />
      <div v-if="mode === 'query'" class="flex flex-wrap items-center gap-2">
        <span class="text-sm text-muted-foreground">{{ t('components.sqlWork.paramsLabel') }}</span>
        <Input v-model="argsText" class="min-w-0 flex-1 sm:max-w-70" placeholder='["u-1001"]' />
        <Input
          v-model="maxRowsText"
          type="number"
          class="w-30 shrink-0"
          :placeholder="t('components.sqlWork.rowsLimitPlaceholder')"
          min="1"
          max="1000"
        />
      </div>
      <div v-else-if="mode === 'execute'" class="flex flex-wrap items-center gap-2">
        <span class="text-sm text-muted-foreground">{{ t('components.sqlWork.paramsLabel') }}</span>
        <Input v-model="argsText" class="min-w-0 flex-1 sm:max-w-70" placeholder='["u-1001"]' />
      </div>
      <div v-else class="flex flex-wrap items-center gap-2">
        <span class="text-sm text-muted-foreground">{{ t('components.sqlWork.batchHint') }}</span>
        <div class="flex items-center gap-2">
          <Switch :model-value="transactional" @update:model-value="transactional = $event" />
          <span class="text-sm">{{ transactional ? t('components.sqlWork.transactional') : t('components.sqlWork.independent') }}</span>
        </div>
      </div>
      <p v-if="sqlHint" class="text-xs text-warning">{{ sqlHint }}</p>

      <Separator />

      <div class="min-h-30">
        <SbAsyncRegion
          :pending="running"
          :show-skeleton="showSkeleton"
          :show-empty="showEmpty"
          :show-error="showError"
          :refreshing="refreshing"
          :error="loadError"
          :block="mode === 'query' ? 'lines' : 'spinner'"
          :empty-description="t('components.sqlWork.emptyDesc')"
          @retry="run"
        >
          <template #skeleton>
            <Table v-if="mode === 'query'">
              <TableBody>
                <SbTableSkeleton :columns="4" :rows="4" />
              </TableBody>
            </Table>
            <SbBlockSkeleton v-else variant="spinner" />
          </template>
          <SqlResultPanel
            :mode="mode"
            :query-result="queryResult"
            :execute-result="executeResult"
            :batch-result="batchResult"
          />
        </SbAsyncRegion>
      </div>
    </div>

    <template #footer>
      <Button variant="outline" @click="onOpenChange(false)">{{ t('common.close') }}</Button>
      <Button :disabled="!canRun || running" @click="run">
        <Spinner v-if="running" data-icon="inline-start" />
        {{ modeLabel }}
      </Button>
    </template>
  </SbModal>
</template>

<script setup lang="ts">
/**
 * SQL 工作台（查询 / 执行 / 批量）。
 *
 * 结果区已拆到 `SqlResultPanel`（planv5.0 ui-optimization-v3 §3.6）；
 * 本文件保留表单、模式切换、解析器与请求编排 —— 解析器（parseArgs /
 * parseBatch）与 formatCell 是既有测试直接调用的表面，故仍在此暴露。
 */
import { errorMessage } from '@/utils/format'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody } from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { api } from '../../services/api'
import type { DatabaseItem, SqlBatchResult, SqlExecuteResult, SqlQueryResult } from '../../services/api'
import { useLoadState } from '@/composables/useLoadState'
import SbAsyncRegion from '../SbAsyncRegion.vue'
import SbBlockSkeleton from '../SbBlockSkeleton.vue'
import SbTableSkeleton from '../SbTableSkeleton.vue'
import SqlResultPanel from './SqlResultPanel.vue'
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

const { t } = useI18n()

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
const {
  pending: running,
  showSkeleton,
  showEmpty,
  showError,
  refreshing,
  error: loadError,
  run: runLoad,
  reset: resetLoad
} = useLoadState({ fallback: 'components.sqlWork.executeFailed' })
const queryResult = ref<SqlQueryResult | null>(null)
const executeResult = ref<SqlExecuteResult | null>(null)
const batchResult = ref<SqlBatchResult | null>(null)

const modalTitle = computed(() =>
  props.database
    ? t('components.sqlWork.titleWithDb', { name: props.database.name })
    : t('components.sqlWork.title')
)

const modeLabel = computed(
  () =>
    ({
      query: t('components.sqlWork.submitQuery'),
      execute: t('components.sqlWork.submitExecute'),
      batch: t('components.sqlWork.submitBatch')
    })[mode.value]
)

const sqlPlaceholder = computed(() => {
  if (mode.value === 'batch') return t('components.sqlWork.batchPlaceholder')
  return 'SELECT id, data FROM "users" WHERE id = ?'
})

const canRun = computed(() => {
  if (!props.database || running.value) return false
  return sqlText.value.trim().length > 0
})

const sqlHint = computed(() => {
  if (mode.value === 'query' && /^\s*(insert|update|delete|drop|alter|create|truncate)\b/i.test(sqlText.value.trim())) {
    return t('components.sqlWork.writeWarning')
  }
  return ''
})

function formatCell(v: unknown): string {
  if (v === null || v === undefined) return 'NULL'
  if (typeof v === 'string') return v
  return JSON.stringify(v)
}

function parseArgs(): unknown[] {
  const raw = argsText.value.trim()
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) throw new Error(t('components.sqlWork.paramsMustBeArray'))
    return parsed
  } catch (e) {
    throw new Error(
      t('components.sqlWork.paramsParseFailed', { message: e instanceof Error ? e.message : String(e) })
    )
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
  resetResults()
  resetLoad()
}

function onOpenChange(v: boolean) {
  emit('update:open', v)
}

watch(mode, () => {
  resetResults()
  resetLoad()
}, { flush: 'sync' })

watch(
  () => [props.open, props.database?.id],
  () => {
    if (props.open) resetAll()
  }
)

async function run() {
  if (!props.database) {
    toast.warning(t('components.sqlWork.selectDbFirst'))
    return
  }
  const pid = props.projectId
  const dbId = props.database.id
  const currentMode = mode.value
  let args: unknown[] = []
  if (currentMode !== 'batch') {
    try {
      args = parseArgs()
    } catch (e) {
      toast.error(errorMessage(e, t('components.sqlWork.executeFailed')))
      return
    }
  }
  const statements = currentMode === 'batch' ? parseBatch() : []
  if (currentMode === 'batch' && !statements.length) {
    toast.warning(t('components.sqlWork.sqlRequired'))
    return
  }
  const maxRows = maxRowsText.value ? Number(maxRowsText.value) : undefined
  await runLoad(async () => {
    if (currentMode === 'query') {
      const result = await api.sql.query(pid, dbId, {
        sql: sqlText.value.trim(),
        args,
        maxRows
      })
      queryResult.value = result
      return result.columns.length > 0 && result.rowCount > 0
    }
    if (currentMode === 'execute') {
      executeResult.value = await api.sql.execute(pid, dbId, {
        sql: sqlText.value.trim(),
        args
      })
      toast.success(t('components.sqlWork.executeSuccess', { n: executeResult.value.rowsAffected }))
      return true
    }
    batchResult.value = await api.sql.batch(pid, dbId, { statements, transactional: transactional.value })
    const failed = batchResult.value.results.filter((r) => r.errorCode).length
    if (batchResult.value.error) {
      toast.warning(t('components.sqlWork.rollbackToast', { index: batchResult.value.error.failedIndex + 1 }))
    } else if (failed) {
      toast.warning(t('components.sqlWork.batchFailed', { n: failed }))
    } else {
      toast.success(t('components.sqlWork.batchSuccess', { n: statements.length }))
    }
    return true
  })
}
</script>
