<template>
  <SbModal
    :open="open"
    :title="user ? (readonly ? '用户详情' : '编辑用户') : '新建用户'"
    :confirm-loading="submitting"
    :ok-text="user ? '保存' : '创建'"
    :ok-button-props="{ disabled: readonly || !canSubmit }"
    :hide-footer="readonly"
    @ok="submit"
    @update:open="emit('update:open', $event)"
  >
    <FieldGroup>
      <Field>
        <FieldLabel for="uf-username">用户名</FieldLabel>
        <Input
          id="uf-username"
          v-model="username"
          :disabled="Boolean(user)"
          placeholder="登录用户名"
          autocomplete="off"
        />
        <FieldDescription>{{ user ? '用户名不可修改' : '登录名，大小写不敏感' }}</FieldDescription>
      </Field>
      <Field>
        <FieldLabel for="uf-display">显示名</FieldLabel>
        <Input id="uf-display" v-model="displayName" :disabled="readonly" placeholder="可选" />
      </Field>
      <Field>
        <FieldLabel for="uf-role">角色</FieldLabel>
        <Select v-model="role" :disabled="readonly || isProtected">
          <SelectTrigger id="uf-role" class="w-full">
            <SelectValue placeholder="选择角色" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="admin">管理员（只读）</SelectItem>
            <SelectItem value="user">普通用户</SelectItem>
          </SelectContent>
        </Select>
        <FieldDescription v-if="isProtected">保护账号不可改角色/状态</FieldDescription>
      </Field>
      <Field v-if="!user">
        <FieldLabel for="uf-password">初始密码</FieldLabel>
        <Input
          id="uf-password"
          v-model="password"
          type="password"
          placeholder="至少 8 位"
          autocomplete="new-password"
        />
        <FieldDescription>至少 8 位</FieldDescription>
      </Field>
      <Field v-else-if="!readonly && !isProtected">
        <FieldLabel for="uf-reset">重置密码</FieldLabel>
        <Input
          id="uf-reset"
          v-model="password"
          type="password"
          placeholder="留空则不修改"
          autocomplete="new-password"
        />
        <FieldDescription>重置后用户下次登录需改密</FieldDescription>
      </Field>
      <Field v-if="user">
        <FieldLabel>状态</FieldLabel>
        <Input :model-value="user.status === 'active' ? '启用' : '禁用'" disabled />
        <FieldDescription>
          创建于 {{ fmtTime(user.createdAt) }}
          <template v-if="user.lastLoginAt"> · 最后登录 {{ fmtTime(user.lastLoginAt) }}</template>
        </FieldDescription>
      </Field>
      <Alert v-if="error" variant="destructive">
        <AlertTitle>操作失败</AlertTitle>
        <AlertDescription>{{ error }}</AlertDescription>
      </Alert>
    </FieldGroup>
  </SbModal>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import SbModal from './SbModal.vue'
import { api } from '../../services/api'
import type { UserItem, UserRole } from '../../services/types'
import { useAuthStore } from '../../stores/auth'

const auth = useAuthStore()
const props = defineProps<{
  open: boolean
  user: UserItem | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  saved: []
}>()

const readonly = computed(() => Boolean(props.user) && !auth.canManageUsers)
const isProtected = computed(() => props.user?.username === 'simplebase2026')

const username = ref('')
const displayName = ref('')
const role = ref<UserRole | ''>('')
const password = ref('')
const error = ref('')
const submitting = ref(false)

const canSubmit = computed(() => {
  if (readonly.value) return false
  if (props.user) return true
  return username.value.trim().length > 0 && password.value.length >= 8 && role.value !== ''
})

watch(
  () => props.open,
  (v) => {
    if (!v) return
    error.value = ''
    password.value = ''
    username.value = props.user?.username || ''
    displayName.value = props.user?.displayName || ''
    role.value = (props.user?.role as UserRole) || ''
  }
)

function fmtTime(s?: string) {
  if (!s) return '—'
  try {
    return new Date(s).toLocaleString()
  } catch {
    return s
  }
}

async function submit() {
  if (!canSubmit.value || submitting.value) return
  submitting.value = true
  error.value = ''
  try {
    if (props.user) {
      await api.users.update(props.user.id, {
        displayName: displayName.value,
        role: role.value || undefined,
        password: password.value || undefined
      })
      toast.success('用户已更新')
    } else {
      await api.users.create({
        username: username.value.trim(),
        password: password.value,
        role: (role.value || 'user') as Exclude<UserRole, 'superadminl1'>,
        displayName: displayName.value
      })
      toast.success('用户已创建')
    }
    emit('saved')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '操作失败'
  } finally {
    submitting.value = false
  }
}
</script>
