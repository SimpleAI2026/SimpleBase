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
        <FieldLabel for="sb-create-image">{{ t('components.sandboxCreate.imageLabel') }}</FieldLabel>
        <Select v-model="image">
          <SelectTrigger id="sb-create-image" class="w-full" :aria-label="t('components.sandboxCreate.imageAria')">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem v-for="option in capabilities?.images || []" :key="option" :value="option">{{ option }}</SelectItem>
          </SelectContent>
        </Select>
      </Field>
      <div class="grid grid-cols-2 gap-3">
        <Field>
          <FieldLabel for="sb-create-cpus">{{ t('components.sandboxCreate.cpuLabel') }}</FieldLabel>
          <Input
            id="sb-create-cpus"
            v-model.number="cpus"
            type="number"
            min="1"
            :max="capabilities?.cpusMax || 4"
            :aria-label="t('components.sandboxCreate.cpuAria')"
          />
        </Field>
        <Field>
          <FieldLabel for="sb-create-memory">{{ t('components.sandboxCreate.memoryLabel') }}</FieldLabel>
          <Input
            id="sb-create-memory"
            v-model.number="memoryMiB"
            type="number"
            min="128"
            :max="capabilities?.memoryMiBMax || 4096"
            :aria-label="t('components.sandboxCreate.memoryAria')"
          />
        </Field>
      </div>
      <Field>
        <FieldLabel for="sb-create-network">{{ t('components.sandboxCreate.networkLabel') }}</FieldLabel>
        <Select v-model="network">
          <SelectTrigger id="sb-create-network" class="w-full" :aria-label="t('components.sandboxCreate.networkAria')">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem v-for="option in capabilities?.networkOptions || ['none']" :key="option" :value="option">{{ option }}</SelectItem>
          </SelectContent>
        </Select>
      </Field>
      <label class="flex items-center gap-2 text-sm">
        <input v-model="start" type="checkbox" class="size-4 accent-primary" />
        {{ t('components.sandboxCreate.startNow') }}
      </label>
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
const cpus = ref(1)
const memoryMiB = ref(256)
const network = ref('none')
const start = ref(false)
const saving = ref(false)

const nameInvalid = computed(() => Boolean(name.value) && !SANDBOX_NAME_RE.test(name.value))

const valid = computed(
  () =>
    !nameInvalid.value &&
    cpus.value >= 1 &&
    cpus.value <= (props.capabilities?.cpusMax || 4) &&
    memoryMiB.value >= 128 &&
    memoryMiB.value <= (props.capabilities?.memoryMiBMax || 4096)
)

watch(() => props.open, (open) => {
  if (!open) return
  name.value = ''
  image.value = props.capabilities?.defaultImage || ''
  cpus.value = 1
  memoryMiB.value = 256
  network.value = 'none'
  start.value = false
})

async function save() {
  if (!valid.value || saving.value) return
  saving.value = true
  try {
    await api.sandboxes.create(props.projectId, {
      name: name.value || undefined,
      image: image.value,
      cpus: cpus.value,
      memoryMiB: memoryMiB.value,
      network: network.value,
      start: start.value
    }, crypto.randomUUID())
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
