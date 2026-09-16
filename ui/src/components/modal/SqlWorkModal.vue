<template>
  <SbModal
    :open="open"
    :title="modalTitle"
    :width="880"
    :mask-closable="!running"
    @update:open="onOpenChange"
  >
    <div class="sb-sql-work">
      <a-radio-group v-model:value="mode" button-style="solid" class="sb-sql-tabs">
        <a-radio-button value="query">查询</a-radio-button>
        <a-radio-button value="execute">执行</a-radio-button>
        <a-radio-button value="batch">批量</a-radio-button>
      </a-radio-group>

      <a-textarea
        v-model:value="sqlText"
        :rows="mode === 'batch' ? 8 : 6"
        class="sb-sql-input"
        :placeholder="sqlPlaceholder"
      />
      <div v-if="mode === 'query'" class="sb-args-row">
        <span class="sb-label">参数（JSON 数组，可空）</span>
        <a-input v-model:value="argsText" style="max-width: 280px" placeholder='["u-1001"]' />
        <a-input-number v-model:value="maxRows" :min="1" :max="1000" style="width: 120px" placeholder="行数上限" />
      </div>
      <div v-else-if="mode === 'execute'" class="sb-args-row">
        <span class="sb-label">参数（JSON 数组，可空）</span>
        <a-input v-model:value="argsText" style="max-width: 280px" placeholder='["u-1001"]' />
      </div>
      <div v-else class="sb-args-row">
        <span class="sb-label">每行一条 SQL（可带 -- args=[...] 注释）</span>
        <a-switch v-model:checked="transactional" checked-children="事务" un-checked-children="独立" />
      </div>
      <div v-if="sqlHint" class="sb-sql-hint">{{ sqlHint }}</div>

      <div class="sb-sql-result">
        <template v-if="mode === 'query'">
          <template v-if="queryResult">
            <div class="sb-meta">
              <a-tag color="blue">{{ queryResult.rowCount }} 行</a-tag>
              <a-tag>{{ queryResult.durationMs }} ms</a-tag>
              <span class="sb-request-id">request_id: {{ queryResult.requestId }}</span>
            </div>
            <a-table
              v-if="queryResult.columns.length && queryResult.rowCount > 0"
              :columns="queryColumns"
              :data-source="queryRows"
              :pagination="pagination"
              :scroll="{ x: 'max-content' }"
              row-key="__idx"
              size="small"
            >
              <template #bodyCell="{ text }">{{ formatCell(text) }}</template>
            </a-table>
            <a-empty v-else description="暂时未查询到数据" />
          </template>
          <a-empty v-else description="执行后结果将显示在这里" />
        </template>

        <template v-else-if="mode === 'execute'">
          <template v-if="executeResult">
            <div class="sb-meta">
              <a-tag color="green">{{ executeResult.rowsAffected }} 行受影响</a-tag>
              <a-tag>{{ executeResult.durationMs }} ms</a-tag>
              <a-tag color="orange">{{ executeResult.durability }}</a-tag>
              <span class="sb-request-id">request_id: {{ executeResult.requestId }}</span>
            </div>
          </template>
          <a-empty v-else description="执行后结果将显示在这里" />
        </template>

        <template v-else>
          <template v-if="batchResult">
            <div class="sb-meta">
              <a-tag color="blue">{{ batchResult.results.length }} 条语句</a-tag>
              <a-tag>{{ batchResult.durationMs }} ms</a-tag>
              <a-tag color="orange">{{ batchResult.durability }}</a-tag>
            </div>
            <a-alert
              v-if="batchResult.error"
              type="error"
              show-icon
              :message="`事务回滚：第 ${batchResult.error.failedIndex + 1} 条失败（${batchResult.error.code}）`"
              :description="batchResult.error.message"
              style="margin-bottom: 12px"
            />
            <a-table
              :columns="batchColumns"
              :data-source="batchResult.results"
              :pagination="false"
              row-key="index"
              size="small"
            >
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'status'">
                  <a-tag :color="record.errorCode ? 'error' : 'success'">
                    {{ record.errorCode ? '失败' : '成功' }}
                  </a-tag>
                </template>
                <template v-else-if="column.key === 'rowsAffected'">
                  {{ record.rowsAffected ?? '-' }}
                </template>
                <template v-else-if="column.key === 'detail'">
                  <span v-if="record.errorMessage" class="sb-err-text">{{ record.errorMessage }}</span>
                  <span v-else>-</span>
                </template>
              </template>
            </a-table>
          </template>
          <a-empty v-else description="执行后结果将显示在这里" />
        </template>
      </div>
    </div>

    <template #footer>
      <a-button @click="onOpenChange(false)">关闭</a-button>
      <a-button type="primary" :loading="running" :disabled="!canRun" @click="run">
        {{ modeLabel }}
      </a-button>
    </template>
  </SbModal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { api } from '../../services/api'
import type { DatabaseItem, SqlBatchResult, SqlExecuteResult, SqlQueryResult } from '../../services/api'
import { usePagination } from '../../composables/usePagination'
import SbModal from './SbModal.vue'

const props = defineProps<{
  open: boolean
  projectId: string
  database: DatabaseItem | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

type Mode = 'query' | 'execute' | 'batch'
const mode = ref<Mode>('query')
const sqlText = ref('')
const argsText = ref('')
const maxRows = ref<number>()
const transactional = ref(true)
const running = ref(false)
const queryResult = ref<SqlQueryResult | null>(null)
const executeResult = ref<SqlExecuteResult | null>(null)
const batchResult = ref<SqlBatchResult | null>(null)
const pagination = usePagination()

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

const queryColumns = computed(() =>
  (queryResult.value?.columns ?? []).map((c) => ({
    title: c,
    dataIndex: c,
    key: c,
    ellipsis: true,
    width: 160
  }))
)

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

const batchColumns = [
  { title: '#', dataIndex: 'index', key: 'index', width: 60, customRender: ({ index }: { index: number }) => index + 1 },
  { title: '状态', key: 'status', width: 80 },
  { title: '受影响行数', key: 'rowsAffected', width: 110 },
  {
    title: '耗时',
    dataIndex: 'durationMs',
    key: 'durationMs',
    width: 100,
    customRender: ({ text }: { text?: number }) => (text ?? '-') + ' ms'
  },
  { title: '错误详情', key: 'detail' }
]

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
  maxRows.value = undefined
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
    message.warning('请先选择数据库')
    return
  }
  const pid = props.projectId
  const dbId = props.database.id
  try {
    running.value = true
    if (mode.value === 'query') {
      const result = await api.sql.query(pid, dbId, {
        sql: sqlText.value.trim(),
        args: parseArgs(),
        maxRows: maxRows.value
      })
      queryResult.value = result
    } else if (mode.value === 'execute') {
      executeResult.value = await api.sql.execute(pid, dbId, {
        sql: sqlText.value.trim(),
        args: parseArgs()
      })
      message.success(`执行成功：${executeResult.value.rowsAffected} 行受影响`)
    } else {
      const statements = parseBatch()
      if (!statements.length) {
        message.warning('请输入至少一条 SQL')
        return
      }
      batchResult.value = await api.sql.batch(pid, dbId, { statements, transactional: transactional.value })
      const failed = batchResult.value.results.filter((r) => r.errorCode).length
      if (batchResult.value.error) {
        message.warning(`事务回滚：第 ${batchResult.value.error.failedIndex + 1} 条失败`)
      } else if (failed) {
        message.warning(`执行完成：${failed} 条失败`)
      } else {
        message.success(`批量执行成功（${statements.length} 条）`)
      }
    }
  } catch (e) {
    message.error(e instanceof Error ? e.message : '执行失败')
  } finally {
    running.value = false
  }
}
</script>

<style scoped>
.sb-sql-work {
  display: flex;
  flex-direction: column;
  gap: var(--sb-space-3);
}
.sb-sql-tabs {
  margin-bottom: var(--sb-space-1);
}
.sb-sql-input {
  font-family: var(--sb-font-mono);
  font-size: var(--sb-fs-sm);
}
.sb-args-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--sb-space-2);
}
.sb-label {
  color: var(--sb-text-secondary);
  font-size: var(--sb-fs-sm);
}
.sb-sql-hint {
  color: var(--sb-warning);
  font-size: var(--sb-fs-xs);
}
.sb-sql-result {
  border-top: 1px solid var(--sb-border-soft);
  padding-top: var(--sb-space-3);
  min-height: 120px;
}
.sb-meta {
  display: flex;
  align-items: center;
  gap: var(--sb-space-2);
  margin-bottom: var(--sb-space-3);
  flex-wrap: wrap;
}
.sb-request-id {
  color: var(--sb-text-muted);
  font-size: var(--sb-fs-xs);
  font-family: var(--sb-font-mono);
}
.sb-err-text {
  color: var(--sb-danger);
  font-size: var(--sb-fs-xs);
}
</style>
