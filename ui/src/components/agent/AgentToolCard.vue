<template>
  <!-- AgentToolCard：工具执行卡（planv4.1 §3.2）。
       完成的调用默认折叠，失败用红色标识；运行中和待确认保持展开。 -->
  <Card size="sm" :data-status="status" :class="isError && 'border-destructive/40'">
    <button
      v-if="collapsible"
      type="button"
      class="flex w-full items-center gap-1.5 px-3 py-2 text-left text-xs"
      :aria-expanded="expanded"
      :data-tool-summary="tool.name || 'tool'"
      @click="expanded = !expanded"
    >
      <ChevronDownIcon
        class="size-3.5 shrink-0 text-muted-foreground transition-transform"
        :class="!expanded && '-rotate-90'"
        aria-hidden="true"
      />
      <XIcon v-if="isError" class="size-3.5 shrink-0 text-destructive" aria-hidden="true" />
      <CheckIcon v-else class="size-3.5 shrink-0 text-success" aria-hidden="true" />
      <span class="truncate text-muted-foreground">{{ summaryText }}</span>
    </button>

    <template v-if="!collapsible || expanded">
      <CardHeader :class="collapsible && 'pt-0'">
        <CardTitle class="flex items-center gap-1.5 text-xs">
          <span :class="isError ? 'text-destructive' : 'text-primary'">{{ tool.name || 'tool' }}</span>
          <Badge v-if="isError" variant="destructive" class="text-[10px]">{{ t('components.agentToolCard.error') }}</Badge>
          <Badge v-else-if="tool.truncated" variant="secondary" class="text-[10px]">{{ t('components.agentToolCard.truncated') }}</Badge>
          <span v-if="tool.duration_ms !== undefined" class="text-muted-foreground">{{ tool.duration_ms }}ms</span>
          <span v-else-if="running" class="animate-pulse text-muted-foreground">{{ t('components.agentToolCard.running', { n: progressSecs }) }}</span>
        </CardTitle>
      </CardHeader>
      <CardContent class="space-y-2 px-3 text-xs">
      <details v-if="tool.arguments"><summary class="cursor-pointer text-muted-foreground">{{ t('components.agentToolCard.arguments') }}</summary><pre class="max-h-32 overflow-auto whitespace-pre-wrap">{{ pretty(tool.arguments) }}</pre></details>
      <template v-if="tool.content">
        <template v-if="(tool.name === 'readonly_sql' || tool.name === 'simplebase') && sqlResult">
          <div class="max-h-60 overflow-auto">
            <table class="w-full border-collapse text-left">
              <thead><tr><th v-for="column in sqlResult.Columns" :key="column" class="border-b px-1 py-1">{{ column }}</th></tr></thead>
              <tbody><tr v-for="(row, i) in sqlResult.Rows.slice(0, 20)" :key="i"><td v-for="(cell, j) in row" :key="j" class="border-b px-1 py-1">{{ cell }}</td></tr></tbody>
            </table>
          </div>
          <span class="text-muted-foreground">{{ t('components.agentToolCard.rowCount', { n: sqlResult.RowCount }) }}</span>
        </template>
        <template v-else-if="tool.name?.startsWith('sandbox_') && sandboxResult">
          <span class="text-muted-foreground">exit {{ sandboxResult.exit_code ?? sandboxResult.ExitCode ?? 0 }}</span>
          <pre v-if="sandboxResult.stdout || sandboxResult.Stdout" class="max-h-48 overflow-auto whitespace-pre-wrap">{{ sandboxResult.stdout || sandboxResult.Stdout }}</pre>
          <pre v-if="sandboxResult.stderr || sandboxResult.Stderr" class="max-h-48 overflow-auto whitespace-pre-wrap text-destructive">{{ sandboxResult.stderr || sandboxResult.Stderr }}</pre>
        </template>
        <details v-else :open="pretty(tool.content).split('\n').length <= 20">
          <summary class="cursor-pointer text-muted-foreground">{{ isError ? t('components.agentToolCard.error') : t('components.agentToolCard.result') }}</summary>
          <pre class="max-h-48 overflow-auto whitespace-pre-wrap" :class="isError && 'text-destructive'">{{ pretty(tool.content) }}</pre>
        </details>
      </template>
      <span v-else-if="running" class="text-muted-foreground">{{ progressSecs ? t('components.agentToolCard.callingWithTime', { n: progressSecs }) : t('components.agentToolCard.calling') }}</span>
      <span v-else class="text-muted-foreground">{{ t('components.agentToolCard.calling') }}</span>
    </CardContent>
    </template>
  </Card>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { CheckIcon, ChevronDownIcon, XIcon } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { AgentToolCallCard } from '@/services/types'

interface SQLToolResult { Columns: string[]; Rows: unknown[][]; RowCount: number }
interface SandboxToolResult { stdout?: string; stderr?: string; exit_code?: number; Stdout?: string; Stderr?: string; ExitCode?: number }

const props = defineProps<{ tool: AgentToolCallCard }>()

const { t } = useI18n()

function parse(value?: string): unknown {
  if (!value) return null
  try { return JSON.parse(value) as unknown } catch { return null }
}
function pretty(value: string) {
  const parsed = parse(value)
  return parsed == null ? value : JSON.stringify(parsed, null, 2)
}

/** 错误判定：显式 is_error，或内容为 {"error":...} 结构（后端 BUG-02 包装）。 */
const isError = computed(() => {
  if (props.tool.is_error) return true
  const raw = parse(props.tool.content)
  return !!raw && typeof raw === 'object' && 'error' in (raw as Record<string, unknown>)
})
const running = computed(() => !props.tool.content && props.tool.status !== 'awaiting_confirm' && props.tool.status !== 'pending' && props.tool.status !== 'generating')
const status = computed(() => {
  if (props.tool.status) return props.tool.status
  if (isError.value) return 'error'
  if (props.tool.content) return 'success'
  return 'pending'
})
const progressSecs = computed(() => (props.tool.duration_ms ? Math.floor(props.tool.duration_ms / 1000) : 0))

/** 完成后的结果默认折叠，运行中与待确认的调用仍展开。 */
const collapsible = computed(() => isError.value || status.value === 'success' || status.value === 'error')
const expanded = ref(false)

/** 折叠行的文案：工具名 + 耗时 + SQL 行数等一句摘要。 */
const summaryText = computed(() => {
  const name = props.tool.name || 'tool'
  const parts = [name]
  if (props.tool.duration_ms !== undefined) parts.push(`${props.tool.duration_ms}ms`)
  if (sqlResult.value) parts.push(t('components.agentToolCard.rowCount', { n: sqlResult.value.RowCount }))
  return parts.join(' · ')
})

const sqlResult = computed(() => {
  const raw = parse(props.tool.content)
  if (!raw || typeof raw !== 'object') return null
  const value = raw as Record<string, unknown>
  const data = value.data && typeof value.data === 'object' ? value.data as Record<string, unknown> : value
  const columns = data.Columns ?? data.columns
  const rows = data.Rows ?? data.rows
  const count = data.RowCount ?? data.row_count
  return Array.isArray(columns) && Array.isArray(rows) ? { Columns: columns as string[], Rows: rows as unknown[][], RowCount: Number(count ?? rows.length) } as SQLToolResult : null
})
const sandboxResult = computed(() => {
  const raw = parse(props.tool.content)
  return raw && typeof raw === 'object' ? raw as SandboxToolResult : null
})
</script>
