<template>
  <div>
    <SbEmptyState
      v-if="!project.id"
      :description="t('components.settingsPanel.noProject')"
      :action-text="t('components.settingsPanel.createProject')"
      @action="project.openCreateModal()"
    />
    <SbAsyncRegion
      v-else
      block="lines"
      class="min-h-40"
      :pending="modelsPending"
      :show-skeleton="modelsSkeleton"
      :show-empty="false"
      :show-error="modelsFailed"
      :refreshing="modelsRefreshing"
      :error="modelsErrorText"
      @retry="loadServerDefaults"
    >
      <FieldGroup class="max-w-lg">
        <Field>
          <FieldLabel>{{ t('components.settingsPanel.defaultProvider') }}</FieldLabel>
          <Select
            :model-value="defaults.defaultProvider || undefined"
            @update:model-value="(v: string) => patchDefaults({ defaultProvider: v })"
          >
            <SelectTrigger class="w-full">
              <SelectValue :placeholder="t('components.settingsPanel.providerPlaceholder')" />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem v-for="p in presets" :key="p.id" :value="p.id">{{ p.name }}</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel for="model-api-key">API Key</FieldLabel>
          <div class="flex flex-wrap gap-2">
            <Input
              id="model-api-key"
              :model-value="apiKey"
              type="password"
              autocomplete="off"
              class="min-w-0 flex-1"
              :placeholder="keyPlaceholder"
              @update:model-value="(v) => (apiKey = String(v))"
            />
            <Button :disabled="saving" class="shrink-0" @click="saveConnection">
              <Spinner v-if="saving" data-icon="inline-start" />
              {{ t('common.save') }}
            </Button>
          </div>
          <FieldDescription>{{ t('components.settingsPanel.apiKeyHint') }}</FieldDescription>
        </Field>
        <Field>
          <FieldLabel for="model-base-url">Base URL</FieldLabel>
          <Input
            id="model-base-url"
            :model-value="defaults.baseUrl ?? ''"
            placeholder="https://api.openai.com/v1"
            @update:model-value="(v) => patchDefaults({ baseUrl: String(v) })"
          />
          <FieldDescription>{{ t('components.settingsPanel.baseUrlHint') }}</FieldDescription>
        </Field>
        <Field>
          <FieldLabel>{{ t('components.settingsPanel.defaultModel') }}</FieldLabel>
          <Combobox
            :model-value="defaults.defaultModel"
            @update:model-value="(v: string) => patchDefaults({ defaultModel: v })"
          >
            <ComboboxAnchor>
              <ComboboxInput :placeholder="t('components.settingsPanel.modelPlaceholder')" />
            </ComboboxAnchor>
            <ComboboxList>
              <ComboboxViewport>
                <ComboboxEmpty>{{ t('components.settingsPanel.noMatchModel') }}</ComboboxEmpty>
                <ComboboxGroup>
                  <ComboboxItem v-for="m in modelSuggests" :key="m" :value="m">{{ m }}</ComboboxItem>
                </ComboboxGroup>
              </ComboboxViewport>
            </ComboboxList>
          </Combobox>
        </Field>
        <Field>
          <FieldLabel>{{ t('components.settingsPanel.temperature') }} {{ defaults.temperature ?? 0.7 }}</FieldLabel>
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
          <FieldLabel for="max-tokens">{{ t('components.settingsPanel.maxTokens') }}</FieldLabel>
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
    </SbAsyncRegion>
  </div>
</template>

<script setup lang="ts">
/**
 * 设置 → 模型：项目级默认供应商 / API Key / Base URL / 采样参数。
 *
 * 连接信息（Key、Base URL）与默认参数同属 sys_llm_settings 一行，见 SettingsModal。
 * api_key 只单向写：服务端只回掩码，输入框留空即沿用既有值。
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import SbEmptyState from '../SbEmptyState.vue'
import { useLoadState } from '@/composables/useLoadState'
import { Button } from '@/components/ui/button'
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
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
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
import { api } from '../../services/api'
import { LLM_PROVIDER_PRESETS, getProviderPreset } from '../../constants/llmProviders'
import { useProjectStore } from '../../stores/project'
import { useSettingsStore } from '../../stores/settings'

const project = useProjectStore()
const settings = useSettingsStore()
const { t } = useI18n()

const presets = LLM_PROVIDER_PRESETS

const defaults = computed(() => settings.defaultsFor(project.id))

const modelSuggests = computed(() => {
  const id = defaults.value.defaultProvider
  const preset = id ? getProviderPreset(id) : undefined
  return preset?.suggestedModels || []
})

/** 待写入的新 Key；留空保存即沿用服务端既有值（前端只有掩码） */
const apiKey = ref('')
const saving = ref(false)

const keyPlaceholder = computed(() =>
  defaults.value.hasApiKey
    ? t('components.settingsPanel.savedPlaceholder', { mask: defaults.value.apiKeyMasked || '' })
    : t('components.settingsPanel.apiKeyPlaceholder')
)

const {
  pending: modelsPending,
  showSkeleton: modelsSkeleton,
  showError: modelsFailed,
  refreshing: modelsRefreshing,
  error: modelsErrorText,
  run: runModels
} = useLoadState({ fallback: 'components.settingsPanel.loadModelsFailed' })

async function loadServerDefaults(replace = false) {
  if (!project.id) return
  return runModels(async () => {
    const remote = await api.llmSettings.get(project.id)
    settings.setProjectDefaults(project.id, remote)
    return true
  }, { replace })
}

async function patchDefaults(patch: Record<string, unknown>) {
  const next = { ...defaults.value, ...patch }
  settings.setProjectDefaults(project.id, next)
  try {
    const saved = await api.llmSettings.put(project.id, next)
    settings.setProjectDefaults(project.id, saved)
  } catch (e) {
    toast.error((e as Error)?.message || t('components.settingsPanel.saveModelFailed'))
  }
}

/** 保存 API Key：留空则沿用既有值；成功后清空输入框并刷新掩码展示 */
async function saveConnection() {
  if (!project.id || saving.value) return
  saving.value = true
  try {
    const saved = await api.llmSettings.put(project.id, defaults.value, apiKey.value.trim())
    settings.setProjectDefaults(project.id, saved)
    apiKey.value = ''
    toast.success(t('components.settingsPanel.savedToServer'))
  } catch (e) {
    toast.error((e as Error)?.message || t('components.settingsPanel.saveFailed'))
  } finally {
    saving.value = false
  }
}

defineExpose({ loadServerDefaults, saveConnection })

watch(
  () => project.id,
  () => {
    apiKey.value = ''
    void loadServerDefaults(true)
  }
)

void loadServerDefaults()
</script>
