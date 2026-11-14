<template>
  <SbModal
    :open="open"
    :title="t('pages.traces.detail.title')"
    :description="traceId"
    :max-width="720"
    :min-width="560"
    hide-footer
    @update:open="onOpenChange"
  >
    <SbAsyncRegion
      :pending="pending"
      :show-skeleton="showSkeleton"
      :show-empty="showEmpty"
      :show-error="showError"
      :refreshing="refreshing"
      :error="error"
      :empty-description="t('pages.traces.detail.empty')"
      @retry="load"
    >
      <!-- 概要：时间 / 状态 / caller / tokens -->
      <dl class="mb-4 grid grid-cols-2 gap-x-4 gap-y-2 text-xs sm:grid-cols-3">
        <div v-for="item in summary" :key="item.label" class="min-w-0">
          <dt class="text-muted-foreground">{{ item.label }}</dt>
          <dd class="mt-0.5 truncate font-medium" :title="item.value">{{ item.value }}</dd>
        </div>
      </dl>

      <!-- span 时间线 -->
      <h4 class="mb-2 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
        {{ t('pages.traces.detail.timeline') }}
      </h4>
      <ol class="relative space-y-3 border-l pl-5">
        <li v-for="(span, idx) in spans" :key="span.id" class="relative">
          <span
            class="absolute top-1 -left-[25px] flex size-4 items-center justify-center rounded-full border-2 border-background"
            :class="span.spanKind === 'tool' ? 'bg-amber-500' : 'bg-primary'"
            :aria-hidden="true"
          />
          <div class="rounded-lg border bg-card px-3 py-2.5">
            <div class="flex flex-wrap items-center gap-2 text-xs">
              <Badge :variant="span.status === 'ok' ? 'default' : 'destructive'" class="font-sans">
                {{ span.status === 'ok' ? t('pages.traces.statusOk') : t('pages.traces.statusError') }}
              </Badge>
              <Badge v-if="span.spanKind === 'tool'" variant="outline" class="font-sans">
                {{ t('pages.traces.detail.toolSpan') }}
              </Badge>
              <span v-else class="font-medium">{{ span.name || span.model }}</span>
              <span class="ml-auto text-muted-foreground">#{{ idx + 1 }} · {{ span.durationMs }} ms</span>
            </div>
            <div v-if="span.spanKind === 'tool'" class="mt-1 text-xs font-medium">
              {{ span.name }}
            </div>
            <div class="mt-1 flex flex-wrap gap-x-3 gap-y-0.5 text-muted-foreground">
              <span v-if="span.spanKind !== 'tool'">{{ formatTime(span.createdAt) }}</span>
              <span v-if="span.inputTokens || span.outputTokens">
                ↑{{ span.inputTokens }} / ↓{{ span.outputTokens }} tok
              </span>
              <span v-if="span.stream">{{ t('pages.traces.streamYes') }}</span>
            </div>

            <!-- 内容观测折叠区 -->
            <details class="group mt-2">
              <summary
                class="cursor-pointer list-none text-xs font-medium text-primary select-none hover:underline"
              >
                {{ t('pages.traces.detail.viewIO') }}
              </summary>
              <div class="mt-2 space-y-2">
                <TraceIOBlock
                  v-if="span.inputJson"
                  :label="t(span.spanKind === 'tool' ? 'pages.traces.detail.arguments' : 'pages.traces.detail.input')"
                  :json="span.inputJson"
                />
                <TraceIOBlock
                  v-if="span.spanKind !== 'tool' && span.toolsJson"
                  :label="t('pages.traces.detail.tools')"
                  :json="span.toolsJson"
                />
                <TraceIOBlock
                  v-if="span.outputJson"
                  :label="t(span.spanKind === 'tool' ? 'pages.traces.detail.result' : 'pages.traces.detail.output')"
                  :json="span.outputJson"
                />
                <p v-if="span.errorMessage" class="rounded-md bg-destructive/10 px-2 py-1.5 font-mono text-xs text-destructive break-all">
                  {{ span.errorMessage }}
                </p>
              </div>
            </details>
          </div>
        </li>
      </ol>
    </SbAsyncRegion>
  </SbModal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Badge } from '@/components/ui/badge'
import SbAsyncRegion from '../SbAsyncRegion.vue'
import SbModal from '../modal/SbModal.vue'
import { api } from '../../services/api'
import type { LLMTrace } from '../../services/api'
import { useLoadState } from '../../composables/useLoadState'
import { useProjectStore } from '../../stores/project'
import TraceIOBlock from './TraceIOBlock.vue'
import { formatTime } from '../../utils/format'

/**
 * Trace 详情 Modal（planv6.5 二期 §9.4，改为通用弹窗）：
 * 弹窗展示一个 trace 的全部 span 时间线（llm 蓝 / tool 琥珀），
 * 每个 span 的输入消息、输出正文、工具声明、错误均可折叠查看。
 */
const props = defineProps<{ traceId?: string }>()
const emit = defineEmits<{ close: [] }>()

const { t } = useI18n()
const projectStore = useProjectStore()
const open = computed(() => Boolean(props.traceId))

function onOpenChange(v: boolean) {
  if (!v) emit('close')
}

const spans = ref<LLMTrace[]>([])
const { pending, showSkeleton, showEmpty, showError, refreshing, error, run, reset } = useLoadState({
  fallback: 'pages.traces.loadFailed'
})

const summary = computed(() => {
  if (spans.value.length === 0) return []
  const first = spans.value[0]
  const last = spans.value[spans.value.length - 1]
  const tokens = spans.value.reduce(
    (acc, s) => ({ in: acc.in + s.inputTokens, out: acc.out + s.outputTokens }),
    { in: 0, out: 0 }
  )
  const hasError = spans.value.some((s) => s.status === 'error')
  return [
    { label: t('pages.traces.detail.startedAt'), value: formatTime(first.createdAt) },
    { label: t('pages.traces.detail.status'), value: hasError ? t('pages.traces.statusError') : t('pages.traces.statusOk') },
    { label: t('pages.traces.detail.tokens'), value: `↑${tokens.in} / ↓${tokens.out}` },
    { label: t('pages.traces.detail.spans'), value: String(spans.value.length) },
    { label: t('pages.traces.detail.duration'), value: `${last.createdAt && first.createdAt ? spanWallMs(first, last) : 0} ms` },
    { label: 'Request ID', value: first.requestId || '—' }
  ]
})

function spanWallMs(first: LLMTrace, last: LLMTrace): number {
  const a = new Date(first.createdAt).getTime()
  const b = new Date(last.createdAt).getTime()
  if (Number.isNaN(a) || Number.isNaN(b)) return 0
  return Math.max(0, b - a)
}

function load() {
  const id = props.traceId
  if (!id || !projectStore.id) return Promise.resolve()
  return run(
    async () => {
      spans.value = await api.llmTraces.detail(projectStore.id, id)
      return spans.value.length > 0
    },
    { replace: true }
  )
}

watch(
  () => props.traceId,
  (id) => {
    spans.value = []
    if (!id) {
      reset()
      return
    }
    void load()
  },
  { immediate: true }
)
</script>
