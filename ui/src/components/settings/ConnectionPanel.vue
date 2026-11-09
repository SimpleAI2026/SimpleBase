<template>
  <div class="flex w-full flex-col gap-4">
    <Alert v-if="unauthorized" variant="destructive">
      <AlertTitle>{{ t('components.connection.invalidTitle') }}</AlertTitle>
      <AlertDescription>{{ t('components.connection.invalidDesc') }}</AlertDescription>
    </Alert>
    <FieldGroup>
      <Field>
        <FieldLabel for="current-project">{{ t('components.connection.currentProject') }}</FieldLabel>
        <div class="flex gap-2">
          <Input id="current-project" :model-value="projectStore.id || t('components.connection.notSelected')" readonly class="min-w-0 flex-1 font-mono" />
          <Button variant="outline" class="shrink-0" :disabled="!projectStore.id" @click="copyText(projectStore.id, t('components.connection.projectIdLabel'))">{{ t('common.copy') }}</Button>
        </div>
        <FieldDescription>
          {{ t('components.connection.switchProjectHint', { prefix: projectPrefix }) }}
        </FieldDescription>
      </Field>
      <Field>
        <FieldLabel for="api-key">API Key</FieldLabel>
        <div class="flex flex-wrap gap-2">
          <Input
            id="api-key"
            v-model="key"
            type="text"
            placeholder="sb_live_..."
            autocomplete="off"
            class="min-w-0 flex-1 font-mono"
            @keydown.enter="saveKey"
          />
          <Button variant="outline" class="shrink-0" :disabled="!key" @click="copyText(key, t('components.connection.apiKeyLabel'))">{{ t('common.copy') }}</Button>
          <Button class="shrink-0" :disabled="saving" @click="saveAll">
            <Spinner v-if="saving" data-icon="inline-start" />
            {{ t('common.save') }}
          </Button>
          <ConfirmAction
            :title="t('components.connection.resetConfirmTitle')"
            :description="t('components.connection.resetConfirmDesc')"
            :disabled="!canReset || resetting"
            @confirm="resetKey"
          >
            <Button variant="outline" class="shrink-0" :disabled="!canReset || resetting" :title="canReset ? '' : t('components.connection.resetDisabledTip')">
              <Spinner v-if="resetting" data-icon="inline-start" />
              {{ t('components.connection.reset') }}
            </Button>
          </ConfirmAction>
        </div>
        <FieldDescription>
          {{ t('components.connection.apiKeyHint') }}
        </FieldDescription>
      </Field>
    </FieldGroup>
    <div class="flex flex-col gap-3 rounded-lg border border-border p-3">
      <div class="text-sm font-medium">{{ t('components.connection.scopedTitle') }}</div>
      <Field>
        <FieldLabel>{{ t('components.connection.scopedResources') }}</FieldLabel>
        <Input v-model="scopeResources" placeholder="collection:orders,object:public/" />
        <FieldDescription>{{ t('components.connection.csvHint') }}</FieldDescription>
      </Field>
      <Field>
        <FieldLabel>{{ t('components.connection.scopedActions') }}</FieldLabel>
        <Input v-model="scopeActions" placeholder="read,write" />
      </Field>
      <div class="grid gap-3 sm:grid-cols-2">
        <Field>
          <FieldLabel>{{ t('components.connection.hiddenFields') }}</FieldLabel>
          <Input v-model="scopeHidden" />
        </Field>
        <Field>
          <FieldLabel>{{ t('components.connection.readonlyFields') }}</FieldLabel>
          <Input v-model="scopeReadonly" />
        </Field>
      </div>
      <Field>
        <FieldLabel>{{ t('components.connection.expiresAt') }}</FieldLabel>
        <Input v-model="scopeExpires" type="datetime-local" />
      </Field>
      <Button class="self-start" :disabled="issuing || !canReset" @click="issueScoped">
        <Spinner v-if="issuing" data-icon="inline-start" />{{ t('components.connection.issueScoped') }}
      </Button>
      <Field v-if="scopedSecret">
        <FieldLabel>{{ t('components.connection.scopedSecret') }}</FieldLabel>
        <Input :model-value="scopedSecret" readonly class="font-mono" />
      </Field>
    </div>
  </div>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import ConfirmAction from '../ConfirmAction.vue'
import { useAuthStore } from '../../stores/auth'
import { useProjectStore } from '../../stores/project'
import { getApiKey } from '../../services/http'
import { api } from '../../services/api'

const authStore = useAuthStore()
const projectStore = useProjectStore()
const { t } = useI18n()

const unauthorized = computed(() => authStore.lastUnauthorizedAt > 0)

const key = ref(authStore.apiKey)
const saving = ref(false)
const resetting = ref(false)
const issuing = ref(false)
const scopeResources = ref('collection:*')
const scopeActions = ref('read')
const scopeHidden = ref('')
const scopeReadonly = ref('')
const scopeExpires = ref('')
const scopedSecret = ref('')

/** 项目显示名前缀：「显示名；」——显示名与 ID 相同时不重复展示。 */
const projectPrefix = computed(() =>
  projectStore.displayName && projectStore.displayName !== projectStore.id ? `${projectStore.displayName}；` : ''
)

watch(
  () => authStore.settingsOpen,
  (v) => {
    if (v) key.value = getApiKey()
  },
  { immediate: true }
)

function saveKey() {
  if (key.value.trim()) authStore.updateKey(key.value)
}

function saveAll() {
  saving.value = true
  try {
    saveKey()
    toast.success(t('components.connection.saved'))
  } finally {
    saving.value = false
  }
}

function copyText(text: string, label: string) {
  return navigator.clipboard
    .writeText(text)
    .then(() => toast.success(t('components.connection.copied', { label })))
    .catch(() => toast.error(t('components.connection.copyFailed')))
}

/** 登录态 admin 只读；未登录（Key 通道）或 super/user 可重置 */
const canReset = computed(() => {
  if (!projectStore.id) return false
  if (!authStore.user) return true
  return authStore.isSuper || authStore.isUser
})

function scopeList(value: string): string[] {
  return value.split(',').map((item) => item.trim()).filter(Boolean)
}
async function issueScoped() {
  issuing.value = true
  try {
    const created = await api.apiKeys.create(projectStore.id, undefined, {
      resources: scopeList(scopeResources.value),
      actions: scopeList(scopeActions.value),
      hiddenFields: scopeList(scopeHidden.value),
      readonlyFields: scopeList(scopeReadonly.value),
      expiresAt: scopeExpires.value ? new Date(scopeExpires.value).toISOString() : undefined
    })
    scopedSecret.value = created.secret
    toast.success(t('components.connection.scopedCreated'))
  } catch (e) {
    toast.error(errorMessage(e, t('components.connection.scopedFailed')))
  } finally {
    issuing.value = false
  }
}

/**
 * 重置：为当前项目签发新 Key → 吊销旧 Key（仍用旧 Key 鉴权）→ 切换为新 Key。
 * 吊销失败不阻断切换，仅提示。
 */
async function resetKey() {
  const pid = projectStore.id
  if (!pid || !canReset.value || resetting.value) return
  resetting.value = true
  try {
    let oldKeys: { id: string }[] = []
    try {
      oldKeys = await api.apiKeys.list(pid)
    } catch {
      // 列表不可用时只签发，不吊销
    }
    const created = await api.apiKeys.create(pid)
    const results = await Promise.allSettled(
      oldKeys.filter((k) => k.id !== created.id).map((k) => api.apiKeys.revoke(pid, k.id))
    )
    key.value = created.secret
    authStore.updateKey(created.secret)
    if (results.some((r) => r.status === 'rejected')) {
      toast.warning(t('components.connection.revokePartialFailed'))
    } else {
      toast.success(t('components.connection.resetSuccess'))
    }
  } catch (e) {
    toast.error(errorMessage(e, t('components.connection.resetFailed')))
  } finally {
    resetting.value = false
  }
}
</script>
