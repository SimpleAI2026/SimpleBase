<template>
  <div v-if="section === 'appearance'">
    <FieldGroup class="max-w-md">
      <Field orientation="horizontal">
        <FieldTitle id="theme-label">主题</FieldTitle>
        <ToggleGroup
          :model-value="settings.theme"
          type="single"
          variant="outline"
          spacing="0"
          aria-labelledby="theme-label"
          @update:model-value="onTheme"
        >
          <ToggleGroupItem value="light">浅色</ToggleGroupItem>
          <ToggleGroupItem value="dark">深色</ToggleGroupItem>
          <ToggleGroupItem value="system">跟随系统</ToggleGroupItem>
        </ToggleGroup>
      </Field>
      <p class="text-sm text-muted-foreground">跟随系统、浅色与深色的选择保存在本机。</p>
    </FieldGroup>
  </div>

  <div v-else-if="section === 'models'">
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
                Key {{ mask(localCfg(p.id)?.credentials.api_key) }}
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
          <CardDescription>Key 仅保存在本机浏览器</CardDescription>
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
              <Button @click="saveEditor">保存到本地</Button>
              <Button variant="destructive" :disabled="!configured(editorPreset.id)" @click="clearEditor">
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
import { Field, FieldGroup, FieldLabel, FieldTitle } from '@/components/ui/field'
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
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import SbEmptyState from '../SbEmptyState.vue'
import { api } from '../../services/api'
import { LLM_PROVIDER_PRESETS, getProviderPreset, maskSecret } from '../../constants/llmProviders'
import type { LlmProviderFieldKey } from '../../constants/llmProviders'
import { useProjectStore } from '../../stores/project'
import { useSettingsStore, type ThemeMode } from '../../stores/settings'

defineProps<{
  section: 'appearance' | 'models' | 'providers'
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

function onTheme(v: string | string[] | undefined) {
  const mode = (Array.isArray(v) ? v[0] : v) as ThemeMode | undefined
  if (mode === 'light' || mode === 'dark' || mode === 'system') settings.setTheme(mode)
}

async function loadServerDefaults() {
  if (!project.id) return
  try {
    const remote = await api.llmSettings.get(project.id)
    settings.setProjectDefaults(project.id, remote)
  } catch {
    /* 保留本地缓存默认值 */
  }
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

function localCfg(providerId: string) {
  return settings.configsFor(project.id)[providerId]
}

function configured(providerId: string) {
  return settings.isProviderConfigured(project.id, providerId)
}

function mask(v?: string) {
  return maskSecret(v)
}

async function setDefault(providerId: string) {
  settings.setDefaultProvider(project.id, providerId)
  try {
    await api.llmSettings.put(project.id, settings.defaultsFor(project.id))
    toast.success('已设为默认供应商')
  } catch (e) {
    toast.error((e as Error)?.message || '保存默认供应商失败')
  }
}

const editorProviderId = ref('')
const editorPreset = computed(() => getProviderPreset(editorProviderId.value))
const editorForm = reactive<Partial<Record<LlmProviderFieldKey, string>>>({})
const editorDefaultModel = ref('')

function openEditor(providerId: string) {
  editorProviderId.value = providerId
  const cfg = localCfg(providerId)
  Object.keys(editorForm).forEach((k) => delete editorForm[k as LlmProviderFieldKey])
  const preset = getProviderPreset(providerId)
  preset?.fields.forEach((f) => {
    if (f.secret) editorForm[f.key] = ''
    else editorForm[f.key] = cfg?.credentials[f.key] || ''
  })
  editorDefaultModel.value = cfg?.defaultModel || ''
}

function closeEditor() {
  editorProviderId.value = ''
}

function saveEditor() {
  const preset = editorPreset.value
  if (!preset) return
  for (const f of preset.fields) {
    if (!f.required) continue
    const existing = localCfg(preset.id)?.credentials[f.key]
    const next = (editorForm[f.key] || '').trim()
    if (!next && !(f.secret && existing)) {
      toast.warning(`请填写 ${f.label}`)
      return
    }
  }
  const credentials: Partial<Record<LlmProviderFieldKey, string>> = {
    ...(localCfg(preset.id)?.credentials || {})
  }
  for (const f of preset.fields) {
    const v = (editorForm[f.key] || '').trim()
    if (f.secret) {
      if (v) credentials[f.key] = v
    } else if (v) {
      credentials[f.key] = v
    } else {
      delete credentials[f.key]
    }
  }
  settings.upsertProviderConfig(project.id, preset.id, {
    enabled: true,
    defaultModel: editorDefaultModel.value || undefined,
    credentials
  })
  closeEditor()
  toast.success('已保存到本地')
}

function clearEditor() {
  const id = editorProviderId.value
  settings.clearProviderCredentials(project.id, id)
  closeEditor()
  toast.success('已清除本地 Key')
}

watch(
  () => project.id,
  () => {
    closeEditor()
    void loadServerDefaults()
  }
)

onMounted(() => {
  void loadServerDefaults()
})
</script>
