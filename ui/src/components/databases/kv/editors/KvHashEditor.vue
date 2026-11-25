<template>
  <div class="flex flex-col gap-3">
    <SbAsyncRegion class="flex flex-col gap-3" block="spinner" :state="state" @retry="load">
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
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import SbAsyncRegion from '../../../SbAsyncRegion.vue'
import { useKvEditor } from '@/composables/useKvEditor'

import ConfirmAction from '../../../ConfirmAction.vue'

const props = defineProps<{
  projectId: string
  kvKey: string
  readonly?: boolean
}>()

const emit = defineEmits<{ changed: [] }>()

const { t } = useI18n()

interface HashField {
  field: string
  value: string
}

const fields = ref<HashField[]>([])
const newField = ref('')
const newValue = ref('')
const editField = ref('')
const editValue = ref('')

const { state, load, mutate } = useKvEditor({
  projectId: () => props.projectId,
  kvKey: () => props.kvKey,
  fallback: 'components.kvHash.loadFailed',
  // HGETALL 返回扁平数组（field,value 交替），转回对象列表。
  read: () => ({ type: 'cmd', argvs: ['HGETALL', props.kvKey] }),
  parse: (flat) => {
    const out: HashField[] = []
    if (Array.isArray(flat)) {
      for (let i = 0; i + 1 < flat.length; i += 2) out.push({ field: String(flat[i]), value: String(flat[i + 1]) })
    }
    return out
  },
  apply: (list) => {
    fields.value = list
  },
  changed: () => emit('changed')
})

function addField() {
  const f = newField.value.trim()
  if (!f) return
  void mutate({ type: 'Hash', args: { key: props.kvKey, fields: { [f]: newValue.value } } }, {
    failKey: 'components.kvHash.saveFailed',
    successKey: 'components.kvHash.saved',
    onSuccess: () => {
      newField.value = ''
      newValue.value = ''
    }
  })
}

function startEdit(f: HashField) {
  editField.value = f.field
  editValue.value = f.value
}

function saveEdit(field: string) {
  void mutate({ type: 'Hash', args: { key: props.kvKey, fields: { [field]: editValue.value } } }, {
    failKey: 'components.kvHash.saveFailed',
    onSuccess: () => {
      editField.value = ''
    }
  })
}

function removeField(field: string) {
  void mutate({ type: 'cmd', argvs: ['HDEL', props.kvKey, field] }, { failKey: 'components.kvHash.deleteFailed' })
}
</script>
