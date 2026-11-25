<template>
  <div class="flex flex-col gap-3">
    <SbAsyncRegion class="flex flex-col gap-3" block="spinner" :state="state" @retry="load">
      <Textarea v-model="value" :rows="6" :disabled="readonly" class="sb-mono text-xs" :placeholder="t('components.kvString.valuePlaceholder')" />
      <div v-if="!readonly" class="flex flex-wrap items-center gap-2">
        <Button size="sm" :disabled="saving || value === original" @click="save">{{ t('components.kvString.save') }}</Button>
        <div class="ml-auto flex items-center gap-1">
          <Input v-model="delta" type="number" class="w-24" :placeholder="t('components.kvString.incrementPlaceholder')" />
          <Button variant="outline" size="sm" :disabled="incring" @click="incr">INCR</Button>
        </div>
      </div>
    </SbAsyncRegion>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import SbAsyncRegion from '../../../SbAsyncRegion.vue'
import { useKvEditor } from '@/composables/useKvEditor'

const props = defineProps<{
  projectId: string
  kvKey: string
  readonly?: boolean
}>()

const emit = defineEmits<{ changed: [] }>()

const { t } = useI18n()

const value = ref('')
const original = ref('')
const delta = ref('1')
const saving = ref(false)
const incring = ref(false)

// 写回与自增都用命令返回值原地更新，不重新读取（保持输入框内容与光标）。
const { state, load, mutate } = useKvEditor({
  projectId: () => props.projectId,
  kvKey: () => props.kvKey,
  fallback: 'components.kvString.loadFailed',
  read: () => ({ type: 'cmd', argvs: ['GET', props.kvKey] }),
  parse: (r) => (typeof r === 'string' ? r : ''),
  apply: (v) => {
    value.value = v
    original.value = v
  },
  changed: () => emit('changed')
})

async function save() {
  saving.value = true
  try {
    await mutate({ type: 'String', args: { key: props.kvKey, value: value.value, keep_ttl: true } }, {
      failKey: 'components.kvString.saveFailed',
      successKey: 'components.kvString.saved',
      reload: false,
      onSuccess: () => {
        original.value = value.value
      }
    })
  } finally {
    saving.value = false
  }
}

async function incr() {
  const d = Number(delta.value)
  if (!Number.isFinite(d)) return
  incring.value = true
  try {
    await mutate({ type: 'cmd', argvs: ['INCRBY', props.kvKey, String(Math.trunc(d))] }, {
      failKey: 'components.kvString.incrFailed',
      reload: false,
      onSuccess: (r) => {
        value.value = String(r)
        original.value = value.value
      }
    })
  } finally {
    incring.value = false
  }
}
</script>
