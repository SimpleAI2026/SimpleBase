<template>
  <SbModal
    :open="open"
    :title="t('components.functionTriggerModal.title')"
    :max-width="720"
    :confirm-loading="saving"
    :ok-text="t('common.save')"
    :ok-button-props="{ disabled: !canSave }"
    @update:open="emit('update:open', $event)"
    @ok="save"
  >
    <FieldGroup>
      <div class="grid gap-5 md:grid-cols-2">
        <Field>
          <FieldLabel id="trigger-kind-label">{{ t('components.functionTriggerModal.kind') }}</FieldLabel>
          <Select :model-value="form.kind" @update:model-value="selectKind">
            <SelectTrigger aria-labelledby="trigger-kind-label"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="record_change">record_change</SelectItem>
              <SelectItem value="object_event">object_event</SelectItem>
              <SelectItem value="http">http</SelectItem>
            </SelectContent>
          </Select>
        </Field>
        <Field v-if="form.kind !== 'http'">
          <FieldLabel for="trigger-target">{{ t('components.functionTriggerModal.target') }}</FieldLabel>
          <Input id="trigger-target" v-model="form.target" placeholder="tasks, images/ or *" />
          <FieldDescription>{{ t('components.functionTriggerModal.targetHint') }}</FieldDescription>
        </Field>
        <Field>
          <FieldLabel for="trigger-events">{{ t('components.functionTriggerModal.events') }}</FieldLabel>
          <Input id="trigger-events" v-model="eventsText" placeholder="create,update,delete or *" />
        </Field>
        <Field>
          <FieldLabel id="trigger-function-label">{{ t('components.functionTriggerModal.function') }}</FieldLabel>
          <SbBlockSkeleton v-if="pending && functions.length === 0" variant="lines" :count="2" />
          <Select v-else :model-value="form.funcFile || undefined" @update:model-value="selectFunction">
            <SelectTrigger aria-labelledby="trigger-function-label" :disabled="Boolean(loadError)">
              <SelectValue :placeholder="t('components.functionTriggerModal.selectFunction')" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="fn in availableFunctions" :key="fn.id" :value="fn.name">{{ fn.file }}</SelectItem>
            </SelectContent>
          </Select>
          <FieldDescription v-if="!pending && !loadError && functions.length > 0 && availableFunctions.length === 0">
            {{ t('components.functionTriggerModal.noPublished') }}
          </FieldDescription>
          <FieldDescription v-else-if="!pending && !loadError && functions.length === 0">
            {{ t('components.functionTriggerModal.noFunctions') }}
          </FieldDescription>
        </Field>
        <Field>
          <FieldLabel id="trigger-export-label">{{ t('components.functionTriggerModal.export') }}</FieldLabel>
          <Select v-model="form.funcExport">
            <SelectTrigger aria-labelledby="trigger-export-label" :disabled="!form.funcFile">
              <SelectValue :placeholder="t('components.functionTriggerModal.selectExport')" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="name in currentExports" :key="name" :value="name">{{ name }}</SelectItem>
            </SelectContent>
          </Select>
        </Field>
        <Field class="md:col-span-2">
          <FieldLabel for="trigger-filter">{{ t('components.functionTriggerModal.filter') }}</FieldLabel>
          <Input id="trigger-filter" v-model="form.filter" placeholder='status="open"' />
        </Field>
      </div>

      <SbAsyncRegion
        v-if="loadError"
        :show-error="true"
        :error="loadError"
        @retry="loadFunctions"
      />

      <Field>
        <label class="flex items-center gap-2 text-sm">
          <Switch v-model="form.enabled" />
          {{ t('common.enabledSw') }}
        </label>
      </Field>
      <FieldError v-if="formError" class="whitespace-pre-wrap break-words">{{ formError }}</FieldError>
    </FieldGroup>
  </SbModal>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { api } from '@/services/api'
import type { FunctionTrigger, FunctionTriggerInput, GoFunctionItem } from '@/services/types'
import { errorMessage } from '@/utils/format'
import SbModal from './SbModal.vue'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import SbBlockSkeleton from '@/components/SbBlockSkeleton.vue'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

const props = defineProps<{
  open: boolean
  projectId: string
}>()
const emit = defineEmits<{
  'update:open': [value: boolean]
  saved: []
}>()

const { t } = useI18n()
const functions = ref<GoFunctionItem[]>([])
const pending = ref(false)
const loadError = ref('')
const saving = ref(false)
const formError = ref('')
const eventsText = ref('*')
const form = reactive<FunctionTriggerInput>({
  kind: 'record_change',
  target: '*',
  events: ['*'],
  funcFile: '',
  funcExport: '',
  filter: '',
  enabled: true
})
let functionsRequest = 0

const availableFunctions = computed(() => functions.value.filter((fn) => fn.published && fn.activeVersion > 0))
const currentExports = computed(() => functions.value.find((fn) => fn.name === form.funcFile)?.exports || [])
const canSave = computed(() => {
  if (pending.value || loadError.value || saving.value) return false
  if (form.kind !== 'http' && !form.target.trim()) return false
  if (!eventsText.value.split(',').some((value) => value.trim())) return false
  if (!form.funcFile || !form.funcExport) return false
  return availableFunctions.value.some((fn) => fn.name === form.funcFile)
})

function resetForm() {
  form.kind = 'record_change'
  form.target = '*'
  form.events = ['*']
  form.funcFile = ''
  form.funcExport = ''
  form.filter = ''
  form.enabled = true
  eventsText.value = '*'
  formError.value = ''
}
function selectKind(kind: FunctionTrigger['kind']) {
  form.kind = kind
  form.target = kind === 'http' ? '' : form.target || '*'
}
function selectFunction(name: string) {
  form.funcFile = name
  form.funcExport = functions.value.find((fn) => fn.name === name)?.exports[0] || ''
}
async function loadFunctions() {
  const currentProject = props.projectId
  const request = ++functionsRequest
  if (!currentProject) {
    functions.value = []
    return
  }
  pending.value = true
  loadError.value = ''
  try {
    const list = await api.gofunctions.list(currentProject)
    if (request !== functionsRequest || currentProject !== props.projectId) return
    functions.value = list
    const available = list.filter((fn) => fn.published && fn.activeVersion > 0)
    if (!available.some((fn) => fn.name === form.funcFile)) {
      form.funcFile = available[0]?.name || ''
      form.funcExport = available[0]?.exports[0] || ''
    } else if (!currentExports.value.includes(form.funcExport)) {
      form.funcExport = currentExports.value[0] || ''
    }
  } catch (error) {
    if (request !== functionsRequest || currentProject !== props.projectId) return
    functions.value = []
    form.funcFile = ''
    form.funcExport = ''
    loadError.value = errorMessage(error, t('components.functionTriggerModal.loadFunctionsFailed'))
  } finally {
    if (request === functionsRequest) pending.value = false
  }
}
async function save() {
  if (!canSave.value) return
  const currentProject = props.projectId
  saving.value = true
  formError.value = ''
  try {
    const payload: FunctionTriggerInput = {
      kind: form.kind,
      target: form.kind === 'http' ? '' : form.target.trim(),
      events: eventsText.value.split(',').map((value) => value.trim()).filter(Boolean),
      funcFile: form.funcFile,
      funcExport: form.funcExport,
      filter: form.filter?.trim() || '',
      enabled: form.enabled
    }
    form.events = payload.events
    form.target = payload.target
    await api.triggers.create(currentProject, payload)
    if (currentProject !== props.projectId) return
    toast.success(t('components.functionTriggerModal.saved'))
    emit('saved')
    emit('update:open', false)
  } catch (error) {
    formError.value = errorMessage(error, t('components.functionTriggerModal.saveFailed'))
  } finally {
    saving.value = false
  }
}

watch(
  () => [props.open, props.projectId],
  ([open]) => {
    functionsRequest++
    if (!open) return
    resetForm()
    functions.value = []
    loadError.value = ''
    void loadFunctions()
  },
  { immediate: true }
)
</script>
