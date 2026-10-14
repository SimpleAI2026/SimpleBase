<template>
  <SbModal
    :open="open"
    :title="t('components.createCollection.title')"
    :confirm-loading="submitting"
    :ok-button-props="{ disabled: !canSubmit }"
    @ok="submit"
    @update:open="emit('update:open', $event)"
  >
    <Field :data-invalid="nameError ? true : undefined">
      <FieldLabel for="collection-name">{{ t('components.createCollection.name') }}</FieldLabel>
      <Input
        id="collection-name"
        v-model="name"
        :placeholder="t('components.createCollection.namePlaceholder')"
        :aria-invalid="nameError ? true : undefined"
        @keydown.enter="submit"
      />
      <FieldDescription v-if="nameError">{{ nameError }}</FieldDescription>
    </Field>
  </SbModal>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
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

const { t } = useI18n()

const name = ref('')
const submitting = ref(false)

const nameError = computed(() => {
  const n = name.value.trim()
  if (!n) return ''
  if (!COLLECTION_NAME_RE.test(n)) {
    return t('components.createCollection.nameInvalid')
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
    if (!name.value.trim()) toast.warning(t('components.createCollection.nameRequired'))
    return
  }
  submitting.value = true
  try {
    const trimmed = name.value.trim()
    await api.db.createCollection(props.projectId, props.database.id, trimmed)
    toast.success(t('components.createCollection.created'))
    emit('created', trimmed)
    emit('update:open', false)
  } catch (e) {
    toast.error(errorMessage(e, t('components.createCollection.createFailed')))
  } finally {
    submitting.value = false
  }
}
</script>
