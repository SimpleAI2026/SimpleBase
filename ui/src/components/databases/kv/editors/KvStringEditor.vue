<template>
  <div class="flex flex-col gap-3">
    <div v-if="loading" class="flex justify-center py-6"><Spinner /></div>
    <template v-else>
      <Textarea v-model="value" :rows="6" :disabled="readonly" class="sb-mono text-xs" placeholder="值" />
      <div v-if="!readonly" class="flex flex-wrap items-center gap-2">
        <Button size="sm" :disabled="saving || value === original" @click="save">保存</Button>
        <div class="ml-auto flex items-center gap-1">
          <Input v-model="delta" type="number" class="w-24" placeholder="步进" />
          <Button variant="outline" size="sm" :disabled="incring" @click="incr">INCR</Button>
        </div>
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
import { Textarea } from '@/components/ui/textarea'
import { api } from '../../../../services/api'

const props = defineProps<{
  projectId: string
  kvKey: string
  readonly?: boolean
}>()

const emit = defineEmits<{ changed: [] }>()

const value = ref('')
const original = ref('')
const delta = ref('1')
const loading = ref(false)
const saving = ref(false)
const incring = ref(false)

async function load() {
  loading.value = true
  try {
    const r = await api.kv.exec(props.projectId, { type: 'cmd', argvs: ['GET', props.kvKey] })
    const v = typeof r === 'string' ? r : ''
    value.value = v
    original.value = v
  } catch (e) {
    toast.error(errorMessage(e, '加载失败'))
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await api.kv.exec(props.projectId, {
      type: 'String',
      args: { key: props.kvKey, value: value.value, keep_ttl: true }
    })
    original.value = value.value
    toast.success('已保存')
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e, '保存失败'))
  } finally {
    saving.value = false
  }
}

async function incr() {
  const d = Number(delta.value)
  if (!Number.isFinite(d)) return
  incring.value = true
  try {
    const r = await api.kv.exec(props.projectId, {
      type: 'cmd',
      argvs: ['INCRBY', props.kvKey, String(Math.trunc(d))]
    })
    value.value = String(r)
    original.value = value.value
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e, 'INCR 失败（值需为整数）'))
  } finally {
    incring.value = false
  }
}

watch(() => props.kvKey, () => void load(), { immediate: true })
</script>
