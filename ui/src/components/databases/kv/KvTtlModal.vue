<template>
  <SbModal
    :open="open"
    title="设置过期时间"
    :width="440"
    :confirm-loading="submitting"
    :ok-button-props="{ disabled: mode === 'custom' && !validSeconds }"
    ok-text="保存"
    @ok="submit"
    @update:open="emit('update:open', $event)"
  >
    <FieldGroup>
      <div class="text-sm text-muted-foreground sb-mono">{{ kvKey?.key }}</div>
      <Field>
        <FieldLabel>过期策略</FieldLabel>
        <div class="flex items-center gap-2">
          <Select v-model="mode">
            <SelectTrigger class="w-40">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="none">永久</SelectItem>
              <SelectItem value="custom">自定义秒数</SelectItem>
            </SelectContent>
          </Select>
          <Input v-if="mode === 'custom'" v-model="seconds" type="number" min="1" class="w-40" placeholder="秒" />
        </div>
      </Field>
      <div v-if="kvKey?.ttl_ms != null" class="text-xs text-muted-foreground">
        当前剩余约 {{ Math.ceil((kvKey?.ttl_ms ?? 0) / 1000) }} 秒
      </div>
    </FieldGroup>
  </SbModal>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { computed, ref, watch } from 'vue'
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
      toast.success('已设为永久')
    } else {
      await api.kv.exec(props.projectId, {
        type: 'cmd',
        argvs: ['PEXPIRE', props.kvKey.key, String(Number(seconds.value) * 1000)]
      })
      toast.success('已设置过期时间')
    }
    emit('update:open', false)
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e, '设置失败'))
  } finally {
    submitting.value = false
  }
}
</script>
