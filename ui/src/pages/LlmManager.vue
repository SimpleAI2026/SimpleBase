<template>
  <ProjectScope>
  <PageContainer title="LLM 对话" subtitle="通用 AiChat 组件（支持流式）；默认模型来自设置页">
    <a-card class="sb-card" title="会话">
      <div class="sb-toolbar" style="margin-bottom: 12px">
        <a-button :loading="providersLoading" @click="loadProviders">
          <template #icon><ReloadOutlined /></template>
          刷新服务端供应商
        </a-button>
        <a-space :size="6">
          <span class="sb-label">服务端</span>
          <a-tag v-for="p in providers" :key="p" color="orange">{{ p }}</a-tag>
          <span v-if="!providers.length" class="sb-label">未返回</span>
        </a-space>
        <router-link to="/settings">设置主题 / 厂商 Key</router-link>
      </div>

      <AiChat
        :project-id="project.id"
        v-model:model="model"
        v-model:streaming="streaming"
        :model-options="modelOptions"
      />
    </a-card>
  </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { ReloadOutlined } from '@ant-design/icons-vue'
import { api } from '../services/api'
import { useProjectStore } from '../stores/project'
import { useSettingsStore } from '../stores/settings'
import { getProviderPreset } from '../constants/llmProviders'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import AiChat from '../components/ai/AiChat.vue'

const project = useProjectStore()
const settings = useSettingsStore()

const providers = ref<string[]>([])
const providersLoading = ref(false)
const streaming = ref(true)
const model = ref<string | undefined>()

const modelOptions = computed(() => {
  const opts: { label: string; value: string }[] = []
  const defs = settings.defaultsFor(project.id)
  const preset = defs.defaultProvider ? getProviderPreset(defs.defaultProvider) : undefined
  for (const m of preset?.suggestedModels || []) {
    opts.push({ label: m, value: m })
  }
  for (const p of providers.value) {
    opts.push({ label: `${p}（服务端默认）`, value: `${p}:default` })
  }
  return opts
})

function syncModelFromSettings() {
  const defs = settings.defaultsFor(project.id)
  if (defs.defaultModel) model.value = defs.defaultModel
}

watch(
  () => project.id,
  () => {
    syncModelFromSettings()
    void loadProviders()
  }
)

async function loadProviders() {
  providersLoading.value = true
  try {
    providers.value = await api.llm.providers(project.id)
  } catch (e) {
    message.error(e instanceof Error ? e.message : '加载供应商失败')
  } finally {
    providersLoading.value = false
  }
}

onMounted(() => {
  syncModelFromSettings()
  loadProviders()
})
</script>

<style scoped>
.sb-label {
  color: var(--sb-text-secondary);
  font-size: var(--sb-fs-sm);
}
</style>
