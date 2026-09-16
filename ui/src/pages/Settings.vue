<template>
  <PageContainer title="设置" subtitle="主题、默认模型与厂商 API Key（本地存储，后端凭证接口未就绪）">
    <a-alert
      type="info"
      show-icon
      style="margin-bottom: 16px"
      message="关于厂商 Key"
      description="Key 保存在浏览器 localStorage，不会自动注入现有 LLM 网关。当前对话仍使用服务端已配置的供应商；后端 §3.9 凭证接口就绪后将改为服务端托管。"
    />

    <a-card class="sb-card" title="外观">
      <a-form layout="vertical" style="max-width: 420px">
        <a-form-item label="主题">
          <a-radio-group :value="settings.theme" button-style="solid" @update:value="onTheme">
            <a-radio-button value="light">浅色</a-radio-button>
            <a-radio-button value="dark">深色</a-radio-button>
            <a-radio-button value="system">跟随系统</a-radio-button>
          </a-radio-group>
        </a-form-item>
      </a-form>
    </a-card>

    <a-card class="sb-card" title="模型默认值">
      <div class="sb-toolbar" style="margin-bottom: 12px">
        <ProjectPicker />
      </div>
      <a-form layout="vertical" style="max-width: 520px">
        <a-form-item label="默认供应商">
          <a-select
            :value="defaults.defaultProvider"
            allow-clear
            placeholder="选择预置厂商"
            :options="providerOptions"
            @update:value="(v: string) => patchDefaults({ defaultProvider: v })"
          />
        </a-form-item>
        <a-form-item label="默认模型">
          <a-auto-complete
            :value="defaults.defaultModel"
            :options="modelSuggestOptions"
            placeholder="如 gpt-4o-mini"
            allow-clear
            @update:value="(v: string) => patchDefaults({ defaultModel: v })"
          />
        </a-form-item>
        <a-form-item label="Temperature">
          <a-slider
            :min="0"
            :max="2"
            :step="0.1"
            :value="defaults.temperature ?? 0.7"
            @update:value="(v: number) => patchDefaults({ temperature: v })"
          />
        </a-form-item>
        <a-form-item label="Max Tokens">
          <a-input-number
            :min="1"
            :max="128000"
            :value="defaults.maxTokens ?? 1024"
            style="width: 160px"
            @update:value="(v: number) => patchDefaults({ maxTokens: v ?? 1024 })"
          />
        </a-form-item>
      </a-form>
    </a-card>

    <a-card class="sb-card" title="供应商与 API Key">
      <div class="provider-grid">
        <div v-for="p in presets" :key="p.id" class="provider-card">
          <div class="provider-head">
            <div class="provider-avatar">{{ p.name.slice(0, 1) }}</div>
            <div class="provider-meta">
              <div class="provider-name">{{ p.name }}</div>
              <div class="provider-proto">{{ p.protocol }}</div>
            </div>
            <a-tag :color="configured(p.id) ? 'green' : 'default'">
              {{ configured(p.id) ? '已配置' : '未配置' }}
            </a-tag>
          </div>
          <div class="provider-hint" v-if="configured(p.id)">
            Key {{ mask(localCfg(p.id)?.credentials.api_key) }}
          </div>
          <div class="provider-actions">
            <a-button size="small" type="primary" ghost @click="openEditor(p.id)">配置</a-button>
            <a-button
              size="small"
              :disabled="defaults.defaultProvider === p.id"
              @click="setDefault(p.id)"
            >
              {{ defaults.defaultProvider === p.id ? '当前默认' : '设为默认' }}
            </a-button>
          </div>
        </div>
      </div>
    </a-card>

    <a-drawer
      v-model:open="editorOpen"
      :title="editorPreset ? `配置 ${editorPreset.name}` : '配置厂商'"
      width="420"
      destroy-on-close
      @close="editorOpen = false"
    >
      <a-form v-if="editorPreset" layout="vertical">
        <a-form-item
          v-for="f in editorPreset.fields"
          :key="f.key"
          :label="f.label"
          :required="f.required"
        >
          <a-input-password
            v-if="f.secret"
            v-model:value="editorForm[f.key]"
            :placeholder="f.placeholder || (configured(editorPreset.id) ? '已保存，留空则不修改' : '')"
            autocomplete="off"
          />
          <a-input
            v-else
            v-model:value="editorForm[f.key]"
            :placeholder="f.placeholder"
            autocomplete="off"
          />
        </a-form-item>
        <a-form-item label="该厂商默认模型">
          <a-auto-complete
            v-model:value="editorDefaultModel"
            :options="editorModelOptions"
            placeholder="可选"
            allow-clear
          />
        </a-form-item>
        <a-space>
          <a-button type="primary" @click="saveEditor">保存到本地</a-button>
          <a-button danger ghost :disabled="!configured(editorPreset.id)" @click="clearEditor">
            清除 Key
          </a-button>
        </a-space>
      </a-form>
    </a-drawer>
  </PageContainer>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import PageContainer from '../components/PageContainer.vue'
import ProjectPicker from '../components/ProjectPicker.vue'
import { LLM_PROVIDER_PRESETS, getProviderPreset, maskSecret } from '../constants/llmProviders'
import type { LlmProviderFieldKey } from '../constants/llmProviders'
import { useProjectStore } from '../stores/project'
import { useSettingsStore, type ThemeMode } from '../stores/settings'

const project = useProjectStore()
const settings = useSettingsStore()
const presets = LLM_PROVIDER_PRESETS

const defaults = computed(() => settings.defaultsFor(project.id))
const providerOptions = presets.map((p) => ({ label: p.name, value: p.id }))

const modelSuggestOptions = computed(() => {
  const id = defaults.value.defaultProvider
  const preset = id ? getProviderPreset(id) : undefined
  const models = preset?.suggestedModels || []
  return models.map((m) => ({ value: m }))
})

function onTheme(v: ThemeMode) {
  settings.setTheme(v)
}

function patchDefaults(patch: Record<string, unknown>) {
  settings.setProjectDefaults(project.id, patch as never)
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

function setDefault(providerId: string) {
  settings.setDefaultProvider(project.id, providerId)
  message.success('已设为默认供应商')
}

const editorOpen = ref(false)
const editorProviderId = ref('')
const editorPreset = computed(() => getProviderPreset(editorProviderId.value))
const editorForm = reactive<Partial<Record<LlmProviderFieldKey, string>>>({})
const editorDefaultModel = ref('')

const editorModelOptions = computed(() =>
  (editorPreset.value?.suggestedModels || []).map((m) => ({ value: m }))
)

function openEditor(providerId: string) {
  editorProviderId.value = providerId
  const cfg = localCfg(providerId)
  Object.keys(editorForm).forEach((k) => delete editorForm[k as LlmProviderFieldKey])
  const preset = getProviderPreset(providerId)
  preset?.fields.forEach((f) => {
    // secret 不回填明文，避免在输入框完整展示
    if (f.secret) editorForm[f.key] = ''
    else editorForm[f.key] = cfg?.credentials[f.key] || ''
  })
  editorDefaultModel.value = cfg?.defaultModel || ''
  editorOpen.value = true
}

function saveEditor() {
  const preset = editorPreset.value
  if (!preset) return
  for (const f of preset.fields) {
    if (!f.required) continue
    const existing = localCfg(preset.id)?.credentials[f.key]
    const next = (editorForm[f.key] || '').trim()
    if (!next && !(f.secret && existing)) {
      message.warning(`请填写 ${f.label}`)
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
  editorOpen.value = false
  message.success('已保存到本地')
}

function clearEditor() {
  const id = editorProviderId.value
  settings.clearProviderCredentials(project.id, id)
  editorOpen.value = false
  message.success('已清除本地 Key')
}

watch(
  () => project.id,
  () => {
    /* defaults computed 自动切换 */
  }
)
</script>

<style scoped>
.provider-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: var(--sb-space-3);
}
.provider-card {
  border: 1px solid var(--sb-border-soft);
  border-radius: var(--sb-radius-md, 12px);
  padding: 14px;
  background: var(--sb-bg, #fff);
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.provider-head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.provider-avatar {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: var(--sb-primary-light);
  color: var(--sb-primary);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
}
.provider-meta {
  flex: 1;
  min-width: 0;
}
.provider-name {
  font-weight: 600;
  color: var(--sb-text);
}
.provider-proto {
  font-size: var(--sb-fs-sm);
  color: var(--sb-text-secondary);
}
.provider-hint {
  font-size: var(--sb-fs-sm);
  color: var(--sb-text-secondary);
  font-family: var(--sb-font-mono);
}
.provider-actions {
  display: flex;
  gap: 8px;
}
</style>
