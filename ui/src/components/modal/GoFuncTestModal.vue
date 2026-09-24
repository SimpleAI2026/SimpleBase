<template>
  <SbModal
    :open="open"
    :title="`测试 · ${record?.file || ''}`"
    :description="'左栏选择「版本 / 导出函数」；试跑不会改变生效版本。'"
    hide-footer
    :max-width="960"
    @update:open="emit('update:open', $event)"
  >
    <div class="flex min-h-[520px] flex-col overflow-hidden rounded-xl border border-border/60 bg-card lg:flex-row">
      <!-- 左栏：版本 + 导出函数 -->
      <aside class="flex w-full shrink-0 flex-col gap-4 border-b bg-card p-4 lg:w-60 lg:border-r lg:border-b-0">
        <div class="flex flex-col gap-2">
          <div class="text-xs font-semibold tracking-wider text-muted-foreground/70 uppercase">版本</div>
          <div class="flex max-h-40 flex-col gap-1 overflow-y-auto">
            <button
              v-for="v in versions"
              :key="v.version"
              type="button"
              class="flex items-center gap-2 rounded-lg px-2.5 py-2 text-left text-sm transition-colors"
              :class="v.version === selectedVersion ? 'bg-accent text-accent-foreground shadow-xs' : 'hover:bg-muted/60'"
              @click="selectVersion(v.version)"
            >
              <span
                class="size-1.5 shrink-0 rounded-full"
                :class="v.active ? 'bg-success' : 'bg-border'"
              />
              <span class="font-mono">v{{ v.version }}</span>
              <span v-if="v.active" class="ml-auto text-[10px] font-medium text-success">生效</span>
            </button>
          </div>
        </div>

        <div class="flex flex-col gap-2">
          <div class="text-xs font-semibold tracking-wider text-muted-foreground/70 uppercase">
            导出函数（v{{ selectedVersion }}）
          </div>
          <div class="flex flex-col gap-1">
            <button
              v-for="fn in currentExports"
              :key="fn"
              type="button"
              class="rounded-lg px-2.5 py-2 text-left text-sm font-medium transition-colors"
              :class="fn === selectedFn ? 'bg-accent text-accent-foreground shadow-xs' : 'hover:bg-muted/60'"
              @click="selectFn(fn)"
            >
              {{ fn }}
            </button>
            <p v-if="!currentExports.length" class="text-xs text-muted-foreground">该版本无导出函数</p>
          </div>
        </div>

        <p class="mt-auto text-[11px] leading-relaxed text-muted-foreground/80">
          POST+JSON，1 参 1 返回。发送走管理端 test API（当前登录态）。
        </p>
      </aside>

      <!-- 右栏：请求 / 响应 -->
      <section class="flex min-w-0 flex-1 flex-col gap-5 p-4 lg:p-5">
        <div class="flex flex-col gap-2">
          <div class="text-xs font-semibold tracking-wider text-muted-foreground/70 uppercase">请求</div>
          <div class="flex items-center gap-2">
            <Badge variant="secondary" class="shrink-0">POST</Badge>
            <code class="sb-mono min-w-0 flex-1 truncate rounded-lg border bg-card px-2.5 py-1.5 text-xs">
              {{ invokeUrl }}
            </code>
            <Button variant="outline" size="sm" class="shrink-0" @click="copyUrl">复制</Button>
          </div>
          <p class="text-[11px] text-muted-foreground">
            对外 URL · 调用方需 <code class="sb-mono">Authorization: Bearer</code> API Key
          </p>
        </div>

        <div class="flex min-h-0 flex-1 flex-col gap-2">
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold tracking-wider text-muted-foreground/70 uppercase">Body (JSON)</span>
            <Button variant="ghost" size="sm" class="h-7 px-2 text-xs" @click="validateBody">校验</Button>
          </div>
          <textarea
            v-model="bodyText"
            class="sb-mono min-h-[160px] flex-1 resize-y rounded-lg border border-border bg-card p-3 text-xs leading-relaxed outline-none focus-visible:ring-2 focus-visible:ring-ring"
            spellcheck="false"
          />
          <p v-if="bodyError" class="text-xs text-destructive">{{ bodyError }}</p>
        </div>

        <div class="flex flex-wrap items-center gap-3 border-t border-border/60 pt-4">
          <Button class="min-w-24" :disabled="sending || !selectedFn" @click="send">
            <Spinner v-if="sending" data-icon="inline-start" />
            发送请求
          </Button>
          <span v-if="result" class="text-xs text-muted-foreground">
            耗时 {{ result.durationMs }}ms · version={{ result.version }} · channel=test
          </span>
        </div>

        <div class="flex min-h-0 flex-col gap-2">
          <div class="flex items-center gap-2">
            <span class="text-xs font-semibold tracking-wider text-muted-foreground/70 uppercase">响应</span>
            <template v-if="result">
              <Badge :variant="statusVariant" class="px-1.5 text-[11px]">{{ result.statusCode }}</Badge>
              <span class="text-xs text-muted-foreground">application/json</span>
            </template>
          </div>
          <Alert v-if="result && !result.ok" variant="destructive">
            <AlertTitle>执行失败</AlertTitle>
            <AlertDescription>{{ result.error }}</AlertDescription>
          </Alert>
          <pre
            v-if="result"
            class="sb-mono max-h-56 overflow-auto rounded-lg border bg-card p-3 text-xs leading-relaxed whitespace-pre-wrap"
          >{{ prettyData }}</pre>
          <p v-else class="rounded-lg border border-dashed border-border/60 px-3 py-6 text-center text-xs text-muted-foreground">
            尚未发送请求
          </p>
        </div>
      </section>
    </div>
  </SbModal>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import SbModal from './SbModal.vue'
import { api } from '../../services/api'
import type { GoFuncTestResult, GoFunctionItem, GoFuncVersionSummary } from '../../services/types'
import { useProjectStore } from '../../stores/project'
import { formatJson } from '../../utils/format'

const props = defineProps<{
  open: boolean
  record: GoFunctionItem | null
}>()

const emit = defineEmits<{ 'update:open': [value: boolean] }>()

const projectStore = useProjectStore()
const versions = ref<GoFuncVersionSummary[]>([])
const selectedVersion = ref(0)
const selectedFn = ref('')
const bodyText = ref('{\n  \n}')
const bodyError = ref('')
const sending = ref(false)
const result = ref<GoFuncTestResult | null>(null)

const currentExports = computed(() => {
  const v = versions.value.find((x) => x.version === selectedVersion.value)
  return v?.exports || props.record?.exports || []
})

const invokeUrl = computed(() => {
  const pid = projectStore.id
  const name = props.record?.name || ''
  const fn = selectedFn.value || '{FunctionName}'
  return `/go/${pid}/${name}/${fn}`
})

const statusVariant = computed(() => {
  const s = result.value?.statusCode || 0
  if (s >= 500) return 'destructive' as const
  if (s >= 400) return 'warning' as const
  return 'success' as const
})

const prettyData = computed(() => {
  if (!result.value) return ''
  if (!result.value.ok) return ''
  return formatJson(result.value.data)
})

function draftKey() {
  return `sb_go_test_${projectStore.id}_${props.record?.name}_${selectedVersion.value}_${selectedFn.value}`
}

function selectVersion(v: number) {
  selectedVersion.value = v
  selectedFn.value = currentExports.value[0] || ''
  result.value = null
  loadDraft()
}

function selectFn(fn: string) {
  selectedFn.value = fn
  result.value = null
  loadDraft()
}

function loadDraft() {
  try {
    const raw = sessionStorage.getItem(draftKey())
    bodyText.value = raw || '{\n  \n}'
  } catch {
    /* ignore */
  }
}

function saveDraft() {
  try {
    sessionStorage.setItem(draftKey(), bodyText.value)
  } catch {
    /* ignore */
  }
}

function validateBody() {
  bodyError.value = ''
  try {
    JSON.parse(bodyText.value || '{}')
    saveDraft()
    toast.success('JSON 合法')
  } catch (e) {
    bodyError.value = e instanceof Error ? e.message : 'JSON 无效'
  }
}

async function copyUrl() {
  try {
    await navigator.clipboard.writeText(invokeUrl.value)
    toast.success('已复制调用路径')
  } catch {
    /* v8 ignore next */
    toast.error('复制失败')
  }
}

async function send() {
  /* v8 ignore next 2 -- 未选函数直接返回 */
  if (!props.record || !selectedFn.value) return
  bodyError.value = ''
  let payload: unknown = {}
  try {
    payload = JSON.parse(bodyText.value || '{}')
    saveDraft()
  } catch (e) {
    bodyError.value = e instanceof Error ? e.message : 'JSON 无效'
    return
  }
  sending.value = true
  try {
    result.value = await api.gofunctions.test(
      projectStore.id,
      props.record.name,
      selectedVersion.value,
      selectedFn.value,
      payload
    )
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '测试失败')
  } finally {
    sending.value = false
  }
}

watch(
  () => [props.open, props.record?.id] as const,
  async ([open]) => {
    /* v8 ignore next -- 弹窗关闭/无记录 */
    if (!open || !props.record) return
    result.value = null
    bodyError.value = ''
    try {
      const res = await api.gofunctions.listVersions(projectStore.id, props.record.name)
      versions.value = res.versions
      selectedVersion.value = res.activeVersion || res.versions[0]?.version || 0
    } catch {
      /* v8 ignore start -- 列表失败回退 record.versions */
      versions.value = props.record.versions || []
      selectedVersion.value =
        versions.value.find((x) => x.active)?.version ||
        versions.value[0]?.version ||
        props.record.activeVersion ||
        0
      /* v8 ignore stop */
    }
    selectedFn.value = currentExports.value[0] || ''
    loadDraft()
  },
  { immediate: true }
)
</script>
