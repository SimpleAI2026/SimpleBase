<template>
  <div class="flex flex-col gap-4 max-w-lg">
    <Alert v-if="unauthorized" variant="destructive">
      <AlertTitle>API Key 无效或已过期</AlertTitle>
      <AlertDescription>最近一次请求返回 401，请检查 Key 是否正确，或点击「重置」获取新 Key。</AlertDescription>
    </Alert>
    <FieldGroup>
      <Field>
        <FieldLabel for="current-project">当前项目 ID</FieldLabel>
        <div class="flex gap-2">
          <Input id="current-project" :model-value="projectStore.id || '未选择'" readonly class="min-w-0 flex-1 font-mono" />
          <Button variant="outline" class="shrink-0" :disabled="!projectStore.id" @click="copyText(projectStore.id, '项目 ID')">复制</Button>
        </div>
        <FieldDescription>
          {{ projectStore.displayName && projectStore.displayName !== projectStore.id ? `${projectStore.displayName}；` : '' }}请在右上角切换或创建项目
        </FieldDescription>
      </Field>
      <Field>
        <FieldLabel for="api-key">API Key</FieldLabel>
        <div class="flex flex-wrap gap-2">
          <Input
            id="api-key"
            v-model="key"
            type="text"
            placeholder="sb_live_..."
            autocomplete="off"
            class="min-w-0 flex-1 font-mono"
            @keydown.enter="saveKey"
          />
          <Button variant="outline" class="shrink-0" :disabled="!key" @click="copyText(key, 'API Key')">复制</Button>
          <Button class="shrink-0" @click="saveAll">保存</Button>
          <Button variant="outline" class="shrink-0" @click="restoreDevKey">恢复默认（DevMode）</Button>
          <ConfirmAction
            title="重置 API Key？"
            description="将为当前项目签发新 Key 并设为当前使用，同时吊销该项目已有的 Key，使用旧 Key 的调用方将失效。"
            :disabled="!canReset"
            @confirm="resetKey"
          >
            <Button variant="outline" class="shrink-0" :disabled="!canReset" :title="canReset ? '' : '请先选择项目，且当前角色需可写'">
              重置
            </Button>
          </ConfirmAction>
        </div>
        <FieldDescription>
          DevMode 种子 Key：<code class="rounded-sm bg-muted px-1 font-mono text-xs">sb_live_dev_key_12345</code>；
          生产环境（dev_mode=false）无种子 Key，请点击「重置」为当前项目生成。
        </FieldDescription>
      </Field>
    </FieldGroup>
  </div>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { computed, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import ConfirmAction from '../ConfirmAction.vue'
import { useAuthStore } from '../../stores/auth'
import { useProjectStore } from '../../stores/project'
import { getApiKey } from '../../services/http'
import { api } from '../../services/api'

const DEV_KEY = 'sb_live_dev_key_12345'

const authStore = useAuthStore()
const projectStore = useProjectStore()

const unauthorized = computed(() => authStore.lastUnauthorizedAt > 0)

const key = ref(authStore.apiKey)

watch(
  () => authStore.settingsOpen,
  (v) => {
    if (v) key.value = getApiKey()
  },
  { immediate: true }
)

function saveKey() {
  if (key.value.trim()) authStore.updateKey(key.value)
}

function saveAll() {
  saveKey()
  toast.success('设置已保存')
}

function restoreDevKey() {
  key.value = DEV_KEY
  authStore.updateKey(key.value)
  toast.success('已恢复 DevMode 默认 Key')
}

function copyText(text: string, label: string) {
  return navigator.clipboard
    .writeText(text)
    .then(() => toast.success(`已复制${label}`))
    .catch(() => toast.error('复制失败'))
}

/** 登录态 admin 只读；未登录（Key 通道）或 super/user 可重置 */
const canReset = computed(() => {
  if (!projectStore.id) return false
  if (!authStore.user) return true
  return authStore.isSuper || authStore.isUser
})

/**
 * 重置：为当前项目签发新 Key → 吊销旧 Key（仍用旧 Key 鉴权）→ 切换为新 Key。
 * 吊销失败不阻断切换，仅提示。
 */
async function resetKey() {
  const pid = projectStore.id
  if (!pid || !canReset.value) return
  try {
    let oldKeys: { id: string }[] = []
    try {
      oldKeys = await api.apiKeys.list(pid)
    } catch {
      // 列表不可用时只签发，不吊销
    }
    const created = await api.apiKeys.create(pid)
    const results = await Promise.allSettled(
      oldKeys.filter((k) => k.id !== created.id).map((k) => api.apiKeys.revoke(pid, k.id))
    )
    key.value = created.secret
    authStore.updateKey(created.secret)
    if (results.some((r) => r.status === 'rejected')) {
      toast.warning('已重置为新 Key，但部分旧 Key 吊销失败')
    } else {
      toast.success('已重置：新 Key 已设为当前使用')
    }
  } catch (e) {
    toast.error(errorMessage(e, '重置失败'))
  }
}
</script>
