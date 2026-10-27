<template>
  <SbModal
    :open="open"
    :title="t('components.collectionPanel.schemaTitle', { collection })"
    :max-width="920"
    :confirm-loading="saving"
    :ok-text="t('common.save')"
    @update:open="emit('update:open', $event)"
    @ok="save"
  >
    <SbAsyncRegion
      :pending="loading"
      :show-skeleton="loading"
      :show-empty="false"
      :show-error="Boolean(loadError)"
      :error="loadError"
      @retry="load"
    >
      <div class="flex flex-col gap-4 py-1">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <p class="text-sm text-muted-foreground">{{ t('components.collectionPanel.schemaHint') }}</p>
          <div class="flex gap-2">
            <Button size="sm" variant="outline" @click="addField">
              <PlusIcon data-icon="inline-start" />{{ t('components.collectionPanel.addField') }}
            </Button>
            <ConfirmAction
              v-if="registered"
              :title="t('components.collectionPanel.unregisterConfirm')"
              @confirm="unregister"
            >
              <Button size="sm" variant="destructiveGhost">{{ t('components.collectionPanel.unregister') }}</Button>
            </ConfirmAction>
          </div>
        </div>
        <div v-if="schemaDiff.length" class="rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-sm">
          <p class="font-medium">{{ t('components.collectionPanel.diffTitle') }}</p>
          <p class="text-muted-foreground">{{ schemaDiff.join(', ') }}</p>
        </div>
        <SbEmptyState
          v-if="fields.length === 0"
          :title="t('components.collectionPanel.noFields')"
          :description="t('components.collectionPanel.noFieldsHint')"
          :action-text="t('components.collectionPanel.addField')"
          @action="addField"
        />
        <div v-for="(field, index) in fields" :key="index" class="rounded-lg border border-border p-3">
          <div class="grid gap-3 md:grid-cols-2">
            <Field>
              <FieldLabel>{{ t('components.collectionPanel.fieldName') }}</FieldLabel>
              <Input v-model="field.name" />
            </Field>
            <Field>
              <FieldLabel>{{ t('components.collectionPanel.fieldType') }}</FieldLabel>
              <Select v-model="field.type">
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem v-for="option in fieldTypes" :key="option" :value="option">{{ option }}</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Field>
              <FieldLabel>{{ t('components.collectionPanel.fieldDefault') }}</FieldLabel>
              <Input v-model="field.defaultText" placeholder='{"enabled":true}' />
            </Field>
            <Field>
              <FieldLabel>{{ t('components.collectionPanel.fieldConstraints') }}</FieldLabel>
              <div class="flex flex-wrap gap-2">
                <Input v-model="field.min" class="min-w-0 flex-1" :placeholder="t('components.collectionPanel.min')" />
                <Input v-model="field.max" class="min-w-0 flex-1" :placeholder="t('components.collectionPanel.max')" />
              </div>
            </Field>
            <Field>
              <FieldLabel>{{ t('components.collectionPanel.fieldPattern') }}</FieldLabel>
              <Input v-model="field.pattern" />
            </Field>
            <Field>
              <FieldLabel>{{ t('components.collectionPanel.fieldValues') }}</FieldLabel>
              <Input v-model="field.values" :placeholder="t('components.collectionPanel.valuesHint')" />
            </Field>
          </div>
          <div class="mt-3 flex flex-wrap items-center gap-4">
            <label class="flex items-center gap-2 text-sm"><Switch v-model="field.required" />{{ t('components.collectionPanel.required') }}</label>
            <label class="flex items-center gap-2 text-sm"><Switch v-model="field.unique" />{{ t('components.collectionPanel.unique') }}</label>
            <label class="flex items-center gap-2 text-sm"><Switch v-model="field.hidden" />{{ t('components.collectionPanel.hidden') }}</label>
            <label class="flex items-center gap-2 text-sm"><Switch v-model="field.readonly" />{{ t('components.collectionPanel.readonly') }}</label>
            <Button class="ml-auto" size="sm" variant="ghost" @click="fields.splice(index, 1)">{{ t('common.del') }}</Button>
          </div>
        </div>
      </div>
    </SbAsyncRegion>
  </SbModal>
  <ConfirmAction
    controlled
    :open="confirmSave"
    :title="t('components.collectionPanel.diffConfirm')"
    :description="schemaDiff.join(', ')"
    @update:open="confirmSave = $event"
    @confirm="commitSave"
  />
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { PlusIcon } from '@lucide/vue'
import { api } from '@/services/api'
import type { ApiError, CollectionField, CollectionFieldOptions, CollectionSchemaInput } from '@/services/types'
import { errorMessage } from '@/utils/format'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Field, FieldLabel } from '@/components/ui/field'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import SbModal from '@/components/modal/SbModal.vue'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import SbEmptyState from '@/components/SbEmptyState.vue'
import ConfirmAction from '@/components/ConfirmAction.vue'

interface EditableField {
  name: string
  type: CollectionField['type']
  required: boolean
  unique: boolean
  hidden: boolean
  readonly: boolean
  defaultText: string
  min: string
  max: string
  pattern: string
  values: string
}

const props = defineProps<{
  open: boolean
  projectId: string
  databaseId: string
  collection: string
}>()
const emit = defineEmits<{
  'update:open': [value: boolean]
  saved: []
}>()
const { t } = useI18n()
const fieldTypes: CollectionField['type'][] = ['text', 'number', 'bool', 'date', 'email', 'url', 'select', 'json']
const fields = ref<EditableField[]>([])
const registered = ref(false)
const loading = ref(false)
const saving = ref(false)
const loadError = ref('')
const originalFields = ref<EditableField[]>([])
const pendingInput = ref<CollectionSchemaInput | null>(null)
const confirmSave = ref(false)

function fieldSnapshot(field: EditableField): string {
  return JSON.stringify(field)
}
const schemaDiff = computed(() => {
  const before = new Map(originalFields.value.map((field) => [field.name, fieldSnapshot(field)]))
  const current = new Map(fields.value.map((field) => [field.name, fieldSnapshot(field)]))
  const changes: string[] = []
  for (const [name, snapshot] of current) {
    if (!before.has(name)) changes.push(t('components.collectionPanel.diffAdded', { name }))
    else if (before.get(name) !== snapshot) changes.push(t('components.collectionPanel.diffChanged', { name }))
  }
  for (const name of before.keys()) {
    if (!current.has(name)) changes.push(t('components.collectionPanel.diffRemoved', { name }))
  }
  return changes
})

function blankField(): EditableField {
  return { name: '', type: 'text', required: false, unique: false, hidden: false, readonly: false, defaultText: '', min: '', max: '', pattern: '', values: '' }
}
function addField() {
  fields.value.push(blankField())
}
function toEditableField(field: CollectionField): EditableField {
  return {
    name: field.name,
    type: field.type,
    required: field.required,
    unique: field.unique,
    hidden: field.hidden,
    readonly: field.readonly,
    defaultText: field.default === undefined ? '' : JSON.stringify(field.default),
    min: field.options.min === undefined ? '' : String(field.options.min),
    max: field.options.max === undefined ? '' : String(field.options.max),
    pattern: field.options.pattern ?? '',
    values: field.options.values?.join(',') ?? ''
  }
}
async function load() {
  if (!props.open || !props.collection) return
  loading.value = true
  loadError.value = ''
  try {
    const schema = await api.db.getCollectionSchema(props.projectId, props.databaseId, props.collection)
    registered.value = true
    fields.value = schema.fields.map(toEditableField)
    originalFields.value = fields.value.map((field) => ({ ...field }))
  } catch (error) {
    const status = (error as ApiError).status
    if (status === 404) {
      registered.value = false
      fields.value = []
      originalFields.value = []
    } else {
      loadError.value = errorMessage(error, t('components.collectionPanel.schemaLoadFailed'))
    }
  } finally {
    loading.value = false
  }
}
function bound(value: string, date: boolean): number | string | undefined {
  if (!value.trim()) return undefined
  return date ? value.trim() : Number(value)
}
function toSchemaField(field: EditableField): CollectionField {
  const options: CollectionFieldOptions = {
    min: bound(field.min, field.type === 'date'),
    max: bound(field.max, field.type === 'date'),
    pattern: field.pattern || undefined,
    values: field.type === 'select' ? field.values.split(',').map((item) => item.trim()).filter(Boolean) : undefined
  }
  let defaultValue: unknown
  if (field.defaultText.trim()) defaultValue = JSON.parse(field.defaultText)
  return {
    name: field.name.trim(),
    type: field.type,
    required: field.required,
    unique: field.unique,
    hidden: field.hidden,
    readonly: field.readonly,
    default: defaultValue,
    options
  }
}
async function save() {
  saving.value = true
  try {
    const input = { name: props.collection, fields: fields.value.map(toSchemaField) }
    const dryRun = await api.db.putCollectionSchema(props.projectId, props.databaseId, input, true)
    if ('invalid' in dryRun && (dryRun.invalid.length > 0 || dryRun.truncated)) {
      toast.error(t('components.collectionPanel.schemaInvalid', { count: dryRun.invalid.length }))
      return
    }
    pendingInput.value = input
    confirmSave.value = true
  } catch (error) {
    toast.error(errorMessage(error, t('components.collectionPanel.schemaSaveFailed')))
  } finally {
    saving.value = false
  }
}
async function commitSave() {
  if (!pendingInput.value) return
  saving.value = true
  try {
    await api.db.putCollectionSchema(props.projectId, props.databaseId, pendingInput.value)
    pendingInput.value = null
    toast.success(t('components.collectionPanel.schemaSaved'))
    emit('saved')
    emit('update:open', false)
  } catch (error) {
    toast.error(errorMessage(error, t('components.collectionPanel.schemaSaveFailed')))
  } finally {
    saving.value = false
  }
}
async function unregister() {
  try {
    await api.db.deleteCollectionSchema(props.projectId, props.databaseId, props.collection)
    toast.success(t('components.collectionPanel.unregistered'))
    emit('saved')
    emit('update:open', false)
  } catch (error) {
    toast.error(errorMessage(error, t('components.collectionPanel.schemaSaveFailed')))
  }
}
watch(() => [props.open, props.collection], load, { immediate: true })
</script>
