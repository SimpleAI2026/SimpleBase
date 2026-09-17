<template>
  <SbModal
    :open="open"
    title="新建集合"
    :confirm-loading="submitting"
    :ok-button-props="{ disabled: !canSubmit }"
    @ok="submit"
    @update:open="emit('update:open', $event)"
  >
    <a-input
      v-model:value="name"
      placeholder="例如：users（字母开头，仅字母数字下划线）"
      :status="nameError ? 'error' : ''"
      @pressEnter="submit"
    />
    <div v-if="nameError" class="sb-name-error">{{ nameError }}</div>
  </SbModal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { api } from '../../services/api'
import type { DatabaseItem } from '../../services/api'
import SbModal from './SbModal.vue'

/** 与后端 collectionNamePattern 对齐：字母开头，最多 63 字符 */
const COLLECTION_NAME_RE = /^[A-Za-z][A-Za-z0-9_]{0,62}$/

const props = defineProps<{
  open: boolean
  projectId: string
  database: DatabaseItem | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  created: [name: string]
}>()

const name = ref('')
const submitting = ref(false)

const nameError = computed(() => {
  const n = name.value.trim()
  if (!n) return ''
  if (!COLLECTION_NAME_RE.test(n)) {
    return '集合名称须以字母开头，仅含字母、数字和下划线'
  }
  return ''
})

const canSubmit = computed(() => name.value.trim().length > 0 && !nameError.value && !!props.database)

watch(
  () => props.open,
  (v) => {
    if (v) {
      name.value = ''
      submitting.value = false
    }
  }
)

async function submit() {
  if (!canSubmit.value || !props.database) {
    if (!name.value.trim()) message.warning('请输入集合名称')
    return
  }
  submitting.value = true
  try {
    const trimmed = name.value.trim()
    await api.db.createCollection(props.projectId, props.database.id, trimmed)
    message.success('集合创建成功')
    emit('created', trimmed)
    emit('update:open', false)
  } catch (e) {
    message.error(e instanceof Error ? e.message : '集合创建失败')
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.sb-name-error {
  color: var(--sb-danger);
  font-size: var(--sb-fs-xs);
  margin-top: var(--sb-space-2);
}
</style>
