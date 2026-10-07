<template>
  <SbModal :open="open" title="新建云沙盒" description="沙盒按需启动，空闲后自动回收。" :confirm-loading="saving" :ok-button-props="{ disabled: !valid }" @update:open="$emit('update:open', $event)" @ok="save">
    <div class="flex flex-col gap-4 py-3">
      <label class="flex flex-col gap-1 text-sm">名称（留空自动生成）
        <Input v-model="name" placeholder="etl-check" aria-label="沙盒名称" />
        <span v-if="name && !/^[a-z0-9][a-z0-9-]{0,62}$/.test(name)" class="text-xs text-destructive">仅限小写字母、数字与连字符，最长 63 位</span>
      </label>
      <label class="flex flex-col gap-1 text-sm">镜像
        <select v-model="image" aria-label="沙盒镜像" class="h-9 rounded-md border border-input bg-background px-3 text-sm">
          <option v-for="option in capabilities?.images || []" :key="option" :value="option">{{ option }}</option>
        </select>
      </label>
      <div class="grid grid-cols-2 gap-3">
        <label class="flex flex-col gap-1 text-sm">CPU 核数
          <Input v-model.number="cpus" type="number" min="1" :max="capabilities?.cpusMax || 4" aria-label="CPU 核数" />
        </label>
        <label class="flex flex-col gap-1 text-sm">内存 MiB
          <Input v-model.number="memoryMiB" type="number" min="128" :max="capabilities?.memoryMiBMax || 4096" aria-label="内存 MiB" />
        </label>
      </div>
      <label class="flex flex-col gap-1 text-sm">网络
        <select v-model="network" aria-label="沙盒网络" class="h-9 rounded-md border border-input bg-background px-3 text-sm">
          <option v-for="option in capabilities?.networkOptions || ['none']" :key="option" :value="option">{{ option }}</option>
        </select>
      </label>
      <label class="flex items-center gap-2 text-sm"><input v-model="start" type="checkbox" class="size-4 accent-primary" />立即启动</label>
    </div>
  </SbModal>
</template>
<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { computed, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { api } from '../../services/api'
import type { SandboxCapabilities } from '../../services/types'
import { Input } from '@/components/ui/input'
import SbModal from './SbModal.vue'

const props = defineProps<{ open: boolean; projectId: string; capabilities: SandboxCapabilities | null }>()
const emit = defineEmits<{ 'update:open': [value: boolean]; saved: [] }>()
const name = ref('')
const image = ref('')
const cpus = ref(1)
const memoryMiB = ref(256)
const network = ref('none')
const start = ref(false)
const saving = ref(false)
const valid = computed(() => (!name.value || /^[a-z0-9][a-z0-9-]{0,62}$/.test(name.value)) &&
  cpus.value >= 1 && cpus.value <= (props.capabilities?.cpusMax || 4) &&
  memoryMiB.value >= 128 && memoryMiB.value <= (props.capabilities?.memoryMiBMax || 4096))
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
    await api.sandboxes.create(props.projectId, { name: name.value || undefined, image: image.value,
      cpus: cpus.value, memoryMiB: memoryMiB.value, network: network.value, start: start.value }, crypto.randomUUID())
    emit('update:open', false)
    emit('saved')
    toast.success('云沙盒已创建')
  } catch (e) {
    toast.error(errorMessage(e, '创建云沙盒失败'))
  } finally {
    saving.value = false
  }
}
</script>
