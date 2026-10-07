<template>
  <div class="flex flex-col gap-3">
    <div v-if="loading" class="flex justify-center py-6"><Spinner /></div>
    <template v-else>
      <div class="overflow-x-auto rounded-md border border-border/70">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="min-w-32">字段</TableHead>
              <TableHead class="min-w-40">值</TableHead>
              <TableHead v-if="!readonly" class="w-28 text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableEmpty v-if="!fields.length" :colspan="readonly ? 2 : 3">暂无字段</TableEmpty>
            <TableRow v-for="f in fields" :key="f.field">
              <TableCell class="sb-mono font-medium">{{ f.field }}</TableCell>
              <TableCell>
                <span v-if="editField !== f.field" class="sb-mono text-xs break-all">{{ f.value }}</span>
                <div v-else class="flex items-center gap-1">
                  <Input v-model="editValue" class="h-7 text-xs" @keydown.enter="saveEdit(f.field)" />
                  <Button size="sm" variant="ghost" @click="saveEdit(f.field)">存</Button>
                </div>
              </TableCell>
              <TableCell v-if="!readonly" class="text-right">
                <div class="flex justify-end gap-1">
                  <Button size="sm" variant="ghost" @click="startEdit(f)">改</Button>
                  <ConfirmAction title="删除该字段？" @confirm="removeField(f.field)">
                    <Button size="sm" variant="destructiveGhost">删</Button>
                  </ConfirmAction>
                </div>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>
      <div v-if="!readonly" class="flex items-end gap-2">
        <Input v-model="newField" placeholder="字段名" class="w-36" />
        <Input v-model="newValue" placeholder="值" class="flex-1" />
        <Button size="sm" :disabled="!newField.trim()" @click="addField">添加</Button>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Table, TableBody, TableCell, TableEmpty, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { api } from '../../../../services/api'
import ConfirmAction from '../../../ConfirmAction.vue'

const props = defineProps<{
  projectId: string
  kvKey: string
  readonly?: boolean
}>()

const emit = defineEmits<{ changed: [] }>()

const fields = ref<{ field: string; value: string }[]>([])
const loading = ref(false)
const newField = ref('')
const newValue = ref('')
const editField = ref('')
const editValue = ref('')

async function load() {
  loading.value = true
  try {
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
  } catch (e) {
    toast.error(errorMessage(e, '加载失败'))
  } finally {
    loading.value = false
  }
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
    toast.success('已保存')
    await load()
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e, '保存失败'))
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
    toast.error(errorMessage(e, '保存失败'))
  }
}

async function removeField(field: string) {
  try {
    await api.kv.exec(props.projectId, { type: 'cmd', argvs: ['HDEL', props.kvKey, field] })
    await load()
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e, '删除失败'))
  }
}

watch(() => props.kvKey, () => void load(), { immediate: true })
</script>
