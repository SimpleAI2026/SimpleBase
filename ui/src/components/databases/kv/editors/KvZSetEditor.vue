<template>
  <div class="flex flex-col gap-3">
    <div class="flex items-center gap-2">
      <Button
        size="sm"
        :variant="desc ? 'outline' : 'default'"
        @click="desc = false"
      >分数升序</Button>
      <Button
        size="sm"
        :variant="desc ? 'default' : 'outline'"
        @click="desc = true"
      >分数降序</Button>
    </div>
    <div v-if="loading" class="flex justify-center py-6"><Spinner /></div>
    <template v-else>
      <div class="overflow-x-auto rounded-md border border-border/70">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="min-w-32">成员</TableHead>
              <TableHead class="w-28 text-right">分数</TableHead>
              <TableHead v-if="!readonly" class="w-32 text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableEmpty v-if="!items.length" :colspan="readonly ? 2 : 3">暂无成员</TableEmpty>
            <TableRow v-for="it in items" :key="it.elem">
              <TableCell class="sb-mono font-medium">{{ it.elem }}</TableCell>
              <TableCell>
                <div v-if="editElem === it.elem" class="flex items-center justify-end gap-1">
                  <Input v-model="editScore" type="number" class="h-7 w-24 text-xs" @keydown.enter="saveScore(it.elem)" />
                  <Button size="sm" variant="ghost" @click="saveScore(it.elem)">存</Button>
                </div>
                <span v-else class="block text-right tabular-nums">{{ it.score }}</span>
              </TableCell>
              <TableCell v-if="!readonly" class="text-right">
                <div class="flex justify-end gap-1">
                  <Button size="sm" variant="ghost" @click="startEdit(it)">改分</Button>
                  <ConfirmAction title="移除该成员？" @confirm="removeMember(it.elem)">
                    <Button size="sm" variant="destructiveGhost">删</Button>
                  </ConfirmAction>
                </div>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>
      <div v-if="!readonly" class="flex items-end gap-2">
        <Input v-model="newElem" placeholder="成员" class="w-36" />
        <Input v-model="newScore" type="number" placeholder="分数" class="w-28" />
        <Button size="sm" :disabled="!newElem.trim() || newScore === ''" @click="addMember">添加</Button>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Table, TableBody, TableCell, TableEmpty, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { api } from '../../../../services/api'
import ConfirmAction from '../../../ConfirmAction.vue'

const props = defineProps<{
  projectId: string
  kvKey: string
  readonly?: boolean
}>()

const emit = defineEmits<{ changed: [] }>()

const items = ref<{ elem: string; score: number }[]>([])
const loading = ref(false)
const desc = ref(false)
const newElem = ref('')
const newScore = ref('')
const editElem = ref('')
const editScore = ref('')

async function load() {
  loading.value = true
  try {
    // ZRANGE WITHSCORES：member/score 交替扁平数组
    const argv = ['ZRANGE', props.kvKey, '0', '199', 'WITHSCORES']
    if (desc.value) argv.push('REV')
    const flat = (await api.kv.exec(props.projectId, { type: 'cmd', argvs: argv })) as (string | number)[]
    const out: { elem: string; score: number }[] = []
    if (Array.isArray(flat)) {
      for (let i = 0; i + 1 < flat.length; i += 2) {
        out.push({ elem: String(flat[i]), score: Number(flat[i + 1]) })
      }
    }
    items.value = out
  } catch (e) {
    toast.error(errorMessage(e, '加载失败'))
  } finally {
    loading.value = false
  }
}

async function addMember() {
  const v = newElem.value.trim()
  const s = Number(newScore.value)
  if (!v || !Number.isFinite(s)) return
  try {
    await api.kv.exec(props.projectId, {
      type: 'ZSet',
      args: { key: props.kvKey, items: [{ elem: v, score: s }] }
    })
    newElem.value = ''
    newScore.value = ''
    await load()
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e, '添加失败'))
  }
}

function startEdit(it: { elem: string; score: number }) {
  editElem.value = it.elem
  editScore.value = String(it.score)
}

async function saveScore(elem: string) {
  const s = Number(editScore.value)
  if (!Number.isFinite(s)) return
  try {
    await api.kv.exec(props.projectId, {
      type: 'ZSet',
      args: { key: props.kvKey, items: [{ elem, score: s }] }
    })
    editElem.value = ''
    await load()
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e, '保存失败'))
  }
}

async function removeMember(elem: string) {
  try {
    await api.kv.exec(props.projectId, { type: 'cmd', argvs: ['ZREM', props.kvKey, elem] })
    await load()
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e, '删除失败'))
  }
}

watch(desc, () => void load())
watch(() => props.kvKey, () => void load(), { immediate: true })
</script>
