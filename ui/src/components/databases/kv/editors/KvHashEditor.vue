<template>
  <div class="flex flex-col gap-3">
    <SbAsyncRegion
      class="flex flex-col gap-3"
      block="spinner"
      :pending="pending"
      :show-skeleton="showSkeleton"
      :show-empty="false"
      :show-error="showError"
      :refreshing="refreshing"
      :error="error"
      @retry="load"
    >
      <div class="overflow-x-auto rounded-md border border-border/70">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="min-w-32">{{ t('components.kvHash.colField') }}</TableHead>
              <TableHead class="min-w-40 text-left">{{ t('components.kvHash.colValue') }}</TableHead>
              <TableHead v-if="!readonly" class="w-28">{{ t('components.kvHash.colActions') }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableEmpty v-if="!fields.length" :colspan="readonly ? 2 : 3">{{ t('components.kvHash.empty') }}</TableEmpty>
            <TableRow v-for="f in fields" :key="f.field">
              <TableCell class="sb-mono font-medium">{{ f.field }}</TableCell>
              <TableCell class="text-left">
                <span v-if="editField !== f.field" class="sb-mono text-xs break-all">{{ f.value }}</span>
                <div v-else class="flex items-center gap-1">
                  <Input v-model="editValue" class="h-7 text-xs" @keydown.enter="saveEdit(f.field)" />
                  <Button size="sm" variant="ghost" @click="saveEdit(f.field)">{{ t('components.kvHash.saveCell') }}</Button>
                </div>
              </TableCell>
              <TableCell v-if="!readonly">
                <div class="flex justify-center gap-1">
                  <Button size="sm" variant="ghost" @click="startEdit(f)">{{ t('components.kvHash.editCell') }}</Button>
                  <ConfirmAction :title="t('components.kvHash.deleteConfirm')" @confirm="removeField(f.field)">
                    <Button size="sm" variant="destructiveGhost">{{ t('components.kvHash.deleteCell') }}</Button>
                  </ConfirmAction>
                </div>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>
      <div v-if="!readonly" class="flex flex-wrap items-center gap-2">
        <Input v-model="newField" :placeholder="t('components.kvHash.fieldPlaceholder')" class="min-w-32 flex-1 sm:w-36 sm:flex-none" />
        <Input v-model="newValue" :placeholder="t('components.kvHash.valuePlaceholder')" class="min-w-32 flex-1" />
        <Button size="sm" class="shrink-0" :disabled="!newField.trim()" @click="addField">{{ t('components.kvHash.add') }}</Button>
      </div>
    </SbAsyncRegion>
  </div>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableEmpty, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { api } from '../../../../services/api'
import SbAsyncRegion from '../../../SbAsyncRegion.vue'
import { useLoadState } from '@/composables/useLoadState'
import ConfirmAction from '../../../ConfirmAction.vue'

const props = defineProps<{
  projectId: string
  kvKey: string
  readonly?: boolean
}>()

const emit = defineEmits<{ changed: [] }>()

const { t } = useI18n()

const fields = ref<{ field: string; value: string }[]>([])
const newField = ref('')
const { pending, showSkeleton, showError, refreshing, error, run } = useLoadState({ fallback: 'components.kvHash.loadFailed' })
const newValue = ref('')
const editField = ref('')
const editValue = ref('')

function load() {
  return run(async () => {
    // HGETALL 返回扁平数组（field,value 交替），转回对象列表
    const flat = (await api.kv.exec(props.projectId, {
      type: 'cmd',
      argvs: ['HGETALL', props.kvKey]
    })) as string[]
    const out: { field: string; value: string }[] = []
    if (Array.isArray(flat)) {
      for (let i = 0; i + 1 < flat.length; i += 2) out.push({ field: flat[i], value: flat[i + 1] })
    }
    fields.value = out
    return true
  })
}

async function addField() {
  const f = newField.value.trim()
  if (!f) return
  try {
    await api.kv.exec(props.projectId, {
      type: 'Hash',
      args: { key: props.kvKey, fields: { [f]: newValue.value } }
    })
    newField.value = ''
    newValue.value = ''
    toast.success(t('components.kvHash.saved'))
    await load()
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e, t('components.kvHash.saveFailed')))
  }
}

function startEdit(f: { field: string; value: string }) {
  editField.value = f.field
  editValue.value = f.value
}

async function saveEdit(field: string) {
  try {
    await api.kv.exec(props.projectId, {
      type: 'Hash',
      args: { key: props.kvKey, fields: { [field]: editValue.value } }
    })
    editField.value = ''
    await load()
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e, t('components.kvHash.saveFailed')))
  }
}

async function removeField(field: string) {
  try {
    await api.kv.exec(props.projectId, { type: 'cmd', argvs: ['HDEL', props.kvKey, field] })
    await load()
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e, t('components.kvHash.deleteFailed')))
  }
}

watch(() => props.kvKey, () => void load(), { immediate: true })
</script>
