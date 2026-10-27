<template>
  <SbModal
    :open="open"
    :title="t('components.createProject.title')"
    :confirm-loading="submitting"
    :ok-button-props="{ disabled: !canSubmit }"
    @ok="submit"
    @update:open="emit('update:open', $event)"
  >
    <FieldGroup>
      <Field :data-invalid="nameError ? true : undefined">
        <FieldLabel for="project-name">{{ t('components.createProject.name') }}</FieldLabel>
        <Input
          id="project-name"
          v-model="name"
          :placeholder="t('components.createProject.namePlaceholder')"
          maxlength="128"
          :aria-invalid="nameError ? true : undefined"
          @keydown.enter="submit"
        />
        <FieldDescription v-if="nameError">{{ nameError }}</FieldDescription>
      </Field>
      <Field :data-invalid="idError ? true : undefined">
        <FieldLabel for="project-id">{{ t('components.createProject.projectId') }}</FieldLabel>
        <Input
          id="project-id"
          v-model="customId"
          :placeholder="t('components.createProject.projectIdPlaceholder')"
          class="font-mono"
          maxlength="8"
          :aria-invalid="idError ? true : undefined"
          @keydown.enter="submit"
        />
        <FieldDescription>{{ idError || t('components.createProject.projectIdHint') }}</FieldDescription>
      </Field>
      <Field>
        <FieldLabel for="project-engine">{{ t('components.createProject.engine') }}</FieldLabel>
        <select
          id="project-engine"
          v-model="engine"
          class="border-input bg-background h-9 w-full rounded-md border px-3 text-sm"
        >
          <option value="duckdb">DuckDB</option>
          <option value="sqlite">SQLite</option>
        </select>
        <FieldDescription>{{ t('components.createProject.engineHint') }}</FieldDescription>
      </Field>
    </FieldGroup>
  </SbModal>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { api } from '../../services/api'
import type { ProjectItem } from '../../services/types'
import SbModal from './SbModal.vue'

const PROJECT_ID_RE = /^[A-Za-z0-9-]{8}$/

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  created: [project: ProjectItem]
}>()

const { t } = useI18n()

const name = ref('')
const customId = ref('')
const engine = ref<'duckdb' | 'sqlite'>('duckdb')
const submitting = ref(false)

const nameError = computed(() => {
  const n = name.value.trim()
  if (!n) return ''
  if (n.length > 128) return t('components.createProject.nameTooLong')
  if (/[/\\]/.test(n) || /[\x00-\x1f]/.test(n)) return t('components.createProject.nameInvalid')
  return ''
})

const idError = computed(() => {
  const id = customId.value.trim()
  if (!id) return ''
  if (!PROJECT_ID_RE.test(id)) return t('components.createProject.projectIdInvalid')
  return ''
})

const canSubmit = computed(() => name.value.trim().length > 0 && !nameError.value && !idError.value)

watch(
  () => props.open,
  (v) => {
    if (v) {
      name.value = ''
      customId.value = ''
      engine.value = 'duckdb'
      submitting.value = false
    }
  }
)

async function submit() {
  if (!canSubmit.value) {
    if (!name.value.trim()) toast.warning(t('components.createProject.nameRequired'))
    return
  }
  submitting.value = true
  try {
    const created = await api.projects.create({
      name: name.value.trim(),
      id: customId.value.trim() || undefined,
      engine: engine.value
    })
    toast.success(t('components.createProject.created', { name: created.name || created.id }))
    emit('created', created)
    emit('update:open', false)
  } catch (e) {
    toast.error(errorMessage(e, t('components.createProject.createFailed')))
  } finally {
    submitting.value = false
  }
}
</script>
