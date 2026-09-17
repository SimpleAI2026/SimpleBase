<template>
  <SbModal
    :open="open"
    title="新增文档"
    :width="480"
    :z-index="1100"
    :confirm-loading="submitting"
    :ok-button-props="{ disabled: !canSubmit }"
    @ok="submit"
    @update:open="emit('update:open', $event)"
  >
    <a-form layout="vertical">
      <a-form-item label="Key" required>
        <a-input v-model:value="key" placeholder="字段名，例如 name" @pressEnter="submit" />
      </a-form-item>
      <a-form-item label="Value">
        <a-textarea v-model:value="value" :rows="6" placeholder='字符串，或 JSON（如 1、true、{"city":"SZ"}）' />
      </a-form-item>
    </a-form>
  </SbModal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { api } from '../../services/api'
import SbModal from './SbModal.vue'

const props = defineProps<{
  open: boolean
  projectId: string
  databaseId: string
  collection: string
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  created: []
}>()

const key = ref('')
const value = ref('')
const submitting = ref(false)

const canSubmit = computed(
  () => key.value.trim().length > 0 && !!props.databaseId && !!props.collection
)

function parseValue(raw: string): unknown {
  const t = raw.trim()
  if (!t) return ''
  try {
    return JSON.parse(t)
  } catch {
    return raw
  }
}

watch(
  () => props.open,
  (v) => {
    if (v) {
      key.value = ''
      value.value = ''
      submitting.value = false
    }
  }
)

async function submit() {
  const k = key.value.trim()
  if (!k) {
    message.warning('请输入 Key')
    return
  }
  submitting.value = true
  try {
    const payload: Record<string, unknown> = { [k]: parseValue(value.value) }
    await api.db.insert(props.projectId, props.databaseId, props.collection, payload)
    message.success('创建成功')
    emit('created')
    emit('update:open', false)
  } catch (e) {
    message.error(e instanceof Error ? e.message : '创建失败')
  } finally {
    submitting.value = false
  }
}
</script>
