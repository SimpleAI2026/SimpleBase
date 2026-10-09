<template>
  <div class="flex flex-col gap-3">
    <SbAsyncRegion
      class="flex flex-col gap-3"
      block="spinner"
      :pending="pending"
      :show-skeleton="showSkeleton"
      :show-empty="false"
      :show-error="showError"
      :refreshing="refreshing"
      :error="error"
      @retry="load"
    >
      <div class="overflow-x-auto rounded-md border border-border/70">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="w-14">#</TableHead>
              <TableHead class="min-w-40 text-left">元素</TableHead>
              <TableHead v-if="!readonly" class="w-20">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableEmpty v-if="!elems.length" :colspan="readonly ? 2 : 3">列表为空</TableEmpty>
            <TableRow v-for="(el, i) in elems" :key="i">
              <TableCell class="text-xs text-muted-foreground tabular-nums">{{ i }}</TableCell>
              <TableCell class="text-left">
                <span v-if="editIndex !== i" class="sb-mono text-xs break-all">{{ el }}</span>
                <div v-else class="flex items-center gap-1">
                  <Input v-model="editValue" class="h-7 text-xs" @keydown.enter="saveEdit(i)" />
                  <Button size="sm" variant="ghost" @click="saveEdit(i)">存</Button>
                </div>
              </TableCell>
              <TableCell v-if="!readonly">
                <Button size="sm" variant="ghost" @click="startEdit(i, el)">改</Button>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>
      <div v-if="!readonly" class="flex flex-wrap items-center gap-2">
        <Input v-model="pushValue" placeholder="新元素" class="w-48" @keydown.enter="push('back')" />
        <Button size="sm" variant="outline" :disabled="!pushValue" @click="push('front')">头插</Button>
        <Button size="sm" variant="outline" :disabled="!pushValue" @click="push('back')">尾插</Button>
        <div class="ml-auto flex items-center gap-1">
          <Button size="sm" variant="ghost" :disabled="!elems.length" @click="pop('front')">弹头</Button>
          <Button size="sm" variant="ghost" :disabled="!elems.length" @click="pop('back')">弹尾</Button>
        </div>
      </div>
    </SbAsyncRegion>
  </div>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableEmpty, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { api } from '../../../../services/api'
import SbAsyncRegion from '../../../SbAsyncRegion.vue'
import { useLoadState } from '@/composables/useLoadState'

const props = defineProps<{
  projectId: string
  kvKey: string
  readonly?: boolean
}>()

const emit = defineEmits<{ changed: [] }>()

const elems = ref<string[]>([])
const pushValue = ref('')
const { pending, showSkeleton, showError, refreshing, error, run } = useLoadState({ fallback: '加载失败' })
const editIndex = ref(-1)
const editValue = ref('')

function load() {
  return run(async () => {
    // v1 展示前 200 个元素
    const r = (await api.kv.exec(props.projectId, {
      type: 'cmd',
      argvs: ['LRANGE', props.kvKey, '0', '199']
    })) as string[]
    elems.value = Array.isArray(r) ? r : []
    return true
  })
}

async function push(side: 'front' | 'back') {
  if (!pushValue.value) return
  try {
    await api.kv.exec(props.projectId, {
      type: 'List',
      args: { key: props.kvKey, elems: [pushValue.value], side }
    })
    pushValue.value = ''
    await load()
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e, '插入失败'))
  }
}

async function pop(side: 'front' | 'back') {
  try {
    const r = await api.kv.exec(props.projectId, {
      type: 'cmd',
      argvs: [side === 'front' ? 'LPOP' : 'RPOP', props.kvKey]
    })
    toast.success('已弹出: ' + (r ?? ''))
    await load()
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e, '弹出失败'))
  }
}

function startEdit(i: number, el: string) {
  editIndex.value = i
  editValue.value = el
}

async function saveEdit(i: number) {
  try {
    await api.kv.exec(props.projectId, {
      type: 'cmd',
      argvs: ['LSET', props.kvKey, String(i), editValue.value]
    })
    editIndex.value = -1
    await load()
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e, '保存失败'))
  }
}

watch(() => props.kvKey, () => void load(), { immediate: true })
</script>
