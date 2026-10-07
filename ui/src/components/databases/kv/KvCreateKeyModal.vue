<template>
  <SbModal
    :open="open"
    title="新建 Key"
    :width="520"
    :confirm-loading="submitting"
    :ok-button-props="{ disabled: !canSubmit }"
    ok-text="创建"
    @ok="submit"
    @update:open="emit('update:open', $event)"
  >
    <FieldGroup>
      <Field>
        <FieldLabel for="kv-create-key">Key 名称</FieldLabel>
        <Input id="kv-create-key" v-model="keyName" placeholder="如 session:1001" />
      </Field>
      <Field>
        <FieldLabel>类型</FieldLabel>
        <Select v-model="keyType">
          <SelectTrigger class="w-full">
            <SelectValue placeholder="选择类型" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="string">String</SelectItem>
            <SelectItem value="hash">Hash</SelectItem>
            <SelectItem value="list">List</SelectItem>
            <SelectItem value="set">Set</SelectItem>
            <SelectItem value="zset">ZSet</SelectItem>
          </SelectContent>
        </Select>
      </Field>

      <Field v-if="keyType === 'string'">
        <FieldLabel for="kv-create-string">值</FieldLabel>
        <Textarea id="kv-create-string" v-model="stringValue" :rows="3" placeholder="文本内容" />
      </Field>

      <template v-if="keyType === 'hash'">
        <Field v-for="(pair, i) in hashPairs" :key="i">
          <FieldLabel v-if="i === 0">初始字段</FieldLabel>
          <div class="flex items-center gap-2">
            <Input v-model="pair.field" placeholder="字段名" class="flex-1" />
            <Input v-model="pair.value" placeholder="值" class="flex-1" />
            <Button
              v-if="hashPairs.length > 1"
              variant="ghost"
              size="sm"
              @click="hashPairs.splice(i, 1)"
            >删</Button>
          </div>
        </Field>
        <Button variant="outline" size="sm" class="self-start" @click="hashPairs.push({ field: '', value: '' })">
          + 添加字段
        </Button>
      </template>

      <Field v-if="keyType === 'list'">
        <FieldLabel for="kv-create-list">初始元素（每行一个，入尾）</FieldLabel>
        <Textarea id="kv-create-list" v-model="listText" :rows="3" placeholder="a&#10;b&#10;c" />
      </Field>

      <Field v-if="keyType === 'set'">
        <FieldLabel for="kv-create-set">初始成员（每行一个）</FieldLabel>
        <Textarea id="kv-create-set" v-model="setText" :rows="3" placeholder="tag-a&#10;tag-b" />
      </Field>

      <template v-if="keyType === 'zset'">
        <Field v-for="(pair, i) in zsetPairs" :key="i">
          <FieldLabel v-if="i === 0">初始成员</FieldLabel>
          <div class="flex items-center gap-2">
            <Input v-model="pair.elem" placeholder="成员" class="flex-1" />
            <Input v-model="pair.score" placeholder="分数" type="number" class="w-28" />
            <Button
              v-if="zsetPairs.length > 1"
              variant="ghost"
              size="sm"
              @click="zsetPairs.splice(i, 1)"
            >删</Button>
          </div>
        </Field>
        <Button variant="outline" size="sm" class="self-start" @click="zsetPairs.push({ elem: '', score: '' })">
          + 添加成员
        </Button>
      </template>

      <Field>
        <FieldLabel>过期时间</FieldLabel>
        <div class="flex items-center gap-2">
          <Select v-model="ttlMode">
            <SelectTrigger class="w-36">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="none">永久</SelectItem>
              <SelectItem value="custom">自定义</SelectItem>
            </SelectContent>
          </Select>
          <Input
            v-if="ttlMode === 'custom'"
            v-model="ttlSeconds"
            type="number"
            min="1"
            class="w-40"
            placeholder="秒"
          />
          <span v-if="ttlMode === 'custom'" class="text-xs text-muted-foreground">秒</span>
        </div>
      </Field>
    </FieldGroup>
  </SbModal>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { computed, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { api } from '../../../services/api'
import SbModal from '../../modal/SbModal.vue'

const props = defineProps<{
  open: boolean
  projectId: string
  readonly?: boolean
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
  created: [key: string]
}>()

const keyName = ref('')
const keyType = ref<'string' | 'hash' | 'list' | 'set' | 'zset'>('string')
const stringValue = ref('')
const hashPairs = ref([{ field: '', value: '' }])
const listText = ref('')
const setText = ref('')
const zsetPairs = ref([{ elem: '', score: '' }])
const ttlMode = ref<'none' | 'custom'>('none')
const ttlSeconds = ref('')
const submitting = ref(false)

watch(
  () => props.open,
  (v) => {
    if (v) {
      keyName.value = ''
      keyType.value = 'string'
      stringValue.value = ''
      hashPairs.value = [{ field: '', value: '' }]
      listText.value = ''
      setText.value = ''
      zsetPairs.value = [{ elem: '', score: '' }]
      ttlMode.value = 'none'
      ttlSeconds.value = ''
    }
  }
)

const canSubmit = computed(() => {
  if (!keyName.value.trim()) return false
  if (ttlMode.value === 'custom') {
    const s = Number(ttlSeconds.value)
    if (!Number.isFinite(s) || s <= 0) return false
  }
  switch (keyType.value) {
    case 'hash':
      return hashPairs.value.some((p) => p.field.trim())
    case 'list':
      return listText.value.trim().length > 0
    case 'set':
      return setText.value.trim().length > 0
    case 'zset':
      return zsetPairs.value.some((p) => p.elem.trim() && p.score !== '' && Number.isFinite(Number(p.score)))
    default:
      return true // string 允许空值
  }
})

function lines(text: string): string[] {
  return text.split('\n').map((s) => s.trim()).filter((s) => s.length > 0)
}

async function submit() {
  const key = keyName.value.trim()
  if (!key) return
  submitting.value = true
  try {
    const pid = props.projectId
    // 类型化写入统一带 ttl_ms（0 之外的正数相对过期），不再区分命令补 expire
    const ttlMs = ttlMode.value === 'custom' ? Number(ttlSeconds.value) * 1000 : undefined
    const common = ttlMs !== undefined ? { ttl_ms: ttlMs } : {}
    switch (keyType.value) {
      case 'string': {
        await api.kv.exec(pid, {
          type: 'String',
          args: { key, value: stringValue.value, ...common }
        })
        break
      }
      case 'hash': {
        const fields: Record<string, string> = {}
        for (const p of hashPairs.value) {
          if (p.field.trim()) fields[p.field.trim()] = p.value
        }
        await api.kv.exec(pid, { type: 'Hash', args: { key, fields, ...common } })
        break
      }
      case 'list':
        await api.kv.exec(pid, {
          type: 'List',
          args: { key, elems: lines(listText.value), side: 'back', ...common }
        })
        break
      case 'set':
        await api.kv.exec(pid, { type: 'Set', args: { key, elems: lines(setText.value), ...common } })
        break
      case 'zset':
        await api.kv.exec(pid, {
          type: 'ZSet',
          args: {
            key,
            items: zsetPairs.value
              .filter((p) => p.elem.trim() && p.score !== '')
              .map((p) => ({ elem: p.elem.trim(), score: Number(p.score) })),
            ...common
          }
        })
        break
    }
    toast.success('创建成功')
    emit('update:open', false)
    emit('created', key)
  } catch (e) {
    toast.error(errorMessage(e, '创建失败'))
  } finally {
    submitting.value = false
  }
}
</script>
