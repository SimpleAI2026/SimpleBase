<template>
  <div class="flex flex-col gap-3">
    <div class="flex items-center gap-2">
      <Button
        size="sm"
        :variant="desc ? 'outline' : 'default'"
        @click="desc = false"
      >{{ t('components.kvZset.sortAsc') }}</Button>
      <Button
        size="sm"
        :variant="desc ? 'default' : 'outline'"
        @click="desc = true"
      >{{ t('components.kvZset.sortDesc') }}</Button>
    </div>
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
              <TableHead class="min-w-32 text-left">{{ t('components.kvZset.colMember') }}</TableHead>
              <TableHead class="w-28">{{ t('components.kvZset.colScore') }}</TableHead>
              <TableHead v-if="!readonly" class="w-32">{{ t('components.kvZset.colActions') }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableEmpty v-if="!items.length" :colspan="readonly ? 2 : 3">{{ t('components.kvZset.empty') }}</TableEmpty>
            <TableRow v-for="it in items" :key="it.elem">
              <TableCell class="sb-mono text-left font-medium">{{ it.elem }}</TableCell>
              <TableCell>
                <div v-if="editElem === it.elem" class="flex items-center justify-center gap-1">
                  <Input v-model="editScore" type="number" class="h-7 w-24 text-xs" @keydown.enter="saveScore(it.elem)" />
                  <Button size="sm" variant="ghost" @click="saveScore(it.elem)">{{ t('components.kvZset.saveCell') }}</Button>
                </div>
                <span v-else class="block tabular-nums">{{ it.score }}</span>
              </TableCell>
              <TableCell v-if="!readonly">
                <div class="flex justify-center gap-1">
                  <Button size="sm" variant="ghost" @click="startEdit(it)">{{ t('components.kvZset.editCell') }}</Button>
                  <ConfirmAction :title="t('components.kvZset.removeConfirm')" @confirm="removeMember(it.elem)">
                    <Button size="sm" variant="destructiveGhost">{{ t('components.kvZset.deleteCell') }}</Button>
                  </ConfirmAction>
                </div>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>
      <div v-if="!readonly" class="flex flex-wrap items-center gap-2">
        <Input v-model="newElem" :placeholder="t('components.kvZset.memberPlaceholder')" class="min-w-32 flex-1 sm:w-36 sm:flex-none" />
        <Input v-model="newScore" type="number" :placeholder="t('components.kvZset.scorePlaceholder')" class="w-28 shrink-0" />
        <Button size="sm" class="shrink-0" :disabled="!newElem.trim() || newScore === ''" @click="addMember">{{ t('components.kvZset.add') }}</Button>
      </div>
    </SbAsyncRegion>
  </div>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableEmpty, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import SbAsyncRegion from '../../../SbAsyncRegion.vue'
import { useLoadState } from '@/composables/useLoadState'
import { api } from '../../../../services/api'
import ConfirmAction from '../../../ConfirmAction.vue'

const props = defineProps<{
  projectId: string
  kvKey: string
  readonly?: boolean
}>()

const emit = defineEmits<{ changed: [] }>()

const { t } = useI18n()

const items = ref<{ elem: string; score: number }[]>([])
const desc = ref(false)
const { pending, showSkeleton, showError, refreshing, error, run } = useLoadState({ fallback: 'components.kvZset.loadFailed' })
const newElem = ref('')
const newScore = ref('')
const editElem = ref('')
const editScore = ref('')

function load() {
  return run(async () => {
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
    return true
  })
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
    toast.error(errorMessage(e, t('components.kvZset.addFailed')))
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
    toast.error(errorMessage(e, t('components.kvZset.saveFailed')))
  }
}

async function removeMember(elem: string) {
  try {
    await api.kv.exec(props.projectId, { type: 'cmd', argvs: ['ZREM', props.kvKey, elem] })
    await load()
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e, t('components.kvZset.deleteFailed')))
  }
}

watch(desc, () => void load())
watch(() => props.kvKey, () => void load(), { immediate: true })
</script>
