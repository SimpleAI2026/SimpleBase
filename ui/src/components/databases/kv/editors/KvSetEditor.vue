<template>
  <div class="flex flex-col gap-3">
    <div v-if="loading" class="flex justify-center py-6"><Spinner /></div>
    <template v-else>
      <div v-if="!elems.length" class="rounded-md border border-dashed border-border/70 py-6 text-center text-xs text-muted-foreground">
        集合为空
      </div>
      <div v-else class="flex flex-wrap gap-1.5">
        <Badge v-for="el in elems" :key="el" variant="secondary" class="sb-mono text-xs">
          {{ el }}
          <button
            v-if="!readonly"
            class="ml-1 opacity-60 hover:opacity-100"
            title="移除"
            @click="removeMember(el)"
          >×</button>
        </Badge>
      </div>
      <div v-if="!readonly" class="flex flex-wrap items-center gap-2">
        <Input v-model="newMember" placeholder="新成员" class="w-48" @keydown.enter="addMember" />
        <Button size="sm" :disabled="!newMember.trim()" @click="addMember">添加</Button>
        <Button
          size="sm"
          variant="outline"
          class="ml-auto"
          :disabled="!elems.length"
          @click="popRandom"
        >随机弹出</Button>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { api } from '../../../../services/api'

const props = defineProps<{
  projectId: string
  kvKey: string
  readonly?: boolean
}>()

const emit = defineEmits<{ changed: [] }>()

const elems = ref<string[]>([])
const loading = ref(false)
const newMember = ref('')

async function load() {
  loading.value = true
  try {
    const r = (await api.kv.exec(props.projectId, {
      type: 'cmd',
      argvs: ['SMEMBERS', props.kvKey]
    })) as string[]
    elems.value = Array.isArray(r) ? r : []
  } catch (e) {
    toast.error(errorMessage(e, '加载失败'))
  } finally {
    loading.value = false
  }
}

async function addMember() {
  const v = newMember.value.trim()
  if (!v) return
  try {
    await api.kv.exec(props.projectId, { type: 'Set', args: { key: props.kvKey, elems: [v] } })
    newMember.value = ''
    await load()
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e, '添加失败'))
  }
}

async function removeMember(v: string) {
  try {
    await api.kv.exec(props.projectId, { type: 'cmd', argvs: ['SREM', props.kvKey, v] })
    await load()
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e, '移除失败'))
  }
}

async function popRandom() {
  try {
    const r = await api.kv.exec(props.projectId, { type: 'cmd', argvs: ['SPOP', props.kvKey] })
    toast.success('已弹出: ' + (r ?? ''))
    await load()
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e, '弹出失败'))
  }
}

watch(() => props.kvKey, () => void load(), { immediate: true })
</script>
