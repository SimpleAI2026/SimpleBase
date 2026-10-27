<template>
  <SbModal
    :open="open"
    :title="mode === 'password' ? t('components.login.titleChange') : t('components.login.titleLogin')"
    :description="
      mode === 'password'
        ? t('components.login.descChange')
        : t('components.login.descLogin')
    "
    :hide-footer="true"
    :max-width="420"
    @update:open="onOpenChange"
  >
    <!-- 登录表单 -->
    <form v-if="mode === 'login'" class="flex flex-col gap-4" @submit.prevent="submitLogin">
      <Alert v-if="auth.loginReason" variant="destructive">
        <AlertTitle>{{ t('components.login.reloginTitle') }}</AlertTitle>
        <AlertDescription>{{ loginReasonText }}</AlertDescription>
      </Alert>
      <FieldGroup>
        <Field :data-invalid="error ? true : undefined">
          <FieldLabel for="login-username">{{ t('components.login.username') }}</FieldLabel>
          <Input
            id="login-username"
            v-model="username"
            :placeholder="t('components.login.usernamePlaceholder')"
            autocomplete="username"
            autofocus
          />
          <FieldDescription>{{ t('components.login.usernameDesc') }}</FieldDescription>
        </Field>
        <Field :data-invalid="error ? true : undefined">
          <FieldLabel for="login-password">{{ t('components.login.password') }}</FieldLabel>
          <Input
            id="login-password"
            v-model="password"
            type="password"
            :placeholder="t('components.login.passwordPlaceholder')"
            autocomplete="current-password"
            @keydown.enter="submitLogin"
          />
          <FieldDescription>{{ t('components.login.passwordDesc') }}</FieldDescription>
        </Field>
      </FieldGroup>
      <Alert v-if="error" variant="destructive">
        <AlertTitle>{{ t('components.login.loginFailedTitle') }}</AlertTitle>
        <AlertDescription>{{ error }}</AlertDescription>
      </Alert>
      <div class="flex justify-end gap-2">
        <Button type="submit" :disabled="submitting || !canSubmit">
          <Spinner v-if="submitting" data-icon="inline-start" />
          {{ t('components.login.login') }}
        </Button>
      </div>
    </form>

    <!-- 强制改密 -->
    <form v-else class="flex flex-col gap-4" @submit.prevent="submitPassword">
      <FieldGroup>
        <Field>
          <FieldLabel for="pw-old">{{ t('components.login.oldPassword') }}</FieldLabel>
          <Input id="pw-old" v-model="oldPassword" type="password" autocomplete="current-password" />
        </Field>
        <Field :data-invalid="pwError ? true : undefined">
          <FieldLabel for="pw-new">{{ t('components.login.newPassword') }}</FieldLabel>
          <Input id="pw-new" v-model="newPassword" type="password" autocomplete="new-password" />
          <FieldDescription>{{ t('components.login.newPasswordDesc') }}</FieldDescription>
        </Field>
        <Field :data-invalid="pwError || pwMismatch ? true : undefined">
          <FieldLabel for="pw-confirm">{{ t('components.login.confirmPassword') }}</FieldLabel>
          <Input id="pw-confirm" v-model="confirmPassword" type="password" autocomplete="new-password" />
          <FieldDescription v-if="pwError || pwMismatch">{{ pwError || t('components.login.mismatch') }}</FieldDescription>
        </Field>
      </FieldGroup>
      <Alert v-if="error" variant="destructive">
        <AlertTitle>{{ t('components.login.changeFailedTitle') }}</AlertTitle>
        <AlertDescription>{{ error }}</AlertDescription>
      </Alert>
      <div class="flex justify-end gap-2">
        <Button variant="outline" type="button" @click="signOut">{{ t('components.login.signOut') }}</Button>
        <Button type="submit" :disabled="submitting || !canSubmitPassword">
          <Spinner v-if="submitting" data-icon="inline-start" />
          {{ t('components.login.submitChange') }}
        </Button>
      </div>
    </form>
  </SbModal>
</template>
<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import SbModal from './SbModal.vue'
import { useAuthStore } from '../../stores/auth'

const auth = useAuthStore()
const { t, te } = useI18n()

const props = defineProps<{
  open: boolean
  /** force-password：must_change_password 视图 */
  mode?: 'login' | 'password'
}>()

const mode = computed(() => props.mode || (auth.mustChangePassword ? 'password' : 'login'))

/**
 * store 里存的是 i18n key（如 `error.codes.unauthorized`），
 * 渲染时再翻译，这样切换语言后提示也会跟着变。
 */
const loginReasonText = computed(() => (te(auth.loginReason) ? t(auth.loginReason) : auth.loginReason))

const username = ref('')
const password = ref('')
const oldPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const error = ref('')
const pwError = ref('')
const submitting = ref(false)

const canSubmit = computed(() => username.value.trim().length > 0 && password.value.length > 0)
const canSubmitPassword = computed(() => oldPassword.value.length > 0 && newPassword.value.length >= 8)

const pwMismatch = computed(() => {
  if (!confirmPassword.value) return false
  return confirmPassword.value !== newPassword.value
})

watch(
  () => props.open,
  (v) => {
    if (v) {
      error.value = ''
      pwError.value = ''
      password.value = ''
    }
  }
)

function onOpenChange(open: boolean) {
  if (open || mode.value !== 'password' || auth.user?.mustChangePassword) return
  auth.mustChangePassword = false
  auth.closeLogin()
  error.value = ''
  pwError.value = ''
  oldPassword.value = ''
  newPassword.value = ''
  confirmPassword.value = ''
}

async function submitLogin() {
  if (!canSubmit.value || submitting.value) return
  submitting.value = true
  error.value = ''
  try {
    await auth.login(username.value.trim(), password.value)
    window.location.reload()
  } catch (e) {
    error.value = errorMessage(e, t('components.login.badCredentials'))
  } finally {
    submitting.value = false
  }
}

async function submitPassword() {
  if (!canSubmitPassword.value || submitting.value) return
  if (newPassword.value !== confirmPassword.value) {
    pwError.value = t('components.login.mismatch')
    return
  }
  if (newPassword.value.length < 8) {
    pwError.value = t('components.login.newPasswordTooShort')
    return
  }
  submitting.value = true
  error.value = ''
  try {
    await auth.changePassword(oldPassword.value, newPassword.value)
    // 改密后服务端吊销会话，重新登录
    auth.markUnauthorized()
  } catch (e) {
    error.value = errorMessage(e, t('components.login.changeFailed'))
  } finally {
    submitting.value = false
  }
}

function signOut() {
  void auth.logout()
}
</script>
