<template>
  <Card size="sm">
    <CardHeader><CardTitle class="text-xs text-primary">{{ tool.name || 'tool' }} <span v-if="tool.duration_ms !== undefined" class="text-muted-foreground">{{ tool.duration_ms }}ms</span></CardTitle></CardHeader>
    <CardContent class="space-y-2 px-3 text-xs">
      <details v-if="tool.arguments"><summary class="cursor-pointer text-muted-foreground">参数</summary><pre class="max-h-32 overflow-auto whitespace-pre-wrap">{{ pretty(tool.arguments) }}</pre></details>
      <template v-if="tool.content">
        <template v-if="tool.name === 'readonly_sql' && sqlResult">
          <div class="max-h-60 overflow-auto">
            <table class="w-full border-collapse text-left">
              <thead><tr><th v-for="column in sqlResult.Columns" :key="column" class="border-b px-1 py-1">{{ column }}</th></tr></thead>
              <tbody><tr v-for="(row, i) in sqlResult.Rows.slice(0, 20)" :key="i"><td v-for="(cell, j) in row" :key="j" class="border-b px-1 py-1">{{ cell }}</td></tr></tbody>
            </table>
          </div>
          <span class="text-muted-foreground">共 {{ sqlResult.RowCount }} 行</span>
        </template>
        <template v-else-if="tool.name?.startsWith('sandbox_') && sandboxResult">
          <span class="text-muted-foreground">exit {{ sandboxResult.exit_code ?? sandboxResult.ExitCode ?? 0 }}</span>
          <pre v-if="sandboxResult.stdout || sandboxResult.Stdout" class="max-h-48 overflow-auto whitespace-pre-wrap">{{ sandboxResult.stdout || sandboxResult.Stdout }}</pre>
          <pre v-if="sandboxResult.stderr || sandboxResult.Stderr" class="max-h-48 overflow-auto whitespace-pre-wrap text-destructive">{{ sandboxResult.stderr || sandboxResult.Stderr }}</pre>
        </template>
        <details v-else :open="pretty(tool.content).split('\n').length <= 20">
          <summary class="cursor-pointer text-muted-foreground">结果</summary>
          <pre class="max-h-48 overflow-auto whitespace-pre-wrap">{{ pretty(tool.content) }}</pre>
        </details>
      </template>
      <span v-else class="text-muted-foreground">正在调用…</span>
    </CardContent>
  </Card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { AgentToolCallCard } from '@/services/types'

interface SQLToolResult { Columns: string[]; Rows: unknown[][]; RowCount: number }
interface SandboxToolResult { stdout?: string; stderr?: string; exit_code?: number; Stdout?: string; Stderr?: string; ExitCode?: number }
const props = defineProps<{ tool: AgentToolCallCard }>()
function parse(value?: string): unknown {
  if (!value) return null
  try { return JSON.parse(value) as unknown } catch { return null }
}
function pretty(value: string) {
  const parsed = parse(value)
  return parsed == null ? value : JSON.stringify(parsed, null, 2)
}
const sqlResult = computed(() => {
  const raw = parse(props.tool.content)
  if (!raw || typeof raw !== 'object') return null
  const value = raw as Record<string, unknown>
  const columns = value.Columns ?? value.columns
  const rows = value.Rows ?? value.rows
  const count = value.RowCount ?? value.row_count
  return Array.isArray(columns) && Array.isArray(rows) ? { Columns: columns as string[], Rows: rows as unknown[][], RowCount: Number(count ?? rows.length) } as SQLToolResult : null
})
const sandboxResult = computed(() => {
  const raw = parse(props.tool.content)
  return raw && typeof raw === 'object' ? raw as SandboxToolResult : null
})
</script>
