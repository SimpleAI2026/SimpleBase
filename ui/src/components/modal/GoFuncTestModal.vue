<template>
  <SbModal
    :open="open"
    :title="t('components.goFuncTest.title', { file: record?.file || '' })"
    :description="t('components.goFuncTest.descriptionTop')"
    hide-footer
    :max-width="960"
    @update:open="emit('update:open', $event)"
  >
    <div class="flex min-h-[520px] min-w-0 max-w-full flex-col overflow-hidden rounded-xl border border-border/60 bg-card lg:flex-row">
      <!-- 左栏：版本 + 导出函数 -->
      <aside class="flex w-full shrink-0 flex-col gap-4 border-b bg-card p-4 lg:w-60 lg:border-r lg:border-b-0">
        <div class="flex flex-col gap-2">
          <div class="text-xs font-semibold tracking-wider text-muted-foreground/70 uppercase">{{ t('components.goFuncTest.versions') }}</div>
          <SbBlockSkeleton v-if="versionsLoading" variant="lines" :count="3" />
          <div v-else class="flex max-h-40 flex-col gap-1 overflow-y-auto">
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
              <span v-if="v.active" class="ml-auto text-[10px] font-medium text-success">{{ t('components.goFuncTest.active') }}</span>
            </button>
          </div>
        </div>

        <div class="flex flex-col gap-2">
          <div class="text-xs font-semibold tracking-wider text-muted-foreground/70 uppercase">
            {{ t('components.goFuncTest.exports', { version: selectedVersion }) }}
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
            <p v-if="!currentExports.length" class="text-xs text-muted-foreground">{{ t('components.goFuncTest.noExports') }}</p>
          </div>
        </div>

        <p class="mt-auto text-[11px] leading-relaxed text-muted-foreground/80">
          {{ t('components.goFuncTest.protocolHint') }}
        </p>
      </aside>

      <!-- 右栏：请求 / 响应 -->
      <section class="flex min-w-0 flex-1 flex-col gap-5 p-4 lg:p-5">
        <div class="flex flex-col gap-2">
          <div class="text-xs font-semibold tracking-wider text-muted-foreground/70 uppercase">{{ t('components.goFuncTest.request') }}</div>
          <div class="flex items-center gap-2">
            <Badge variant="secondary" class="shrink-0">POST</Badge>
            <code class="sb-mono min-w-0 flex-1 truncate rounded-lg border bg-card px-2.5 py-1.5 text-xs">
              {{ invokeUrl }}
            </code>
            <Button variant="outline" size="sm" class="shrink-0" @click="copyUrl">{{ t('common.copy') }}</Button>
          </div>
          <p class="text-[11px] text-muted-foreground">
            {{ t('components.goFuncTest.externalUrl') }}
          </p>
        </div>

        <div class="flex min-h-0 flex-1 flex-col gap-2">
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold tracking-wider text-muted-foreground/70 uppercase">Body (JSON)</span>
            <Button variant="ghost" size="sm" class="h-7 px-2 text-xs" @click="validateBody">{{ t('components.goFuncTest.validate') }}</Button>
          </div>
          <textarea
            v-model="bodyText"
            class="sb-code-view sb-mono min-h-[160px] w-full min-w-0 max-w-full flex-1 resize-y whitespace-pre-wrap [word-break:break-word] [overflow-wrap:anywhere] rounded-lg border border-border bg-card p-3 text-xs leading-relaxed outline-none focus-visible:ring-2 focus-visible:ring-ring"
            spellcheck="false"
          />
          <p v-if="bodyError" class="text-xs text-destructive">{{ bodyError }}</p>
        </div>

        <div class="flex flex-wrap items-center gap-3 border-t border-border/60 pt-4">
          <Button class="min-w-24" :disabled="sending || !selectedFn" @click="send">
            <Spinner v-if="sending" data-icon="inline-start" />
            {{ t('components.goFuncTest.send') }}
          </Button>
          <span v-if="result" class="text-xs text-muted-foreground">
            {{ t('components.goFuncTest.resultMeta', { duration: result.durationMs, version: result.version }) }}
          </span>
        </div>

        <div class="flex min-h-0 flex-col gap-2">
          <div class="flex items-center gap-2">
            <span class="text-xs font-semibold tracking-wider text-muted-foreground/70 uppercase">{{ t('components.goFuncTest.response') }}</span>
            <template v-if="result">
              <Badge :variant="statusVariant" class="px-1.5 text-[11px]">{{ result.statusCode }}</Badge>
              <span class="text-xs text-muted-foreground">application/json</span>
            </template>
          </div>
          <Alert v-if="result && !result.ok" variant="destructive">
            <AlertTitle>{{ t('components.goFuncTest.runFailedTitle') }}</AlertTitle>
            <AlertDescription class="sb-code-view max-w-full min-w-0 whitespace-pre-wrap [word-break:break-word] [overflow-wrap:anywhere]">{{ result.error }}</AlertDescription>
          </Alert>
          <pre
            v-if="result"
            class="sb-code-view sb-mono max-h-56 max-w-full min-w-0 overflow-auto whitespace-pre-wrap [word-break:break-word] [overflow-wrap:anywhere] rounded-lg border bg-card p-3 text-xs leading-relaxed"
          >{{ prettyData }}</pre>
          <p v-else class="rounded-lg border border-dashed border-border/60 px-3 py-6 text-center text-xs text-muted-foreground">
            {{ t('components.goFuncTest.notSent') }}
          </p>
        </div>
      </section>
    </div>
  </SbModal>
</template>
<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import SbBlockSkeleton from '@/components/SbBlockSkeleton.vue'
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

const { t } = useI18n()
const projectStore = useProjectStore()
const versions = ref<GoFuncVersionSummary[]>([])
const versionsLoading = ref(false)
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
    toast.success(t('components.goFuncTest.jsonValid'))
  } catch (e) {
    bodyError.value = errorMessage(e, t('components.goFuncTest.jsonInvalid'))
  }
}

async function copyUrl() {
  try {
    await navigator.clipboard.writeText(invokeUrl.value)
    toast.success(t('components.goFuncTest.copiedPath'))
  } catch {
    /* v8 ignore next */
    toast.error(t('components.goFuncTest.copyFailed'))
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
    bodyError.value = errorMessage(e, t('components.goFuncTest.jsonInvalid'))
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
    toast.error(errorMessage(e, t('components.goFuncTest.testFailed')))
  } finally {
    sending.value = false
  }
}

watch(
  () => [props.open, props.record?.id] as const,
  async ([open]) => {
    /* v8 ignore next -- 弹窗关闭/无记录 */
    if (!open || !props.record) {
      versionsLoading.value = false
      return
    }
    const record = props.record
    versionsLoading.value = true
    result.value = null
    bodyError.value = ''
    try {
      const res = await api.gofunctions.listVersions(projectStore.id, record.name)
      versions.value = res.versions
      selectedVersion.value = res.activeVersion || res.versions[0]?.version || 0
    } catch {
      /* v8 ignore start -- 列表失败回退 record.versions */
      versions.value = record.versions || []
      selectedVersion.value =
        versions.value.find((x) => x.active)?.version ||
        versions.value[0]?.version ||
        record.activeVersion ||
        0
      /* v8 ignore stop */
    } finally {
      versionsLoading.value = false
    }
    selectedFn.value = currentExports.value[0] || ''
    loadDraft()
  },
  { immediate: true }
)
</script>
