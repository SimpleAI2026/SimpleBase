<template>
  <!-- 会话重命名：会话操作收进「⋯」菜单。 -->
  <SbModal
    :open="open"
    :title="t('pages.agents.renameThreadTitle')"
    :max-width="440"
    :confirm-loading="saving"
    :ok-button-props="{ disabled: !modelValue.trim() }"
    @ok="emit('submit')"
    @update:open="(v: boolean) => emit('update:open', v)"
  >
    <FieldGroup>
      <Field>
        <FieldLabel for="thread-rename">{{ t('pages.agents.renameThreadLabel') }}</FieldLabel>
        <Input
          id="thread-rename"
          :model-value="modelValue"
          :placeholder="t('pages.agents.renameThreadPlaceholder')"
          @update:model-value="(v: string | number) => emit('update:modelValue', String(v))"
          @keydown.enter="emit('submit')"
        />
      </Field>
    </FieldGroup>
  </SbModal>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import SbModal from '@/components/modal/SbModal.vue'

defineProps<{
  open: boolean
  /** 标题输入框的值（v-model）。 */
  modelValue: string
  saving?: boolean
}>()
const emit = defineEmits<{
  'update:open': [value: boolean]
  'update:modelValue': [value: string]
  submit: []
}>()
const { t } = useI18n()
</script>
