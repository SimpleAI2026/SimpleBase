<template>
  <SbModal
    :open="open"
    title="新建项目"
    :confirm-loading="submitting"
    :ok-button-props="{ disabled: !canSubmit }"
    @ok="submit"
    @update:open="emit('update:open', $event)"
  >
    <FieldGroup>
      <Field :data-invalid="nameError ? true : undefined">
        <FieldLabel for="project-name">项目名称</FieldLabel>
        <Input
          id="project-name"
          v-model="name"
          placeholder="例如：商城后台"
          maxlength="128"
          :aria-invalid="nameError ? true : undefined"
          @keydown.enter="submit"
        />
        <FieldDescription v-if="nameError">{{ nameError }}</FieldDescription>
      </Field>
      <Field :data-invalid="idError ? true : undefined">
        <FieldLabel for="project-id">项目 ID</FieldLabel>
        <Input
          id="project-id"
          v-model="customId"
          placeholder="可选 UUID"
          class="font-mono"
          :aria-invalid="idError ? true : undefined"
          @keydown.enter="submit"
        />
        <FieldDescription>{{ idError || '可选。留空由服务端生成 UUID。' }}</FieldDescription>
      </Field>
    </FieldGroup>
  </SbModal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { api } from '../../services/api'
import type { ProjectItem } from '../../services/types'
import SbModal from './SbModal.vue'

const UUID_RE = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  created: [project: ProjectItem]
}>()

const name = ref('')
const customId = ref('')
const submitting = ref(false)

const nameError = computed(() => {
  const n = name.value.trim()
  if (!n) return ''
  if (n.length > 128) return '名称不能超过 128 个字符'
  if (/[/\\]/.test(n) || /[\x00-\x1f]/.test(n)) return '名称不能包含路径分隔符或控制字符'
  return ''
})

const idError = computed(() => {
  const id = customId.value.trim()
  if (!id) return ''
  if (!UUID_RE.test(id)) return '须为合法 UUID'
  return ''
})

const canSubmit = computed(() => name.value.trim().length > 0 && !nameError.value && !idError.value)

watch(
  () => props.open,
  (v) => {
    if (v) {
      name.value = ''
      customId.value = ''
      submitting.value = false
    }
  }
)

async function submit() {
  if (!canSubmit.value) {
    if (!name.value.trim()) toast.warning('请输入项目名称')
    return
  }
  submitting.value = true
  try {
    const created = await api.projects.create({
      name: name.value.trim(),
      id: customId.value.trim() || undefined
    })
    toast.success(`已创建项目「${created.name || created.id}」`)
    emit('created', created)
    emit('update:open', false)
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '创建项目失败')
  } finally {
    submitting.value = false
  }
}
</script>
