<template>
  <SbModal
    :open="open"
    :title="t('components.starter.modalTitle')"
    :description="template ? t(template.descKey) : undefined"
    :ok-text="t('components.starter.start')"
    :confirm-loading="launching"
    :ok-button-props="{ disabled: !canSubmit }"
    @ok="submit"
    @update:open="emit('update:open', $event)"
  >
    <FieldGroup>
      <Field :data-invalid="nameError ? true : undefined">
        <FieldLabel for="starter-project-name">{{ t('components.createProject.name') }}</FieldLabel>
        <Input
          id="starter-project-name"
          v-model="name"
          maxlength="128"
          :aria-invalid="nameError ? true : undefined"
          @keydown.enter="submit"
        />
        <FieldDescription v-if="nameError">{{ nameError }}</FieldDescription>
      </Field>
      <Field>
        <FieldLabel for="starter-project-engine">{{ t('components.createProject.engine') }}</FieldLabel>
        <Select v-model="engine">
          <SelectTrigger id="starter-project-engine" class="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value="duckdb">DuckDB</SelectItem>
              <SelectItem value="sqlite">SQLite</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
        <FieldDescription>{{ t('components.createProject.engineHint') }}</FieldDescription>
      </Field>
    </FieldGroup>

    <p class="mt-4 mb-0 text-xs leading-5 text-muted-foreground" data-starter-notice>{{ t('components.starter.newProjectNotice') }}</p>

    <details v-if="template" class="mt-3 rounded-lg border border-border bg-muted/40 px-3 py-2" data-starter-preview>
      <summary class="cursor-pointer text-xs font-medium text-foreground">{{ t('components.starter.previewPrompt') }}</summary>
      <pre class="mt-2 mb-0 max-h-48 overflow-auto text-xs leading-5 whitespace-pre-wrap text-text-secondary">{{ previewText }}</pre>
    </details>
  </SbModal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import SbModal from '@/components/modal/SbModal.vue'
import { findStarterTemplate } from '@/constants/starter-prompts'

const props = defineProps<{
  open: boolean
  templateId: string
  launching?: boolean
  /** 从登录续跑恢复时预填的项目名 / 引擎。 */
  initialName?: string
  initialEngine?: 'duckdb' | 'sqlite'
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  confirm: [payload: { name: string; engine: 'duckdb' | 'sqlite' }]
}>()

const { t } = useI18n()

const template = computed(() => findStarterTemplate(props.templateId))
const name = ref('')
const engine = ref<'duckdb' | 'sqlite'>('duckdb')

/** 预览里的项目上下文占位：真实 ID 在创建后才知道。 */
const previewText = computed(() => {
  const tpl = template.value
  if (!tpl) return ''
  return t(tpl.promptKey, { projectId: '…', siteBaseUrl: '…', publishable: '…' })
})

const nameError = computed(() => {
  const n = name.value.trim()
  if (!n) return ''
  if (n.length > 128) return t('components.createProject.nameTooLong')
  if (/[/\\]/.test(n) || /[\x00-\x1f]/.test(n)) return t('components.createProject.nameInvalid')
  return ''
})

const canSubmit = computed(() => !!template.value && name.value.trim().length > 0 && !nameError.value && !props.launching)

watch(
  () => [props.open, props.templateId] as const,
  ([open]) => {
    if (!open) return
    name.value = props.initialName?.trim() || (template.value ? t(template.value.titleKey) : '')
    engine.value = props.initialEngine === 'sqlite' ? 'sqlite' : 'duckdb'
  },
  { immediate: true }
)

function submit() {
  if (!canSubmit.value) return
  emit('confirm', { name: name.value.trim(), engine: engine.value })
}
</script>
