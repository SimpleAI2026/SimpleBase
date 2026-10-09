<template>
  <SbModal
    :open="open"
    :title="kvKey?.key || 'Key 详情'"
    :width="720"
    hide-footer
    @update:open="emit('update:open', $event)"
  >
    <div
      v-if="metaPending || meta"
      class="mb-3 flex min-h-6 items-center gap-2 text-sm text-muted-foreground"
    >
      <Skeleton v-if="metaPending" class="h-5 w-40" aria-hidden="true" />
      <template v-else-if="meta">
        <Badge variant="secondary">{{ meta.type }}</Badge>
        <span v-if="meta.len != null" class="text-xs">{{ meta.len }} 个元素</span>
        <span class="text-xs">{{ meta.ttl_ms == null ? '永久' : 'TTL ' + Math.ceil(meta.ttl_ms / 1000) + 's' }}</span>
      </template>
    </div>

    <div class="max-h-[60vh] overflow-y-auto pr-1">
      <div v-if="!kvKey" class="py-6 text-center text-sm text-muted-foreground">未选择 Key</div>
      <component
        :is="editorComponent"
        v-else-if="editorComponent"
        :project-id="projectId"
        :kv-key="kvKey.key"
        :readonly="readonly"
        @changed="onChanged"
      />
    </div>

    <div v-if="kvKey && !readonly" class="mt-4 flex justify-end border-t pt-3">
      <ConfirmAction title="确认删除该 Key？此操作不可恢复。" @confirm="removeKey">
        <Button variant="destructiveGhost" size="sm">删除 Key</Button>
      </ConfirmAction>
    </div>
  </SbModal>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { computed, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { api } from '../../../services/api'
import type { KvKeyMeta } from '../../../services/api'
import ConfirmAction from '../../ConfirmAction.vue'
import SbModal from '../../modal/SbModal.vue'
import KvStringEditor from './editors/KvStringEditor.vue'
import KvHashEditor from './editors/KvHashEditor.vue'
import KvListEditor from './editors/KvListEditor.vue'
import KvSetEditor from './editors/KvSetEditor.vue'
import KvZSetEditor from './editors/KvZSetEditor.vue'

const props = defineProps<{
  open: boolean
  projectId: string
  kvKey: KvKeyMeta | null
  readonly?: boolean
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
  changed: []
  deleted: []
}>()

const meta = ref<KvKeyMeta | null>(null)
const metaPending = ref(false)
let metaRequest = 0

const editorComponent = computed(() => {
  switch (props.kvKey?.type) {
    case 'string':
      return KvStringEditor
    case 'hash':
      return KvHashEditor
    case 'list':
      return KvListEditor
    case 'set':
      return KvSetEditor
    case 'zset':
      return KvZSetEditor
    default:
      return null
  }
})

/** 用 TYPE / PTTL / 长度命令重建列表行元数据 */
async function refreshMeta() {
  if (!props.open || !props.kvKey) {
    metaRequest += 1
    metaPending.value = false
    if (!props.kvKey) meta.value = null
    return
  }
  const request = ++metaRequest
  const current = props.kvKey
  const key = current.key
  metaPending.value = true
  try {
    const [type, pttl, lenCmd] = await api.kv.execBatch(props.projectId, [
      { type: 'cmd', argvs: ['TYPE', key] },
      { type: 'cmd', argvs: ['PTTL', key] },
      { type: 'cmd', argvs: lenArgv(current.type, key) }
    ])
    if (request !== metaRequest) return
    const len = typeof lenCmd === 'number' ? lenCmd : null
    meta.value = {
      key,
      type: (typeof type === 'string' ? type : current.type) as KvKeyMeta['type'],
      len: current.type === 'string' ? null : (len ?? 0),
      ttl_ms: typeof pttl === 'number' && pttl >= 0 ? pttl : null,
      mtime_ms: current.mtime_ms,
      version: current.version
    }
  } catch {
    if (request !== metaRequest) return
    meta.value = current
  } finally {
    if (request === metaRequest) metaPending.value = false
  }
}

/** 长度命令按类型选择；string 无长度概念返回占位 */
function lenArgv(type: KvKeyMeta['type'], key: string): string[] {
  switch (type) {
    case 'hash':
      return ['HLEN', key]
    case 'list':
      return ['LLEN', key]
    case 'set':
      return ['SCARD', key]
    case 'zset':
      return ['ZCARD', key]
    default:
      return ['DBSIZE'] // string：占位，不使用其返回值
  }
}

watch(
  () => [props.open, props.kvKey?.key],
  () => void refreshMeta(),
  { immediate: true }
)

function onChanged() {
  void refreshMeta()
  emit('changed')
}

async function removeKey() {
  if (!props.kvKey) return
  try {
    await api.kv.exec(props.projectId, { type: 'cmd', argvs: ['DEL', props.kvKey.key] })
    toast.success('已删除')
    emit('update:open', false)
    emit('deleted')
  } catch (e) {
    toast.error(errorMessage(e, '删除失败'))
  }
}
</script>
