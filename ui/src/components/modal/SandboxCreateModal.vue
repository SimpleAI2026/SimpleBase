<template>
  <SbModal
    :open="open"
    :title="t('components.sandboxCreate.title')"
    :description="t('components.sandboxCreate.description')"
    :confirm-loading="saving"
    :ok-button-props="{ disabled: !valid }"
    @update:open="$emit('update:open', $event)"
    @ok="save"
  >
    <FieldGroup class="py-3">
      <Field>
        <FieldLabel for="sb-create-name">{{ t('components.sandboxCreate.nameLabel') }}</FieldLabel>
        <Input
          id="sb-create-name"
          v-model="name"
          placeholder="etl-check"
          :aria-label="t('components.sandboxCreate.nameAria')"
          :aria-invalid="nameInvalid ? true : undefined"
        />
        <FieldError v-if="nameInvalid">{{ t('components.sandboxCreate.nameInvalid') }}</FieldError>
      </Field>
      <Field>
        <FieldLabel for="sb-create-image">{{ t('components.sandboxCreate.templateLabel') }}</FieldLabel>
        <Select v-model="image">
          <SelectTrigger id="sb-create-image" class="w-full" :aria-label="t('components.sandboxCreate.templateAria')">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem v-for="option in capabilities?.images || []" :key="option" :value="option">{{ option }}</SelectItem>
          </SelectContent>
        </Select>
      </Field>
      <Field>
        <FieldLabel for="sb-create-timeout">{{ t('components.sandboxCreate.timeoutLabel') }}</FieldLabel>
        <Input
          id="sb-create-timeout"
          v-model.number="timeoutS"
          type="number"
          min="1"
          :max="timeoutMax"
          :aria-label="t('components.sandboxCreate.timeoutAria')"
        />
        <FieldError v-if="timeoutInvalid">{{ t('components.sandboxCreate.timeoutInvalid', { max: timeoutMax }) }}</FieldError>
      </Field>
    </FieldGroup>
  </SbModal>
</template>
<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { api } from '../../services/api'
import type { SandboxCapabilities } from '../../services/types'
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import SbModal from './SbModal.vue'

const SANDBOX_NAME_RE = /^[a-z0-9][a-z0-9-]{0,62}$/

const props = defineProps<{ open: boolean; projectId: string; capabilities: SandboxCapabilities | null }>()
const emit = defineEmits<{ 'update:open': [value: boolean]; saved: [] }>()

const { t } = useI18n()

const name = ref('')
const image = ref('')
const timeoutS = ref(300)
const saving = ref(false)

const nameInvalid = computed(() => Boolean(name.value) && !SANDBOX_NAME_RE.test(name.value))
const timeoutMax = computed(() => props.capabilities?.timeoutMaxS || 3600)
const timeoutInvalid = computed(() => !Number.isInteger(timeoutS.value) || timeoutS.value < 1 || timeoutS.value > timeoutMax.value)

const valid = computed(() => !nameInvalid.value && !timeoutInvalid.value)

watch(() => props.open, (open) => {
  if (!open) return
  name.value = ''
  image.value = props.capabilities?.defaultTemplate || props.capabilities?.defaultImage || ''
  timeoutS.value = props.capabilities?.timeoutDefaultS || 300
})

async function save() {
  if (!valid.value || saving.value) return
  saving.value = true
  try {
    await api.sandboxes.create(props.projectId, {
      name: name.value || undefined,
      templateId: image.value || undefined,
      timeoutS: timeoutS.value
    })
    emit('update:open', false)
    emit('saved')
    toast.success(t('components.sandboxCreate.created'))
  } catch (e) {
    toast.error(errorMessage(e, t('components.sandboxCreate.createFailed')))
  } finally {
    saving.value = false
  }
}
</script>
