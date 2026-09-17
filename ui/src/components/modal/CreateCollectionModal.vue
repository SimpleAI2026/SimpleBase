<template>
  <SbModal
    :open="open"
    title="新建集合"
    :confirm-loading="submitting"
    :ok-button-props="{ disabled: !canSubmit }"
    @ok="submit"
    @update:open="emit('update:open', $event)"
  >
    <Field :data-invalid="nameError ? true : undefined">
      <FieldLabel for="collection-name">集合名称</FieldLabel>
      <Input
        id="collection-name"
        v-model="name"
        placeholder="例如：users（字母开头，仅字母数字下划线）"
        :aria-invalid="nameError ? true : undefined"
        @keydown.enter="submit"
      />
      <FieldDescription v-if="nameError">{{ nameError }}</FieldDescription>
    </Field>
  </SbModal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { api } from '../../services/api'
import type { DatabaseItem } from '../../services/api'
import SbModal from './SbModal.vue'

const COLLECTION_NAME_RE = /^[A-Za-z][A-Za-z0-9_]{0,62}$/

const props = defineProps<{
  open: boolean
  projectId: string
  database: DatabaseItem | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  created: [name: string]
}>()

const name = ref('')
const submitting = ref(false)

const nameError = computed(() => {
  const n = name.value.trim()
  if (!n) return ''
  if (!COLLECTION_NAME_RE.test(n)) {
    return '集合名称须以字母开头，仅含字母、数字和下划线'
  }
  return ''
})

const canSubmit = computed(() => name.value.trim().length > 0 && !nameError.value && !!props.database)

watch(
  () => props.open,
  (v) => {
    if (v) {
      name.value = ''
      submitting.value = false
    }
  }
)

async function submit() {
  if (!canSubmit.value || !props.database) {
    if (!name.value.trim()) toast.warning('请输入集合名称')
    return
  }
  submitting.value = true
  try {
    const trimmed = name.value.trim()
    await api.db.createCollection(props.projectId, props.database.id, trimmed)
    toast.success('集合创建成功')
    emit('created', trimmed)
    emit('update:open', false)
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '集合创建失败')
  } finally {
    submitting.value = false
  }
}
</script>
