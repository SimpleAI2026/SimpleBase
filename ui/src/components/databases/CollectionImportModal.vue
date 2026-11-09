<template>
  <SbModal :open="open" :title="t('components.collectionImport.title', { collection })" :max-width="640" hide-footer @update:open="emit('update:open', $event)">
    <div class="flex flex-col gap-4">
      <Field>
        <FieldLabel>{{ t('components.collectionImport.file') }}</FieldLabel>
        <input type="file" accept=".csv,.ndjson,.jsonl" class="text-sm" @change="selectFile" />
      </Field>
      <label class="flex items-center gap-2 text-sm"><Switch v-model="dryRun" />{{ t('components.collectionImport.dryRun') }}</label>
      <Button :disabled="!file || running" @click="run">{{ running ? t('common.loading') : t('components.collectionImport.run') }}</Button>
      <p v-if="error" class="text-sm text-destructive">{{ error }}</p>
      <p v-if="result" class="rounded-lg bg-muted p-3 text-sm">
        {{ result.dryRun ? t('components.collectionImport.dryResult', { scanned: result.scanned }) : t('components.collectionImport.result', { imported: result.imported, scanned: result.scanned }) }}
      </p>
    </div>
  </SbModal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { api } from '@/services/api'
import type { CollectionImportResult } from '@/services/types'
import { errorMessage } from '@/utils/format'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Field, FieldLabel } from '@/components/ui/field'
import SbModal from '@/components/modal/SbModal.vue'

const props = defineProps<{ open: boolean; projectId: string; databaseId: string; collection: string }>()
const emit = defineEmits<{ 'update:open': [value: boolean]; saved: [] }>()
const { t } = useI18n()
const file = ref<File | null>(null)
const dryRun = ref(true)
const running = ref(false)
const result = ref<CollectionImportResult | null>(null)
const error = ref('')
const format = computed<'csv' | 'ndjson'>(() => file.value?.name.toLowerCase().endsWith('.csv') ? 'csv' : 'ndjson')

watch(() => props.open, (open) => {
  if (open) {
    file.value = null
    result.value = null
    error.value = ''
    dryRun.value = true
  }
})
function selectFile(event: Event) {
  file.value = (event.target as HTMLInputElement).files?.[0] || null
  result.value = null
}
async function run() {
  if (!file.value) return
  running.value = true
  error.value = ''
  try {
    result.value = await api.db.importCollection(props.projectId, props.databaseId, props.collection, file.value, format.value, dryRun.value)
    if (!dryRun.value) {
      toast.success(t('components.collectionImport.saved'))
      emit('saved')
      emit('update:open', false)
    }
  } catch (reason) {
    error.value = errorMessage(reason, t('components.collectionImport.failed'))
  } finally {
    running.value = false
  }
}
</script>
