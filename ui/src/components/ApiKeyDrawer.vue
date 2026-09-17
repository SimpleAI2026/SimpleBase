<template>
  <Sheet :open="open" @update:open="onOpen">
    <SheetContent side="right" class="sm:max-w-sm">
      <SheetHeader>
        <SheetTitle>连接设置</SheetTitle>
        <SheetDescription>API Key 存储于浏览器 localStorage，刷新后保留</SheetDescription>
      </SheetHeader>
      <div class="flex flex-col gap-4 px-4">
        <Alert v-if="unauthorized" variant="destructive">
          <AlertTitle>API Key 无效或已过期</AlertTitle>
          <AlertDescription>最近一次请求返回 401，请检查 Key 是否正确。</AlertDescription>
        </Alert>
        <FieldGroup>
          <Field data-disabled>
            <FieldLabel for="current-project">当前项目</FieldLabel>
            <Input id="current-project" :model-value="projectLabel" disabled />
            <FieldDescription>请在右上角切换或创建项目</FieldDescription>
          </Field>
          <Field>
            <FieldLabel for="api-key">API Key</FieldLabel>
            <Input
              id="api-key"
              v-model="key"
              type="password"
              placeholder="sb_live_..."
              autocomplete="off"
              @keydown.enter="saveKey"
            />
          </Field>
        </FieldGroup>
        <div class="flex gap-2">
          <Button @click="saveAll">保存</Button>
          <Button variant="outline" @click="resetKey">恢复默认（DevMode）</Button>
        </div>
        <Separator />
        <p class="text-sm leading-relaxed text-muted-foreground">
          DevMode 种子 Key：<code class="rounded-sm bg-muted px-1 font-mono text-xs">sb_live_dev_key_12345</code>
          （权限：Database Read/Write/Admin + LLMInvoke + ProjectAdmin）。生产环境请使用管理端签发的 Key。
        </p>
      </div>
    </SheetContent>
  </Sheet>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { useAuthStore } from '../stores/auth'
import { useProjectStore } from '../stores/project'
import { getApiKey } from '../services/http'

const authStore = useAuthStore()
const projectStore = useProjectStore()

const open = computed(() => authStore.keyDrawerOpen)

function onOpen(v: boolean) {
  if (v) authStore.openDrawer()
  else authStore.closeDrawer()
}

const unauthorized = computed(() => authStore.lastUnauthorizedAt > 0)

const key = ref(authStore.apiKey)
const projectLabel = computed(() => {
  const name = projectStore.displayName
  const id = projectStore.id
  if (!id) return '未选择'
  return name && name !== id ? `${name}（${id}）` : id
})

watch(open, (v) => {
  if (v) key.value = getApiKey()
})

function saveKey() {
  if (key.value.trim()) authStore.updateKey(key.value)
}

function saveAll() {
  saveKey()
  toast.success('设置已保存')
  authStore.closeDrawer()
}

function resetKey() {
  key.value = 'sb_live_dev_key_12345'
  authStore.updateKey(key.value)
  toast.success('已恢复 DevMode 默认 Key')
}
</script>
