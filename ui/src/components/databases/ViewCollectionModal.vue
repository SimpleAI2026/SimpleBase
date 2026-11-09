<template>
  <SbModal :open="open" :title="t('components.viewCollection.title')" :max-width="900" hide-footer @update:open="emit('update:open', $event)">
    <div class="flex flex-col gap-4">
      <Field>
        <FieldLabel>{{ t('components.viewCollection.name') }}</FieldLabel>
        <Input v-model="name" @update:model-value="preview = null" />
      </Field>
      <Field>
        <FieldLabel>{{ t('components.viewCollection.query') }}</FieldLabel>
        <Textarea v-model="query" rows="7" class="font-mono" @update:model-value="preview = null" />
      </Field>
      <div class="flex gap-2">
        <Button variant="outline" :disabled="running || !name.trim() || !query.trim()" @click="dryRun">{{ t('components.viewCollection.dryRun') }}</Button>
        <Button :disabled="!preview || running" @click="save">{{ t('common.save') }}</Button>
      </div>
      <p v-if="error" class="text-sm text-destructive">{{ error }}</p>
      <div v-if="preview" class="rounded-lg border p-3">
        <p class="mb-2 text-sm font-medium">{{ t('components.viewCollection.preview', { count: preview.scanned }) }}</p>
        <div class="mb-3 flex flex-wrap gap-2">
          <span v-for="field in preview.fields" :key="field.name" class="rounded bg-muted px-2 py-1 text-xs">{{ field.name }} · {{ field.type }}</span>
        </div>
        <SbCodeBlock :value="preview.items" max-height="20rem" />
      </div>
    </div>
  </SbModal>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { api } from '@/services/api'
import type { ViewDryRunResult } from '@/services/types'
import { errorMessage } from '@/utils/format'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Field, FieldLabel } from '@/components/ui/field'
import SbModal from '@/components/modal/SbModal.vue'
import SbCodeBlock from '@/components/SbCodeBlock.vue'

const props = defineProps<{ open: boolean; projectId: string; databaseId: string }>()
const emit = defineEmits<{ 'update:open': [value: boolean]; saved: [] }>()
const { t } = useI18n()
const name = ref('')
const query = ref('SELECT id, data, created_at, updated_at FROM collection_name')
const preview = ref<ViewDryRunResult | null>(null)
const running = ref(false)
const error = ref('')

watch(() => props.open, (open) => {
  if (open) {
    name.value = ''
    preview.value = null
    error.value = ''
  }
})
async function dryRun() {
  running.value = true
  error.value = ''
  try {
    preview.value = await api.db.dryRunView(props.projectId, props.databaseId, name.value.trim(), query.value)
  } catch (reason) {
    preview.value = null
    error.value = errorMessage(reason, t('components.viewCollection.failed'))
  } finally {
    running.value = false
  }
}
async function save() {
  if (!preview.value) return
  running.value = true
  try {
    await api.db.putCollectionSchema(props.projectId, props.databaseId, {
      name: name.value.trim(), schemaMode: 'view', viewQuery: query.value, fields: preview.value.fields
    })
    toast.success(t('components.viewCollection.saved'))
    emit('saved')
    emit('update:open', false)
  } catch (reason) {
    error.value = errorMessage(reason, t('components.viewCollection.failed'))
  } finally {
    running.value = false
  }
}
</script>
