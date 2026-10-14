<template>
  <div class="flex flex-col gap-4 max-w-lg">
    <Alert v-if="error" variant="destructive">
      <AlertTitle>{{ t('components.storage.errorTitle') }}</AlertTitle>
      <AlertDescription>{{ error }}</AlertDescription>
    </Alert>

    <div v-if="loading" class="flex items-center gap-2 text-sm text-muted-foreground">
      <Spinner data-icon="inline-start" />
      {{ t('common.loading') }}
    </div>

    <template v-else-if="settings">
      <Alert v-if="!settings.writable_config" variant="default">
        <AlertTitle>{{ t('components.storage.readonlyConfigTitle') }}</AlertTitle>
        <AlertDescription>{{ t('components.storage.readonlyConfigDesc') }}</AlertDescription>
      </Alert>

      <Alert v-if="settings.data_present.local || settings.data_present.s3" variant="default">
        <AlertTitle>{{ t('components.storage.dataPresentTitle') }}</AlertTitle>
        <AlertDescription>{{ t('components.storage.dataPresentDesc') }}</AlertDescription>
      </Alert>

      <FieldGroup>
        <Field>
          <FieldLabel for="storage-database">{{ t('components.storage.databaseMode') }}</FieldLabel>
          <Select v-model="database" :disabled="!canEdit || sourceOf('database') === 'env'">
            <SelectTrigger id="storage-database" class="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="local">{{ t('components.storage.modeLocal') }}</SelectItem>
              <SelectItem value="s3">{{ t('components.storage.modeS3') }}</SelectItem>
            </SelectContent>
          </Select>
          <FieldDescription>
            {{ sourceOf('database') === 'env' ? t('components.storage.lockedByEnv') : t('components.storage.databaseHint') }}
          </FieldDescription>
        </Field>

        <Field>
          <FieldLabel for="storage-files">{{ t('components.storage.filesMode') }}</FieldLabel>
          <Select v-model="files" :disabled="!canEdit || sourceOf('files') === 'env'">
            <SelectTrigger id="storage-files" class="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="local">{{ t('components.storage.modeLocal') }}</SelectItem>
              <SelectItem value="s3">{{ t('components.storage.modeS3') }}</SelectItem>
            </SelectContent>
          </Select>
          <FieldDescription>
            {{ sourceOf('files') === 'env' ? t('components.storage.lockedByEnv') : t('components.storage.filesHint') }}
          </FieldDescription>
        </Field>

        <Field>
          <FieldLabel for="storage-data-dir">{{ t('components.storage.dataDir') }}</FieldLabel>
          <Input
            id="storage-data-dir"
            v-model="dataDir"
            type="text"
            class="font-mono"
            :disabled="!canEdit || sourceOf('data_dir') === 'env'"
          />
          <FieldDescription>
            {{ sourceOf('data_dir') === 'env' ? t('components.storage.lockedByEnv') : t('components.storage.dataDirHint') }}
          </FieldDescription>
        </Field>

        <Field>
          <FieldLabel for="storage-cache-dir">{{ t('components.storage.cacheDir') }}</FieldLabel>
          <Input id="storage-cache-dir" :model-value="settings.cache_dir" type="text" readonly class="font-mono" />
          <FieldDescription>{{ t('components.storage.cacheDirHint') }}</FieldDescription>
        </Field>

        <Field v-if="settings.bucket">
          <FieldLabel for="storage-bucket">{{ t('components.storage.bucket') }}</FieldLabel>
          <Input
            id="storage-bucket"
            :model-value="`${settings.bucket}/${settings.prefix}`"
            type="text"
            readonly
            class="font-mono"
          />
          <FieldDescription>{{ t('components.storage.bucketHint') }}</FieldDescription>
        </Field>
      </FieldGroup>

      <div v-if="canEdit" class="flex flex-wrap gap-2">
        <ConfirmAction
          :title="t('components.storage.confirmTitle')"
          :description="t('components.storage.confirmDesc')"
          :disabled="saving || !dirty"
          @confirm="save"
        >
          <Button :disabled="saving || !dirty">
            <Spinner v-if="saving" data-icon="inline-start" />
            {{ t('common.save') }}
          </Button>
        </ConfirmAction>
      </div>

      <Alert v-if="updated" variant="default">
        <AlertTitle>{{ t('components.storage.updatedTitle') }}</AlertTitle>
        <AlertDescription>
          {{ t('components.storage.updatedDesc') }}
          <code class="rounded-sm bg-muted px-1 font-mono text-xs">./build.sh reset</code>
          →
          <code class="rounded-sm bg-muted px-1 font-mono text-xs">./build.sh dev</code>
        </AlertDescription>
      </Alert>
    </template>
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import ConfirmAction from '../ConfirmAction.vue'
import { useAuthStore } from '../../stores/auth'
import { api, type StorageSettings } from '../../services/api'

const authStore = useAuthStore()
const { t } = useI18n()

const loading = ref(false)
const saving = ref(false)
const error = ref('')
const updated = ref(false)
const settings = ref<StorageSettings | null>(null)

const database = ref('local')
const files = ref('local')
const dataDir = ref('')

/** 仅超级管理员可改（与后端 PUT /v1/system/storage 一致）。 */
const canEdit = computed(() => Boolean(authStore.user && authStore.isSuper))

const dirty = computed(() => {
  const s = settings.value
  if (!s) return false
  return database.value !== s.database || files.value !== s.files || dataDir.value !== s.data_dir
})

function sourceOf(key: string): string {
  return settings.value?.source?.[key] || 'default'
}

async function load(keepUpdated = false) {
  loading.value = true
  error.value = ''
  if (!keepUpdated) updated.value = false
  try {
    const s = await api.storage.get()
    settings.value = s
    database.value = s.database
    files.value = s.files
    dataDir.value = s.data_dir
  } catch (e) {
    error.value = errorMessage(e, t('components.storage.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function save() {
  if (saving.value || !dirty.value) return
  saving.value = true
  error.value = ''
  try {
    await api.storage.put({
      database: database.value,
      files: files.value,
      data_dir: dataDir.value,
      confirm_reset: true
    })
    updated.value = true
    toast.success(t('components.storage.saved'))
    // 重载配置展示最新值，同时保留“需重启”指引。
    await load(true)
  } catch (e) {
    error.value = errorMessage(e, t('components.storage.saveFailed'))
  } finally {
    saving.value = false
  }
}

watch(
  () => authStore.settingsOpen,
  (v) => {
    if (v) void load()
  },
  { immediate: true }
)
</script>
