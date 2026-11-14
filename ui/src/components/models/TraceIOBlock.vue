<template>
  <div class="rounded-md border bg-muted/30">
    <div class="border-b px-2.5 py-1.5 text-xs font-medium text-muted-foreground">
      {{ label }}
      <span v-if="truncated" class="ml-1 text-amber-600 dark:text-amber-400">{{ t('pages.traces.detail.truncated') }}</span>
    </div>
    <div class="max-h-72 overflow-auto px-2.5 py-2">
      <!-- 消息数组：逐条渲染 role + 内容（正文不转义展示更易读） -->
      <template v-if="messages">
        <div v-for="(m, i) in messages" :key="i" class="mb-1.5 last:mb-0">
          <span
            class="mr-1.5 inline-block rounded px-1 py-px font-mono text-[10px] leading-4"
            :class="roleClass(m.role)"
          >{{ m.role }}</span>
          <span class="text-xs break-all">{{ m.content }}</span>
          <div v-if="m.toolCalls?.length" class="mt-1 pl-3 text-xs text-muted-foreground">
            <div v-for="(tc, j) in m.toolCalls" :key="j" class="break-all">
              → {{ tc.function?.name || tc.name }}({{ clippedArgs(tc) }})
            </div>
          </div>
        </div>
      </template>
      <!-- llm 输出对象：正文 + tool_calls -->
      <template v-else-if="output">
        <p v-if="output.content" class="mb-1.5 text-xs whitespace-pre-wrap break-all">{{ output.content }}</p>
        <p v-if="output.error" class="mb-1.5 font-mono text-xs text-destructive break-all">{{ output.error }}</p>
        <div v-for="(tc, i) in output.toolCalls || []" :key="i" class="mb-1 text-xs break-all last:mb-0">
          <span class="mr-1 rounded bg-primary/10 px-1 py-px font-mono text-[10px] text-primary">tool_call</span>
          {{ tc.name }}({{ tc.arguments }})
        </div>
      </template>
      <!-- 其余 JSON：预览 + 展开原文 -->
      <template v-else>
        <div v-if="tools" class="space-y-1">
          <div v-for="tool in tools" :key="tool.name" class="text-xs break-all">
            <span class="font-medium">{{ tool.name }}</span>
            <span v-if="tool.description" class="text-muted-foreground"> — {{ tool.description }}</span>
          </div>
        </div>
        <pre v-else class="font-mono text-xs whitespace-pre-wrap break-all">{{ raw }}</pre>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

/**
 * Trace 内容展示块（planv6.5 二期 §9.4）：
 * 按内容形状自适应渲染——llm 输入（消息数组）、llm 输出（{content, tool_calls}）、
 * 工具声明（[{name, description}]）、其余原始 JSON。
 * 截断标记（{"truncated":true}）显示提示。
 */
const props = defineProps<{ label: string; json: string }>()

const { t } = useI18n()

interface TraceMessage {
  role?: string
  content?: string
  tool_calls?: { id?: string; function?: { name?: string; arguments?: string } }[]
}

interface TraceOutput {
  content?: string
  error?: string
  tool_calls?: { id?: string; name?: string; arguments?: string }[]
}

interface ParsedPayload {
  ok: boolean
  value: unknown
}

const truncated = computed(() => props.json.trim() === '{"truncated":true}')

const parsed = computed<ParsedPayload>(() => {
  try {
    return { ok: true, value: JSON.parse(props.json) }
  } catch {
    return { ok: false, value: undefined }
  }
})

const messages = computed<TraceMessage[] | null>(() => {
  if (truncated.value || !parsed.value.ok || !Array.isArray(parsed.value.value)) return null
  const arr = parsed.value.value
  if (arr.length === 0) return null
  const first = arr[0]
  if (first && typeof first === 'object' && 'role' in first) {
    return arr as TraceMessage[]
  }
  return null
})

const output = computed<TraceOutput | null>(() => {
  if (truncated.value || !parsed.value.ok) return null
  const v = parsed.value.value
  if (v && typeof v === 'object' && !Array.isArray(v) && ('content' in v || 'tool_calls' in v || 'error' in v)) {
    return v as TraceOutput
  }
  return null
})

const tools = computed<{ name: string; description?: string }[] | null>(() => {
  if (truncated.value || !parsed.value.ok || !Array.isArray(parsed.value.value)) return null
  const arr = parsed.value.value
  if (arr.length === 0) return null
  const first = arr[0]
  if (first && typeof first === 'object' && 'name' in first && !('role' in first)) {
    return arr as { name: string; description?: string }[]
  }
  return null
})

const raw = computed(() => parsed.value.ok ? JSON.stringify(parsed.value.value, null, 2) : props.json)

function roleClass(role?: string): string {
  if (role === 'system') return 'bg-purple-500/10 text-purple-600 dark:text-purple-400'
  if (role === 'assistant') return 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
  if (role === 'tool') return 'bg-amber-500/10 text-amber-600 dark:text-amber-400'
  return 'bg-primary/10 text-primary'
}

function clippedArgs(tc: NonNullable<TraceMessage['tool_calls']>[number]): string {
  const args = tc.function?.arguments || ''
  return args.length > 80 ? args.slice(0, 80) + '…' : args
}
</script>
