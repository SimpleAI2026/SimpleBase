<template>
  <SbModal
    :open="open"
    :title="server ? t('components.mcpServerModal.editTitle') : t('components.mcpServerModal.createTitle')"
    :description="t('components.mcpServerModal.description')"
    :confirm-loading="saving"
    hide-footer
    @update:open="emit('update:open', $event)"
  >
    <FieldGroup>
      <Field>
        <FieldLabel for="mcp-name">{{ t('components.mcpServerModal.name') }}</FieldLabel>
        <Input
          id="mcp-name"
          :model-value="name"
          placeholder="github"
          @update:model-value="(v) => (name = String(v))"
        />
        <FieldDescription>{{ t('components.mcpServerModal.nameHint') }}</FieldDescription>
      </Field>
      <Field>
        <FieldLabel for="mcp-url">{{ t('components.mcpServerModal.url') }}</FieldLabel>
        <Input
          id="mcp-url"
          :model-value="url"
          placeholder="https://example.com/mcp"
          @update:model-value="(v) => (url = String(v))"
        />
        <FieldDescription>{{ t('components.mcpServerModal.urlHint') }}</FieldDescription>
      </Field>
      <Field>
        <FieldLabel for="mcp-token">{{ t('components.mcpServerModal.credential') }}</FieldLabel>
        <Input
          id="mcp-token"
          :model-value="credential"
          type="password"
          autocomplete="off"
          :placeholder="credentialPlaceholder"
          @update:model-value="(v) => (credential = String(v))"
        />
        <FieldDescription>{{ t('components.mcpServerModal.credentialHint') }}</FieldDescription>
      </Field>
      <Field>
        <FieldLabel for="mcp-timeout">{{ t('components.mcpServerModal.timeout') }}</FieldLabel>
        <Input
          id="mcp-timeout"
          type="number"
          class="w-32"
          :model-value="timeout"
          min="1"
          max="300"
          @update:model-value="(v) => (timeout = Number(v) || 60)"
        />
      </Field>
    </FieldGroup>
    <div class="mt-4 flex justify-end gap-2">
      <Button variant="outline" @click="emit('update:open', false)">{{ t('common.cancel') }}</Button>
      <Button :disabled="saving" @click="save">
        <Spinner v-if="saving" data-icon="inline-start" />
        {{ t('common.save') }}
      </Button>
    </div>
  </SbModal>
</template>

<script setup lang="ts">
/**
 * MCP Server 新建 / 编辑弹窗（planv6.2 §6-7）。
 *
 * 凭据输入框始终为空：列表只拿到 header 名字，不回传原文。
 * 留空保存 = 沿用既有 Authorization 值；填了新值则覆盖。
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { api } from '@/services/api'
import type { McpServer } from '@/services/types'
import { errorMessage } from '@/utils/format'
import SbModal from './SbModal.vue'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'

const props = defineProps<{
  open: boolean
  projectId: string
  /** null 表示新建。 */
  server: McpServer | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  saved: []
}>()

const { t } = useI18n()

const name = ref('')
const url = ref('')
const credential = ref('')
const timeout = ref(60)
const saving = ref(false)

const credentialPlaceholder = computed(() =>
  props.server?.has_credential
    ? t('components.mcpServerModal.credentialSaved')
    : t('components.mcpServerModal.credentialPlaceholder')
)

/** 打开时重置表单：编辑态回填非敏感字段。 */
watch(
  () => [props.open, props.server] as const,
  () => {
    if (!props.open) return
    name.value = props.server?.name || ''
    url.value = props.server?.url || ''
    credential.value = ''
    timeout.value = props.server?.timeout_s || 60
  },
  { immediate: true }
)

async function save() {
  if (saving.value) return
  const trimmedName = name.value.trim()
  const trimmedUrl = url.value.trim()
  if (!trimmedName || !trimmedUrl) {
    toast.error(t('components.mcpServerModal.required'))
    return
  }
  saving.value = true
  try {
    // 凭据留空 = 沿用既有值（不传 headers）；填了才写 Authorization。
    const headers = credential.value.trim()
      ? { Authorization: `Bearer ${credential.value.trim()}` }
      : props.server
        ? undefined
        : {}
    if (props.server) {
      await api.mcpServers.update(props.projectId, props.server.id, {
        name: trimmedName,
        url: trimmedUrl,
        timeout_s: timeout.value,
        ...(headers ? { headers } : {})
      })
    } else {
      await api.mcpServers.create(props.projectId, {
        name: trimmedName,
        url: trimmedUrl,
        timeout_s: timeout.value,
        headers: headers || {},
        enabled: true
      })
    }
    toast.success(t('components.mcpServerModal.saved'))
    emit('saved')
    emit('update:open', false)
  } catch (e) {
    toast.error(errorMessage(e, t('components.mcpServerModal.saveFailed')))
  } finally {
    saving.value = false
  }
}
</script>