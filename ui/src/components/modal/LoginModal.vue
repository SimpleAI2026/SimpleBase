<template>
  <SbModal
    :open="open"
    :title="mode === 'password' ? '修改密码' : '登录 SimpleBase'"
    :description="
      mode === 'password'
        ? '首次登录请设置新密码后再进入控制台'
        : '使用管理员分配的账号登录控制台'
    "
    :hide-footer="true"
    :max-width="420"
    @update:open="noop"
  >
    <!-- 登录表单 -->
    <form v-if="mode === 'login'" class="flex flex-col gap-4" @submit.prevent="submitLogin">
      <FieldGroup>
        <Field :data-invalid="error ? true : undefined">
          <FieldLabel for="login-username">用户名</FieldLabel>
          <Input
            id="login-username"
            v-model="username"
            placeholder="请输入用户名"
            autocomplete="username"
            autofocus
          />
          <FieldDescription>输入登录用户名</FieldDescription>
        </Field>
        <Field :data-invalid="error ? true : undefined">
          <FieldLabel for="login-password">密码</FieldLabel>
          <Input
            id="login-password"
            v-model="password"
            type="password"
            placeholder="请输入密码"
            autocomplete="current-password"
            @keydown.enter="submitLogin"
          />
          <FieldDescription>输入登录密码</FieldDescription>
        </Field>
      </FieldGroup>
      <Alert v-if="error" variant="destructive">
        <AlertTitle>登录失败</AlertTitle>
        <AlertDescription>{{ error }}</AlertDescription>
      </Alert>
      <div class="flex justify-end gap-2">
        <Button type="submit" :disabled="submitting || !canSubmit">
          <Spinner v-if="submitting" data-icon="inline-start" />
          登录
        </Button>
      </div>
    </form>

    <!-- 强制改密 -->
    <form v-else class="flex flex-col gap-4" @submit.prevent="submitPassword">
      <FieldGroup>
        <Field>
          <FieldLabel for="pw-old">当前密码</FieldLabel>
          <Input id="pw-old" v-model="oldPassword" type="password" autocomplete="current-password" />
        </Field>
        <Field :data-invalid="pwError ? true : undefined">
          <FieldLabel for="pw-new">新密码</FieldLabel>
          <Input id="pw-new" v-model="newPassword" type="password" autocomplete="new-password" />
          <FieldDescription>至少 8 位</FieldDescription>
        </Field>
        <Field :data-invalid="pwError || pwMismatch ? true : undefined">
          <FieldLabel for="pw-confirm">确认新密码</FieldLabel>
          <Input id="pw-confirm" v-model="confirmPassword" type="password" autocomplete="new-password" />
          <FieldDescription v-if="pwError || pwMismatch">{{ pwError || '两次输入不一致' }}</FieldDescription>
        </Field>
      </FieldGroup>
      <Alert v-if="error" variant="destructive">
        <AlertTitle>修改失败</AlertTitle>
        <AlertDescription>{{ error }}</AlertDescription>
      </Alert>
      <div class="flex justify-end gap-2">
        <Button variant="outline" type="button" @click="signOut">退出</Button>
        <Button type="submit" :disabled="submitting || !canSubmitPassword">
          <Spinner v-if="submitting" data-icon="inline-start" />
          确认修改
        </Button>
      </div>
    </form>
  </SbModal>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import SbModal from './SbModal.vue'
import { useAuthStore } from '../../stores/auth'

const auth = useAuthStore()

const props = defineProps<{
  open: boolean
  /** force-password：must_change_password 视图 */
  mode?: 'login' | 'password'
}>()

const mode = computed(() => props.mode || (auth.mustChangePassword ? 'password' : 'login'))

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

function noop() {
  // 登录弹窗不可通过遮罩关闭
}

async function submitLogin() {
  if (!canSubmit.value || submitting.value) return
  submitting.value = true
  error.value = ''
  try {
    await auth.login(username.value.trim(), password.value)
  } catch (e) {
    error.value = e instanceof Error ? e.message : '用户名或密码错误'
  } finally {
    submitting.value = false
  }
}

async function submitPassword() {
  if (!canSubmitPassword.value || submitting.value) return
  if (newPassword.value !== confirmPassword.value) {
    pwError.value = '两次输入不一致'
    return
  }
  if (newPassword.value.length < 8) {
    pwError.value = '新密码至少 8 位'
    return
  }
  submitting.value = true
  error.value = ''
  try {
    await auth.changePassword(oldPassword.value, newPassword.value)
    // 改密后服务端吊销会话，重新登录
    auth.markUnauthorized()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '修改失败'
  } finally {
    submitting.value = false
  }
}

function signOut() {
  void auth.logout()
}
</script>
