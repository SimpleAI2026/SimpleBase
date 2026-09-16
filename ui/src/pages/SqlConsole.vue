<template>
  <PageContainer title="SQL 控制台" subtitle="查询（只读）/ 执行 / 批量，SQL 必须参数化占位符">
    <a-card class="sb-card">
      <div class="sb-toolbar">
        <ProjectPicker />
        <a-select
          v-model:value="databaseId"
          style="min-width: 220px"
          placeholder="选择数据库"
          :options="dbOptions"
          :loading="dbLoading"
        >
          <template #suffixIcon><DatabaseOutlined /></template>
        </a-select>
        <a-radio-group v-model:value="mode" button-style="solid">
          <a-radio-button value="query">查询</a-radio-button>
          <a-radio-button value="execute">执行</a-radio-button>
          <a-radio-button value="batch">批量</a-radio-button>
        </a-radio-group>
        <a-button type="primary" :loading="running" :disabled="!canRun" @click="run">
          <template #icon><CaretRightOutlined /></template>
          {{ modeLabel }}
        </a-button>
      </div>
    </a-card>

    <a-card class="sb-card" title="SQL 输入">
      <a-textarea
        v-model:value="sqlText"
        :rows="6"
        class="sb-sql-input"
        placeholder='SELECT id, data FROM "users" WHERE id = ?'
        :disabled="mode === 'batch'"
      />
      <div v-if="mode === 'query'" class="sb-args-row">
        <span class="sb-label">参数（JSON 数组，可空）</span>
        <a-input v-model:value="argsText" style="max-width: 420px" placeholder='["u-1001"]' />
        <a-input-number v-model:value="maxRows" :min="1" :max="1000" style="width: 120px" placeholder="行数上限" />
      </div>
      <div v-if="mode === 'batch'" class="sb-args-row">
        <span class="sb-label">每行一条 SQL（可带 /* args */ 注释形式的 JSON 参数）</span>
        <a-switch v-model:checked="transactional" checked-children="事务" un-checked-children="独立" />
      </div>
      <div v-if="sqlHint" class="sb-sql-hint">{{ sqlHint }}</div>
    </a-card>

    <a-card v-if="mode === 'query' && queryResult" class="sb-card" title="查询结果">
      <div class="sb-meta">
        <a-tag color="blue">{{ queryResult.rowCount }} 行</a-tag>
        <a-tag>{{ queryResult.durationMs }} ms</a-tag>
        <span class="sb-request-id">request_id: {{ queryResult.requestId }}</span>
      </div>
      <a-table
        v-if="queryResult.columns.length"
        :columns="queryColumns"
        :data-source="queryRows"
        :pagination="pagination"
        :scroll="{ x: 'max-content' }"
        row-key="__idx"
        size="small"
      >
        <template #bodyCell="{ column, record, text }">
          <template v-if="isJsonCell(column, record)">
            <pre class="sb-cell-json">{{ formatCell(text) }}</pre>
          </template>
          <template v-else>{{ formatCell(text) }}</template>
        </template>
      </a-table>
      <a-empty v-else description="结果为空" />
    </a-card>

    <a-card v-if="mode === 'execute' && executeResult" class="sb-card" title="执行结果">
      <div class="sb-meta">
        <a-tag color="green">{{ executeResult.rowsAffected }} 行受影响</a-tag>
        <a-tag>{{ executeResult.durationMs }} ms</a-tag>
        <a-tag color="orange">{{ executeResult.durability }}</a-tag>
        <span class="sb-request-id">request_id: {{ executeResult.requestId }}</span>
      </div>
    </a-card>

    <a-card v-if="mode === 'batch' && batchResult" class="sb-card" title="批量结果">
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
    </a-card>
  </PageContainer>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { DatabaseOutlined, CaretRightOutlined } from '@ant-design/icons-vue'
import { api, isMock } from '../services/api'
import type { DatabaseItem, SqlBatchResult, SqlExecuteResult, SqlQueryResult } from '../services/api'
import { useProjectStore } from '../stores/project'
import { usePagination } from '../composables/usePagination'
import PageContainer from '../components/PageContainer.vue'
import ProjectPicker from '../components/ProjectPicker.vue'

const projectStore = useProjectStore()
const pagination = usePagination()

type Mode = 'query' | 'execute' | 'batch'
const mode = ref<Mode>('query')
const databaseId = ref<string>()
const dbList = ref<DatabaseItem[]>([])
const dbLoading = ref(false)
const sqlText = ref('')
const argsText = ref('')
const maxRows = ref<number>()
const transactional = ref(true)
const batchText = ref('')
const running = ref(false)

const queryResult = ref<SqlQueryResult | null>(null)
const executeResult = ref<SqlExecuteResult | null>(null)
const batchResult = ref<SqlBatchResult | null>(null)

const dbOptions = computed(() =>
  dbList.value.map((d) => ({ label: `${d.name}（${d.id}）`, value: d.id }))
)

const modeLabel = computed(() =>
  ({ query: '执行查询', execute: '执行语句', batch: '执行批量' })[mode.value]
)

const canRun = computed(() => {
  if (!databaseId.value) return false
  if (running.value) return false
  if (mode.value === 'batch') return batchText.value.trim().length > 0
  return sqlText.value.trim().length > 0
})

/** 只读意图提示：query 模式下检测写语句（sqlguard 会拒绝 write_in_read_only） */
const sqlHint = computed(() => {
  if (mode.value === 'query' && /^\s*(insert|update|delete|drop|alter|create|truncate)\b/i.test(sqlText.value.trim())) {
    return '检测到写语句：query 是只读意图，服务端会拒绝（write_in_read_only）。请切换到「执行」模式。'
  }
  return ''
})

async function loadDatabases() {
  dbLoading.value = true
  try {
    dbList.value = await api.databases.list(projectStore.id)
    const ids = new Set(dbList.value.map((d) => d.id))
    if (!databaseId.value || !ids.has(databaseId.value)) {
      databaseId.value = dbList.value.length ? dbList.value[0].id : ''
    }
  } catch (e) {
    message.error(e instanceof Error ? e.message : '数据库列表加载失败')
  } finally {
    dbLoading.value = false
  }
}

/** 查询结果：二维数组按 columns zip 成对象数组供表格渲染 */
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

/** data 列按 JSON 展示（DuckLake 文档表 data 是 JSON 文本） */
function isJsonCell(_col: { key: string }, _record: unknown): boolean {
  return false
}
function formatCell(v: unknown): string {
  if (v === null || v === undefined) return 'NULL'
  if (typeof v === 'string') return v
  return JSON.stringify(v)
}

/** args 解析：JSON 数组格式，非法时报错不请求 */
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

/** batch 模式：按行拆分，行内以「-- args=[...]」注释携带参数 */
function parseBatch(): { sql: string; args?: unknown[] }[] {
  return batchText.value
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l && !l.startsWith('--'))
    .map((l) => {
      const m = l.match(/--\s*args\s*=\s*(\[[\s\S]*?\])\s*$/)
      if (m) {
        try {
          return { sql: l.slice(0, m.index).trim(), args: JSON.parse(m[1]) }
        } catch {
          return { sql: l }
        }
      }
      return { sql: l }
    })
}

watch(mode, () => {
  queryResult.value = null
  executeResult.value = null
  batchResult.value = null
})

async function run() {
  if (!databaseId.value) {
    message.warning('请先选择数据库')
    return
  }
  const pid = projectStore.id
  const dbId = databaseId.value
  try {
    running.value = true
    if (mode.value === 'query') {
      const result = await api.sql.query(pid, dbId, {
        sql: sqlText.value.trim(),
        args: parseArgs(),
        maxRows: maxRows.value
      })
      queryResult.value = result
      if (!result.columns.length) message.info('查询结果为空')
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

const batchColumns = [
  { title: '#', dataIndex: 'index', key: 'index', width: 60, customRender: ({ index }: { index: number }) => index + 1 },
  { title: '状态', key: 'status', width: 80 },
  { title: '受影响行数', key: 'rowsAffected', width: 110 },
  { title: '耗时', dataIndex: 'durationMs', key: 'durationMs', width: 100, customRender: ({ text }: { text?: number }) => (text ?? '-') + ' ms' },
  { title: '错误详情', key: 'detail' }
]

onMounted(loadDatabases)
watch(() => projectStore.id, () => {
  void loadDatabases()
})
</script>

<style scoped>
.sb-sql-input {
  font-family: var(--sb-font-mono);
  font-size: var(--sb-fs-sm);
}
.sb-args-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--sb-space-2);
  margin-top: var(--sb-space-3);
}
.sb-label {
  color: var(--sb-text-secondary);
  font-size: var(--sb-fs-sm);
}
.sb-sql-hint {
  margin-top: var(--sb-space-2);
  color: var(--sb-warning);
  font-size: var(--sb-fs-xs);
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
.sb-cell-json {
  margin: 0;
  font-family: var(--sb-font-mono);
  font-size: var(--sb-fs-xs);
  white-space: pre-wrap;
  word-break: break-all;
}
.sb-err-text {
  color: var(--sb-danger);
  font-size: var(--sb-fs-xs);
}
</style>
