<template>
  <SbModal
    :open="open"
    :title="title"
    :confirm-loading="submitting"
    :ok-text="user ? t('components.userForm.save') : t('components.userForm.create')"
    :ok-button-props="{ disabled: readonly || !canSubmit }"
    :hide-footer="readonly"
    @ok="submit"
    @update:open="emit('update:open', $event)"
  >
    <FieldGroup>
      <Field>
        <FieldLabel for="uf-username">{{ t('components.userForm.username') }}</FieldLabel>
        <Input
          id="uf-username"
          v-model="username"
          :disabled="Boolean(user)"
          :placeholder="t('components.userForm.usernamePlaceholder')"
          autocomplete="off"
        />
        <FieldDescription>{{ user ? t('components.userForm.usernameImmutable') : t('components.userForm.usernameDesc') }}</FieldDescription>
      </Field>
      <Field>
        <FieldLabel for="uf-display">{{ t('components.userForm.displayName') }}</FieldLabel>
        <Input id="uf-display" v-model="displayName" :disabled="readonly" :placeholder="t('common.optional')" />
      </Field>
      <Field>
        <FieldLabel for="uf-role">{{ t('components.userForm.role') }}</FieldLabel>
        <Select v-model="role" :disabled="readonly || isProtected">
          <SelectTrigger id="uf-role" class="w-full">
            <SelectValue :placeholder="t('components.userForm.rolePlaceholder')" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="admin">{{ t('components.userForm.roleAdmin') }}</SelectItem>
            <SelectItem value="user">{{ t('components.userForm.roleUser') }}</SelectItem>
          </SelectContent>
        </Select>
        <FieldDescription v-if="isProtected">{{ t('components.userForm.protectedHint') }}</FieldDescription>
      </Field>
      <Field v-if="!user">
        <FieldLabel for="uf-password">{{ t('components.userForm.initialPassword') }}</FieldLabel>
        <Input
          id="uf-password"
          v-model="password"
          type="password"
          :placeholder="t('components.userForm.initialPasswordPlaceholder')"
          autocomplete="new-password"
        />
        <FieldDescription>{{ t('components.userForm.initialPasswordDesc') }}</FieldDescription>
      </Field>
      <Field v-else-if="!readonly && !isProtected">
        <FieldLabel for="uf-reset">{{ t('components.userForm.resetPassword') }}</FieldLabel>
        <Input
          id="uf-reset"
          v-model="password"
          type="password"
          :placeholder="t('components.userForm.resetPasswordPlaceholder')"
          autocomplete="new-password"
        />
        <FieldDescription>{{ t('components.userForm.resetPasswordDesc') }}</FieldDescription>
      </Field>
      <Field v-if="user">
        <FieldLabel>{{ t('common.status') }}</FieldLabel>
        <Input :model-value="user.status === 'active' ? t('common.enabledState') : t('common.disabledState')" disabled />
        <FieldDescription>
          {{ t('components.userForm.createdAt', { time: fmtTime(user.createdAt) }) }}
          <template v-if="user.lastLoginAt">{{ t('components.userForm.lastLogin', { time: fmtTime(user.lastLoginAt) }) }}</template>
        </FieldDescription>
      </Field>
      <Alert v-if="error" variant="destructive">
        <AlertTitle>{{ t('components.userForm.failedTitle') }}</AlertTitle>
        <AlertDescription>{{ error }}</AlertDescription>
      </Alert>
    </FieldGroup>
  </SbModal>
</template>
<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import SbModal from './SbModal.vue'
import { api } from '../../services/api'
import type { UserItem, UserRole } from '../../services/types'
import { useAuthStore } from '../../stores/auth'
import { formatTime } from '../../utils/format'

const auth = useAuthStore()
const { t } = useI18n()
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

const title = computed(() => {
  if (!props.user) return t('components.userForm.titleCreate')
  return readonly.value ? t('components.userForm.titleDetail') : t('components.userForm.titleEdit')
})

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
  return formatTime(s, '—')
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
      toast.success(t('components.userForm.updated'))
    } else {
      await api.users.create({
        username: username.value.trim(),
        password: password.value,
        role: (role.value || 'user') as Exclude<UserRole, 'superadminl1'>,
        displayName: displayName.value
      })
      toast.success(t('components.userForm.created'))
    }
    emit('saved')
  } catch (e) {
    error.value = errorMessage(e, t('components.userForm.failed'))
  } finally {
    submitting.value = false
  }
}
</script>
