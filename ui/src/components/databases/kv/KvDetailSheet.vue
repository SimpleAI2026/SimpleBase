<template>
  <Sheet :open="open" @update:open="emit('update:open', $event)">
    <SheetContent side="right" class="w-full overflow-y-auto sm:max-w-xl">
      <SheetHeader class="border-b">
        <SheetTitle class="sb-mono break-all pr-6">{{ kvKey?.key || 'Key 详情' }}</SheetTitle>
        <SheetDescription v-if="meta" class="flex items-center gap-2">
          <Badge variant="secondary">{{ meta.type }}</Badge>
          <span v-if="meta.len != null" class="text-xs">{{ meta.len }} 个元素</span>
          <span class="text-xs">{{ meta.ttl_ms == null ? '永久' : 'TTL ' + Math.ceil(meta.ttl_ms / 1000) + 's' }}</span>
        </SheetDescription>
      </SheetHeader>

      <div class="flex-1 px-4 py-4">
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

      <SheetFooter v-if="kvKey && !readonly" class="border-t">
        <ConfirmAction title="确认删除该 Key？此操作不可恢复。" @confirm="removeKey">
          <Button variant="destructiveGhost" size="sm">删除 Key</Button>
        </ConfirmAction>
      </SheetFooter>
    </SheetContent>
  </Sheet>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle
} from '@/components/ui/sheet'
import { api } from '../../../services/api'
import type { KvKeyMeta } from '../../../services/api'
import ConfirmAction from '../../ConfirmAction.vue'
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
  if (!props.open || !props.kvKey) return
  const key = props.kvKey.key
  try {
    const [type, pttl, lenCmd] = await api.kv.execBatch(props.projectId, [
      { type: 'cmd', argvs: ['TYPE', key] },
      { type: 'cmd', argvs: ['PTTL', key] },
      { type: 'cmd', argvs: lenArgv(props.kvKey.type, key) }
    ])
    const len = typeof lenCmd === 'number' ? lenCmd : null
    meta.value = {
      key,
      type: (typeof type === 'string' ? type : props.kvKey.type) as KvKeyMeta['type'],
      len: props.kvKey.type === 'string' ? null : (len ?? 0),
      ttl_ms: typeof pttl === 'number' && pttl >= 0 ? pttl : null,
      mtime_ms: props.kvKey.mtime_ms,
      version: props.kvKey.version
    }
  } catch {
    meta.value = props.kvKey
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
    toast.error(e instanceof Error ? e.message : '删除失败')
  }
}
</script>
