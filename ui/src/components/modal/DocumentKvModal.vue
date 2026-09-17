<template>
  <SbModal
    :open="open"
    title="新增文档"
    :width="480"
    :confirm-loading="submitting"
    :ok-button-props="{ disabled: !canSubmit }"
    @ok="submit"
    @update:open="emit('update:open', $event)"
  >
    <FieldGroup>
      <Field>
        <FieldLabel for="doc-key">Key</FieldLabel>
        <Input id="doc-key" v-model="key" placeholder="字段名，例如 name" @keydown.enter="submit" />
      </Field>
      <Field>
        <FieldLabel for="doc-value">Value</FieldLabel>
        <Textarea
          id="doc-value"
          v-model="value"
          :rows="6"
          placeholder='字符串，或 JSON（如 1、true、{"city":"SZ"}）'
        />
      </Field>
    </FieldGroup>
  </SbModal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { api } from '../../services/api'
import SbModal from './SbModal.vue'

const props = defineProps<{
  open: boolean
  projectId: string
  databaseId: string
  collection: string
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  created: []
}>()

const key = ref('')
const value = ref('')
const submitting = ref(false)

const canSubmit = computed(
  () => key.value.trim().length > 0 && !!props.databaseId && !!props.collection
)

function parseValue(raw: string): unknown {
  const t = raw.trim()
  if (!t) return ''
  try {
    return JSON.parse(t)
  } catch {
    return raw
  }
}

watch(
  () => props.open,
  (v) => {
    if (v) {
      key.value = ''
      value.value = ''
      submitting.value = false
    }
  }
)

async function submit() {
  const k = key.value.trim()
  if (!k) {
    toast.warning('请输入 Key')
    return
  }
  submitting.value = true
  try {
    const payload: Record<string, unknown> = { [k]: parseValue(value.value) }
    await api.db.insert(props.projectId, props.databaseId, props.collection, payload)
    toast.success('创建成功')
    emit('created')
    emit('update:open', false)
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '创建失败')
  } finally {
    submitting.value = false
  }
}
</script>
