<template>
  <SbModal
    :open="open"
    :title="t('components.kvTtl.title')"
    :width="440"
    :confirm-loading="submitting"
    :ok-button-props="{ disabled: mode === 'custom' && !validSeconds }"
    :ok-text="t('common.save')"
    @ok="submit"
    @update:open="emit('update:open', $event)"
  >
    <FieldGroup>
      <div class="text-sm text-muted-foreground sb-mono">{{ kvKey?.key }}</div>
      <Field>
        <FieldLabel>{{ t('components.kvTtl.strategy') }}</FieldLabel>
        <div class="flex flex-wrap items-center gap-2">
          <Select v-model="mode">
            <SelectTrigger class="w-40 shrink-0">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="none">{{ t('components.kvTtl.forever') }}</SelectItem>
              <SelectItem value="custom">{{ t('components.kvTtl.custom') }}</SelectItem>
            </SelectContent>
          </Select>
          <Input v-if="mode === 'custom'" v-model="seconds" type="number" min="1" class="min-w-32 flex-1 sm:w-40 sm:flex-none" :placeholder="t('components.kvTtl.secondsPlaceholder')" />
        </div>
      </Field>
      <div v-if="kvKey?.ttl_ms != null" class="text-xs text-muted-foreground">
        {{ t('components.kvTtl.remaining', { n: Math.ceil((kvKey?.ttl_ms ?? 0) / 1000) }) }}
      </div>
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { api } from '../../../services/api'
import type { KvKeyMeta } from '../../../services/api'
import SbModal from '../../modal/SbModal.vue'

const props = defineProps<{
  open: boolean
  projectId: string
  kvKey: KvKeyMeta | null
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
  changed: []
}>()

const { t } = useI18n()

const mode = ref<'none' | 'custom'>('none')
const seconds = ref('')
const submitting = ref(false)

watch(
  () => [props.open, props.kvKey?.key],
  () => {
    if (props.open) {
      mode.value = props.kvKey?.ttl_ms != null ? 'custom' : 'none'
      seconds.value = props.kvKey?.ttl_ms != null ? String(Math.ceil(props.kvKey.ttl_ms / 1000)) : ''
    }
  },
  { immediate: true }
)

const validSeconds = computed(() => {
  const n = Number(seconds.value)
  return Number.isFinite(n) && n > 0
})

async function submit() {
  if (!props.kvKey) return
  submitting.value = true
  try {
    if (mode.value === 'none') {
      await api.kv.exec(props.projectId, { type: 'cmd', argvs: ['PERSIST', props.kvKey.key] })
      toast.success(t('components.kvTtl.setForever'))
    } else {
      await api.kv.exec(props.projectId, {
        type: 'cmd',
        argvs: ['PEXPIRE', props.kvKey.key, String(Number(seconds.value) * 1000)]
      })
      toast.success(t('components.kvTtl.setTtl'))
    }
    emit('update:open', false)
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e, t('components.kvTtl.setFailed')))
  } finally {
    submitting.value = false
  }
}
</script>
