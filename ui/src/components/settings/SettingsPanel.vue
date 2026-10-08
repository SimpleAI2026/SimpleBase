<template>
  <div v-if="section === 'models'">
    <SbEmptyState
      v-if="!project.id"
      description="请先在右上角选择或创建一个项目"
      action-text="创建项目"
      @action="project.openCreateModal()"
    />
    <FieldGroup v-else class="max-w-lg">
      <Field>
        <FieldLabel>默认供应商</FieldLabel>
        <Select
          :model-value="defaults.defaultProvider || undefined"
          @update:model-value="(v: string) => patchDefaults({ defaultProvider: v })"
        >
          <SelectTrigger class="w-full">
            <SelectValue placeholder="选择预置厂商" />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem v-for="p in presets" :key="p.id" :value="p.id">{{ p.name }}</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
      </Field>
      <Field>
        <FieldLabel>默认模型</FieldLabel>
        <Combobox
          :model-value="defaults.defaultModel"
          @update:model-value="(v: string) => patchDefaults({ defaultModel: v })"
        >
          <ComboboxAnchor>
            <ComboboxInput placeholder="如 gpt-4o-mini" />
          </ComboboxAnchor>
          <ComboboxList>
            <ComboboxViewport>
              <ComboboxEmpty>无匹配模型</ComboboxEmpty>
              <ComboboxGroup>
                <ComboboxItem v-for="m in modelSuggests" :key="m" :value="m">{{ m }}</ComboboxItem>
              </ComboboxGroup>
            </ComboboxViewport>
          </ComboboxList>
        </Combobox>
      </Field>
      <Field>
        <FieldLabel>Temperature {{ defaults.temperature ?? 0.7 }}</FieldLabel>
        <div class="flex items-center gap-3">
          <span class="w-3 text-xs text-muted-foreground">0</span>
          <Slider
            class="flex-1"
            :model-value="[defaults.temperature ?? 0.7]"
            :min="0"
            :max="2"
            :step="0.1"
            @update:model-value="(v: number[]) => patchDefaults({ temperature: v[0] })"
          />
          <span class="w-3 text-xs text-muted-foreground">2</span>
        </div>
      </Field>
      <Field>
        <FieldLabel for="max-tokens">Max Tokens</FieldLabel>
        <Input
          id="max-tokens"
          type="number"
          class="w-40"
          :model-value="defaults.maxTokens ?? 1024"
          min="1"
          max="128000"
          @update:model-value="(v) => patchDefaults({ maxTokens: Number(v) || 1024 })"
        />
      </Field>
    </FieldGroup>
  </div>

  <div v-else-if="section === 'providers'" class="flex flex-col gap-6">
    <SbEmptyState
      v-if="!project.id"
      description="请先在右上角选择或创建一个项目"
      action-text="创建项目"
      @action="project.openCreateModal()"
    />
    <template v-else>
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <Card v-for="p in presets" :key="p.id" size="sm" class="flex flex-col justify-between">
          <div class="flex flex-1 flex-col">
            <CardHeader class="p-4 pb-2">
              <div class="flex items-center gap-2.5">
                <Avatar class="size-9 rounded-lg">
                  <AvatarFallback class="rounded-lg bg-primary/12 text-primary font-bold">
                    {{ p.name.slice(0, 1) }}
                  </AvatarFallback>
                </Avatar>
                <div class="min-w-0 flex-1">
                  <CardTitle>{{ p.name }}</CardTitle>
                  <CardDescription>{{ p.protocol }}</CardDescription>
                </div>
                <Badge :variant="configured(p.id) ? 'success' : 'secondary'">
                  {{ configured(p.id) ? '已配置' : '未配置' }}
                </Badge>
              </div>
            </CardHeader>
            <CardContent class="flex-1 p-4 pt-1">
              <p v-if="configured(p.id)" class="font-mono text-xs text-muted-foreground truncate">
                Key {{ remoteMask(p.id) }}
              </p>
              <p v-else class="text-xs text-muted-foreground/60 italic">尚未配置 API Key</p>
            </CardContent>
          </div>
          <CardFooter class="flex gap-2 p-4 pt-3">
            <Button size="sm" variant="outline" @click="openEditor(p.id)">配置</Button>
            <Button
              size="sm"
              variant="ghost"
              :disabled="defaults.defaultProvider === p.id"
              @click="setDefault(p.id)"
            >
              {{ defaults.defaultProvider === p.id ? '当前默认' : '设为默认' }}
            </Button>
          </CardFooter>
        </Card>
      </div>

      <Card v-if="editorPreset" class="border-t">
        <CardHeader class="border-b">
          <CardTitle>配置 {{ editorPreset.name }}</CardTitle>
          <CardDescription>Key 保存在服务端系统表（按项目隔离），不再存本地浏览器</CardDescription>
        </CardHeader>
        <CardContent class="pt-5">
          <FieldGroup>
            <Field v-for="f in editorPreset.fields" :key="f.key">
              <FieldLabel :for="`editor-${f.key}`">{{ f.label }}</FieldLabel>
              <Input
                :id="`editor-${f.key}`"
                v-model="editorForm[f.key]"
                :type="f.secret ? 'password' : 'text'"
                :placeholder="f.placeholder || (configured(editorPreset.id) ? '已保存，留空则不修改' : '')"
                autocomplete="off"
              />
            </Field>
            <Field>
              <FieldLabel>该厂商默认模型</FieldLabel>
              <Combobox v-model="editorDefaultModel">
                <ComboboxAnchor>
                  <ComboboxInput placeholder="可选" />
                </ComboboxAnchor>
                <ComboboxList>
                  <ComboboxViewport>
                    <ComboboxEmpty>无匹配</ComboboxEmpty>
                    <ComboboxGroup>
                      <ComboboxItem v-for="m in editorPreset.suggestedModels" :key="m" :value="m">{{ m }}</ComboboxItem>
                    </ComboboxGroup>
                  </ComboboxViewport>
                </ComboboxList>
              </Combobox>
            </Field>
            <div class="flex gap-2">
              <Button :disabled="saving" @click="saveEditor">
                <Spinner v-if="saving" data-icon="inline-start" />
                保存
              </Button>
              <Button variant="destructive" :disabled="!configured(editorPreset.id) || saving" @click="clearEditor">
                清除 Key
              </Button>
              <Button variant="ghost" @click="closeEditor">取消</Button>
            </div>
          </FieldGroup>
        </CardContent>
      </Card>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Combobox,
  ComboboxAnchor,
  ComboboxEmpty,
  ComboboxGroup,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
  ComboboxViewport,
} from '@/components/ui/combobox'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Slider } from '@/components/ui/slider'
import { Spinner } from '@/components/ui/spinner'
import SbEmptyState from '../SbEmptyState.vue'
import { api } from '../../services/api'
import type { LlmProviderCred } from '../../services/types'
import { LLM_PROVIDER_PRESETS, getProviderPreset } from '../../constants/llmProviders'
import type { LlmProviderFieldKey } from '../../constants/llmProviders'
import { useProjectStore } from '../../stores/project'
import { useSettingsStore } from '../../stores/settings'

defineProps<{
  section: 'models' | 'providers'
}>()

const project = useProjectStore()
const settings = useSettingsStore()
const presets = LLM_PROVIDER_PRESETS

const defaults = computed(() => settings.defaultsFor(project.id))

const modelSuggests = computed(() => {
  const id = defaults.value.defaultProvider
  const preset = id ? getProviderPreset(id) : undefined
  return preset?.suggestedModels || []
})

async function loadServerDefaults() {
  if (!project.id) return
  try {
    const remote = await api.llmSettings.get(project.id)
    settings.setProjectDefaults(project.id, remote)
  } catch {
    /* 保留本地缓存默认值 */
  }
}

/* ---------- 厂商凭证（服务端 sys_llm_provider_creds） ---------- */

const remoteCreds = ref<LlmProviderCred[]>([])
const credsLoading = ref(false)
const saving = ref(false)

function remoteCred(providerId: string): LlmProviderCred | undefined {
  return remoteCreds.value.find((c) => c.provider === providerId)
}

async function loadCreds() {
  if (!project.id) return
  credsLoading.value = true
  try {
    remoteCreds.value = await api.llmProviderCreds.list(project.id)
  } catch {
    remoteCreds.value = []
  } finally {
    credsLoading.value = false
  }
}

function configured(providerId: string): boolean {
  const cred = remoteCred(providerId)
  if (cred) return cred.hasApiKey
  const preset = getProviderPreset(providerId)
  if (!preset) return false
  return preset.fields.filter((f) => f.required).length === 0
}

/** 服务端返回的掩码 Key（如 sk-...abcd） */
function remoteMask(providerId: string): string {
  return remoteCred(providerId)?.credentials.api_key || '••••••••'
}

async function patchDefaults(patch: Record<string, unknown>) {
  const next = { ...defaults.value, ...patch }
  settings.setProjectDefaults(project.id, next)
  try {
    await api.llmSettings.put(project.id, next)
  } catch (e) {
    toast.error((e as Error)?.message || '保存默认模型失败')
  }
}

async function setDefault(providerId: string) {
  const cred = remoteCred(providerId)
  const preset = getProviderPreset(providerId)
  const model = cred?.defaultModel || preset?.suggestedModels[0] || undefined
  await patchDefaults({ defaultProvider: providerId, defaultModel: model })
  toast.success('已设为默认供应商')
}

const editorProviderId = ref('')
const editorPreset = computed(() => getProviderPreset(editorProviderId.value))
const editorForm = reactive<Partial<Record<LlmProviderFieldKey, string>>>({})
const editorDefaultModel = ref('')

function openEditor(providerId: string) {
  editorProviderId.value = providerId
  Object.keys(editorForm).forEach((k) => delete editorForm[k as LlmProviderFieldKey])
  const preset = getProviderPreset(providerId)
  const cred = remoteCred(providerId)
  preset?.fields.forEach((f) => {
    if (f.secret) editorForm[f.key] = ''
    else editorForm[f.key] = cred?.credentials[f.key] || ''
  })
  editorDefaultModel.value = cred?.defaultModel || ''
}

function closeEditor() {
  editorProviderId.value = ''
}

/** 保存到服务端系统表；secret 留空由后端沿用既有值 */
async function saveEditor() {
  const preset = editorPreset.value
  if (!preset || !project.id || saving.value) return
  const cred = remoteCred(preset.id)
  for (const f of preset.fields) {
    if (!f.required) continue
    const next = (editorForm[f.key] || '').trim()
    if (!next && !(f.secret && cred?.hasApiKey)) {
      toast.warning(`请填写 ${f.label}`)
      return
    }
  }
  const credentials: Partial<Record<LlmProviderFieldKey, string>> = {}
  for (const f of preset.fields) {
    const v = (editorForm[f.key] || '').trim()
    if (f.secret) {
      if (v) credentials[f.key] = v
    } else {
      credentials[f.key] = v
    }
  }
  saving.value = true
  try {
    await api.llmProviderCreds.put(project.id, preset.id, {
      credentials: credentials as Record<string, string>,
      defaultModel: editorDefaultModel.value || undefined
    })
    await loadCreds()
    closeEditor()
    toast.success('已保存到服务端')
  } catch (e) {
    toast.error((e as Error)?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function clearEditor() {
  const id = editorProviderId.value
  if (!id || !project.id || saving.value) return
  saving.value = true
  try {
    await api.llmProviderCreds.remove(project.id, id)
    await loadCreds()
    closeEditor()
    toast.success('已清除该厂商 Key')
  } catch (e) {
    toast.error((e as Error)?.message || '清除失败')
  } finally {
    saving.value = false
  }
}

watch(
  () => project.id,
  () => {
    closeEditor()
    void loadServerDefaults()
    void loadCreds()
  }
)

onMounted(() => {
  void loadServerDefaults()
  void loadCreds()
})
</script>
