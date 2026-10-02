<template>
  <div class="flex flex-col gap-4 max-w-lg">
    <Alert v-if="unauthorized" variant="destructive">
      <AlertTitle>API Key 无效或已过期</AlertTitle>
      <AlertDescription>最近一次请求返回 401，请检查 Key 是否正确或签发新 Key。</AlertDescription>
    </Alert>
    <FieldGroup>
      <Field data-disabled>
        <FieldLabel for="current-project">当前项目</FieldLabel>
        <Input id="current-project" :model-value="projectLabel" disabled />
        <FieldDescription>请在右上角切换或创建项目</FieldDescription>
      </Field>
      <Field>
        <FieldLabel for="api-key">API Key</FieldLabel>
        <div class="flex gap-2">
          <Input
            id="api-key"
            v-model="key"
            type="password"
            placeholder="sb_live_..."
            autocomplete="off"
            class="min-w-0 flex-1"
            @keydown.enter="saveKey"
          />
          <Button class="shrink-0" @click="saveAll">保存</Button>
          <Button variant="outline" class="shrink-0" @click="resetKey">恢复默认（DevMode）</Button>
        </div>
        <FieldDescription>
          DevMode 种子 Key：<code class="rounded-sm bg-muted px-1 font-mono text-xs">sb_live_dev_key_12345</code>；
          生产环境（dev_mode=false）无种子 Key，请在下方为当前项目签发。
        </FieldDescription>
      </Field>
    </FieldGroup>

    <!-- 已签发 Key 列表（生产实例无种子 Key 时的入口） -->
    <div class="flex flex-col gap-3">
      <div class="flex items-center justify-between gap-2">
        <h3 class="text-sm font-medium">已签发（{{ keys.length }}）</h3>
        <Button size="sm" :disabled="!projectStore.id || !canIssue" :title="canIssue ? '' : '当前角色只读，无法签发'" @click="issueKey">
          签发新 Key
        </Button>
      </div>
      <p v-if="!keys.length" class="text-xs text-muted-foreground">暂无 Key。签发后明文仅显示一次，请立即保存。</p>
      <ul v-else class="flex flex-col gap-1.5">
        <li v-for="k in keys" :key="k.id" class="flex items-center justify-between gap-2 rounded-lg border border-border px-3 py-2">
          <div class="min-w-0">
            <p class="truncate font-mono text-xs text-foreground">{{ k.id }}</p>
            <p class="mt-0.5 truncate text-xs text-muted-foreground">
              {{ k.permissions.join('、') || '无权限' }}
            </p>
          </div>
          <Button variant="ghost" size="sm" class="shrink-0 text-destructive hover:text-destructive" :disabled="!canIssue" @click="revokeKey(k)">
            吊销
          </Button>
        </li>
      </ul>
    </div>

    <!-- 一次性明文提示 -->
    <Alert v-if="issuedSecret" class="border-primary/40">
      <AlertTitle>新 Key 已签发（仅显示一次）</AlertTitle>
      <AlertDescription class="flex flex-wrap items-center gap-2">
        <code class="min-w-0 break-all rounded-sm bg-muted px-1.5 py-0.5 font-mono text-xs">{{ issuedSecret }}</code>
        <Button variant="outline" size="sm" class="shrink-0" @click="copySecret">复制</Button>
        <Button variant="outline" size="sm" class="shrink-0" @click="useIssued">设为当前使用</Button>
      </AlertDescription>
    </Alert>
  </div>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { useAuthStore } from '../../stores/auth'
import { useProjectStore } from '../../stores/project'
import { getApiKey } from '../../services/http'
import { api } from '../../services/api'
import type { ApiKeyItem } from '../../services/types'

const authStore = useAuthStore()
const projectStore = useProjectStore()

const unauthorized = computed(() => authStore.lastUnauthorizedAt > 0)

const key = ref(authStore.apiKey)

/* ---------- 项目 Key 签发管理 ---------- */

const keys = ref<ApiKeyItem[]>([])
const loadingKeys = ref(false)
const issuedSecret = ref('')

const projectLabel = computed(() => {
  const name = projectStore.displayName
  const id = projectStore.id
  if (!id) return '未选择'
  return name && name !== id ? `${name}（${id}）` : id
})

watch(
  () => authStore.settingsOpen,
  (v) => {
    if (v) {
      key.value = getApiKey()
      loadKeys()
    }
  },
  { immediate: true }
)

watch(
  () => projectStore.id,
  () => {
    issuedSecret.value = ''
    loadKeys()
  }
)

function saveKey() {
  if (key.value.trim()) authStore.updateKey(key.value)
}

function saveAll() {
  saveKey()
  toast.success('设置已保存')
}

function resetKey() {
  key.value = 'sb_live_dev_key_12345'
  authStore.updateKey(key.value)
  toast.success('已恢复 DevMode 默认 Key')
}

/** 登录态 admin 只读；未登录（Key 通道）或 super/user 可签发 */
const canIssue = computed(() => {
  if (!authStore.user) return true
  return authStore.isSuper || authStore.isUser
})

async function loadKeys() {
  if (!projectStore.id) {
    keys.value = []
    return
  }
  loadingKeys.value = true
  try {
    keys.value = await api.apiKeys.list(projectStore.id)
  } catch {
    // 无权限或端点不可用时静默；签发入口仍可用
    keys.value = []
  } finally {
    loadingKeys.value = false
  }
}

async function issueKey() {
  if (!projectStore.id) return
  try {
    const created = await api.apiKeys.create(projectStore.id)
    issuedSecret.value = created.secret
    toast.success('已签发新 Key，明文仅显示一次')
    await loadKeys()
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '签发失败')
  }
}

async function revokeKey(k: ApiKeyItem) {
  if (!projectStore.id) return
  try {
    await api.apiKeys.revoke(projectStore.id, k.id)
    toast.success('已吊销')
    await loadKeys()
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '吊销失败')
  }
}

function copySecret() {
  navigator.clipboard
    .writeText(issuedSecret.value)
    .then(() => toast.success('已复制'))
    .catch(() => toast.error('复制失败'))
}

/** 签发后一键设为当前使用的 Key（手动点击，不自动替换，避免权限收窄导致 403） */
function useIssued() {
  key.value = issuedSecret.value
  authStore.updateKey(issuedSecret.value)
  toast.success('已设为当前使用的 Key')
}
</script>
