<template>
  <div class="flex flex-col gap-3">
    <SbAsyncRegion class="flex flex-col gap-3" block="spinner" :state="state" @retry="load">
      <div class="overflow-x-auto rounded-md border border-border/70">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="w-14">#</TableHead>
              <TableHead class="min-w-40 text-left">{{ t('components.kvList.colElement') }}</TableHead>
              <TableHead v-if="!readonly" class="w-20">{{ t('components.kvList.colActions') }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableEmpty v-if="!elems.length" :colspan="readonly ? 2 : 3">{{ t('components.kvList.empty') }}</TableEmpty>
            <TableRow v-for="(el, i) in elems" :key="i">
              <TableCell class="text-xs text-muted-foreground tabular-nums">{{ i }}</TableCell>
              <TableCell class="text-left">
                <span v-if="editIndex !== i" class="sb-mono text-xs break-all">{{ el }}</span>
                <div v-else class="flex items-center gap-1">
                  <Input v-model="editValue" class="h-7 text-xs" @keydown.enter="saveEdit(i)" />
                  <Button size="sm" variant="ghost" @click="saveEdit(i)">{{ t('components.kvList.saveCell') }}</Button>
                </div>
              </TableCell>
              <TableCell v-if="!readonly">
                <Button size="sm" variant="ghost" @click="startEdit(i, el)">{{ t('components.kvList.editCell') }}</Button>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>
      <div v-if="!readonly" class="flex flex-wrap items-center gap-2">
        <Input v-model="pushValue" :placeholder="t('components.kvList.pushPlaceholder')" class="min-w-0 flex-1 sm:w-48 sm:flex-none" @keydown.enter="push('back')" />
        <Button size="sm" variant="outline" :disabled="!pushValue" @click="push('front')">{{ t('components.kvList.pushFront') }}</Button>
        <Button size="sm" variant="outline" :disabled="!pushValue" @click="push('back')">{{ t('components.kvList.pushBack') }}</Button>
        <div class="ml-auto flex items-center gap-1">
          <Button size="sm" variant="ghost" :disabled="!elems.length" @click="pop('front')">{{ t('components.kvList.popFront') }}</Button>
          <Button size="sm" variant="ghost" :disabled="!elems.length" @click="pop('back')">{{ t('components.kvList.popBack') }}</Button>
        </div>
      </div>
    </SbAsyncRegion>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import SbAsyncRegion from '../../../SbAsyncRegion.vue'
import { useKvEditor } from '@/composables/useKvEditor'

const props = defineProps<{
  projectId: string
  kvKey: string
  readonly?: boolean
}>()

const emit = defineEmits<{ changed: [] }>()

const { t } = useI18n()

const elems = ref<string[]>([])
const pushValue = ref('')
const editIndex = ref(-1)
const editValue = ref('')

const { state, load, mutate } = useKvEditor({
  projectId: () => props.projectId,
  kvKey: () => props.kvKey,
  fallback: 'components.kvList.loadFailed',
  // v1 展示前 200 个元素
  read: () => ({ type: 'cmd', argvs: ['LRANGE', props.kvKey, '0', '199'] }),
  parse: (r) => (Array.isArray(r) ? (r as string[]) : []),
  apply: (list) => {
    elems.value = list
  },
  changed: () => emit('changed')
})

function push(side: 'front' | 'back') {
  if (!pushValue.value) return
  void mutate({ type: 'List', args: { key: props.kvKey, elems: [pushValue.value], side } }, {
    failKey: 'components.kvList.insertFailed',
    onSuccess: () => {
      pushValue.value = ''
    }
  })
}

function pop(side: 'front' | 'back') {
  void mutate({ type: 'cmd', argvs: [side === 'front' ? 'LPOP' : 'RPOP', props.kvKey] }, {
    failKey: 'components.kvList.popFailed',
    onSuccess: (r) => toast.success(t('components.kvList.popped', { value: r ?? '' }))
  })
}

function startEdit(i: number, el: string) {
  editIndex.value = i
  editValue.value = el
}

function saveEdit(i: number) {
  void mutate({ type: 'cmd', argvs: ['LSET', props.kvKey, String(i), editValue.value] }, {
    failKey: 'components.kvList.saveFailed',
    onSuccess: () => {
      editIndex.value = -1
    }
  })
}
</script>
