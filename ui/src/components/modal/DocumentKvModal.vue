<template>
  <SbModal
    :open="open"
    :title="t('components.documentKv.title')"
    :width="480"
    :confirm-loading="submitting"
    :ok-button-props="{ disabled: !canSubmit }"
    @ok="submit"
    @update:open="emit('update:open', $event)"
  >
    <FieldGroup>
      <Field>
        <FieldLabel for="doc-key">Key</FieldLabel>
        <Input id="doc-key" v-model="key" :placeholder="t('components.documentKv.keyPlaceholder')" @keydown.enter="submit" />
      </Field>
      <Field>
        <FieldLabel for="doc-value">Value</FieldLabel>
        <Textarea
          id="doc-value"
          v-model="value"
          :rows="6"
          :placeholder="t('components.documentKv.valuePlaceholder')"
        />
      </Field>
    </FieldGroup>
  </SbModal>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
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

const { t } = useI18n()

const key = ref('')
const value = ref('')
const submitting = ref(false)

const canSubmit = computed(
  () => key.value.trim().length > 0 && !!props.databaseId && !!props.collection
)

function parseValue(raw: string): unknown {
  const trimmed = raw.trim()
  if (!trimmed) return ''
  try {
    return JSON.parse(trimmed)
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
    toast.warning(t('components.documentKv.keyRequired'))
    return
  }
  submitting.value = true
  try {
    const payload: Record<string, unknown> = { [k]: parseValue(value.value) }
    await api.db.insert(props.projectId, props.databaseId, props.collection, payload)
    toast.success(t('components.documentKv.created'))
    emit('created')
    emit('update:open', false)
  } catch (e) {
    toast.error(errorMessage(e, t('components.documentKv.createFailed')))
  } finally {
    submitting.value = false
  }
}
</script>
