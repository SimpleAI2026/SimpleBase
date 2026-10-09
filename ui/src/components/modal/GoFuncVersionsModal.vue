<template>
  <SbModal
    :open="open"
    :title="`版本 · ${record?.file || ''}`"
    hide-footer
    :max-width="520"
    @update:open="emit('update:open', $event)"
  >
    <SbAsyncRegion
      class="flex max-h-[420px] min-w-0 max-w-full flex-col gap-2 overflow-y-auto"
      block="lines"
      :pending="pending"
      :show-skeleton="showSkeleton"
      :show-empty="showEmpty"
      :show-error="showError"
      :refreshing="refreshing"
      :error="error"
      @retry="load"
    >
      <template #empty>
        <p class="py-6 text-center text-sm text-muted-foreground">暂无版本</p>
      </template>
      <div v-for="v in versions" :key="v.version" class="min-w-0 max-w-full rounded-lg border border-border/60 p-3">
        <div class="flex flex-wrap items-center gap-2">
          <span class="sb-mono font-semibold">v{{ v.version }}</span>
          <Badge v-if="v.active" variant="success">● 生效</Badge>
          <span class="text-xs text-muted-foreground">{{ formatTime(v.createdAt) }}</span>
          <span v-if="v.note" class="text-xs text-muted-foreground">· {{ v.note }}</span>
        </div>
        <div class="sb-code-view mt-1 max-w-full min-w-0 whitespace-pre-wrap [word-break:break-word] [overflow-wrap:anywhere] text-xs text-muted-foreground">导出：{{ v.exports.join('、') || '—' }}</div>
        <div class="mt-2 flex gap-1">
          <Button
            v-if="!v.active && !projectStore.isAdmin"
            variant="outline"
            size="sm"
            :disabled="activating"
            @click="activate(v.version)"
          >
            <Spinner v-if="activating" data-icon="inline-start" />
            设为生效
          </Button>
          <Button variant="ghost" size="sm" @click="viewSource(v)">复制源码</Button>
        </div>
      </div>
    </SbAsyncRegion>
    <template #footer>
      <Button variant="outline" @click="emit('update:open', false)">关闭</Button>
    </template>
  </SbModal>
</template>
<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import SbModal from './SbModal.vue'
import SbAsyncRegion from '../SbAsyncRegion.vue'
import { useLoadState } from '@/composables/useLoadState'
import { api } from '../../services/api'
import type { GoFunctionItem, GoFuncVersionSummary } from '../../services/types'
import { useProjectStore } from '../../stores/project'
import { formatTime } from '../../utils/format'

const props = defineProps<{
  open: boolean
  record: GoFunctionItem | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  changed: []
}>()

const projectStore = useProjectStore()
const versions = ref<GoFuncVersionSummary[]>([])
const activating = ref(false)
const { pending, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({
  fallback: '加载版本失败'
})

function load() {
  const record = props.record
  /* v8 ignore next */
  if (!record) return Promise.resolve()
  return run(async () => {
    const res = await api.gofunctions.listVersions(projectStore.id, record.name)
    versions.value = res.versions
    return res.versions.length > 0
  })
}

async function activate(version: number) {
  /* v8 ignore next */
  if (!props.record) return
  activating.value = true
  try {
    await api.gofunctions.activate(projectStore.id, props.record.name, version)
    toast.success(`已设为生效 v${version}`)
    await load()
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e, '操作失败'))
  } finally {
    activating.value = false
  }
}

async function viewSource(v: GoFuncVersionSummary) {
  /* v8 ignore next */
  if (!props.record) return
  try {
    const item = await api.gofunctions.get(projectStore.id, props.record.name)
    const hit = (item.versions || []).find((x) => x.version === v.version)
    const src = hit?.source ?? item.source ?? ''
    await navigator.clipboard.writeText(src)
    toast.success(`v${v.version} 源码已复制`)
  } catch (e) {
    toast.error(errorMessage(e, '读取源码失败'))
  }
}

watch(
  () => [props.open, props.record?.id] as const,
  ([open]) => {
    if (open) void load()
  },
  { immediate: true }
)
</script>
