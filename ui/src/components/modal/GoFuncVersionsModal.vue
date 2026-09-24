<template>
  <SbModal
    :open="open"
    :title="`版本 · ${record?.file || ''}`"
    hide-footer
    :max-width="520"
    @update:open="emit('update:open', $event)"
  >
    <div class="flex max-h-[420px] flex-col gap-2 overflow-y-auto">
      <div v-for="v in versions" :key="v.version" class="rounded-lg border border-border/60 p-3">
        <div class="flex flex-wrap items-center gap-2">
          <span class="sb-mono font-semibold">v{{ v.version }}</span>
          <Badge v-if="v.active" variant="success">● 生效</Badge>
          <span class="text-xs text-muted-foreground">{{ formatTime(v.createdAt) }}</span>
          <span v-if="v.note" class="text-xs text-muted-foreground">· {{ v.note }}</span>
        </div>
        <div class="mt-1 text-xs text-muted-foreground">导出：{{ v.exports.join('、') || '—' }}</div>
        <div class="mt-2 flex gap-1">
          <Button
            v-if="!v.active && !projectStore.isAdmin"
            variant="outline"
            size="sm"
            :disabled="activating"
            @click="activate(v.version)"
          >
            设为生效
          </Button>
          <Button variant="ghost" size="sm" @click="viewSource(v)">复制源码</Button>
        </div>
      </div>
      <p v-if="!versions.length" class="py-6 text-center text-sm text-muted-foreground">暂无版本</p>
    </div>
    <template #footer>
      <Button variant="outline" @click="emit('update:open', false)">关闭</Button>
    </template>
  </SbModal>
</template>
<script setup lang="ts">
import { ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import SbModal from './SbModal.vue'
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

async function load() {
  /* v8 ignore next */
  if (!props.record) return
  try {
    const res = await api.gofunctions.listVersions(projectStore.id, props.record.name)
    versions.value = res.versions
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '加载版本失败')
  }
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
    toast.error(e instanceof Error ? e.message : '操作失败')
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
    toast.error(e instanceof Error ? e.message : '读取源码失败')
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
