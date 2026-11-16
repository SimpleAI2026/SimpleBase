<template>
  <ProjectScope>
    <PageContainer :subtitle="t('pages.models.subtitle')">
      <Tabs :model-value="activeTab" @update:model-value="onTabChange">
        <TabsList class="h-9 w-full max-w-xl">
          <TabsTrigger value="models" class="min-w-24 px-4">{{ t('pages.models.tabModels') }}</TabsTrigger>
          <TabsTrigger value="mcp" class="min-w-24 px-4">{{ t('pages.models.tabMcp') }}</TabsTrigger>
          <TabsTrigger value="approvals" class="min-w-24 px-4">{{ t('pages.models.tabApprovals') }}</TabsTrigger>
          <TabsTrigger value="traces" class="min-w-24 px-4">{{ t('pages.models.tabTraces') }}</TabsTrigger>
        </TabsList>
        <TabsContent value="models">
          <ModelDefaultsPanel />
        </TabsContent>
        <TabsContent value="mcp">
          <McpServersPanel />
        </TabsContent>
        <TabsContent value="approvals">
          <ApprovalsPanel />
        </TabsContent>
        <TabsContent value="traces">
          <ModelTraceList @open-detail="openDetail" />
        </TabsContent>
      </Tabs>
      <TraceDetailModal :trace-id="detailTraceId" @close="detailTraceId = undefined" />
    </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import ProjectScope from '../components/ProjectScope.vue'
import PageContainer from '../components/PageContainer.vue'
import ModelDefaultsPanel from '../components/settings/ModelDefaultsPanel.vue'
import McpServersPanel from '../components/settings/McpServersPanel.vue'
import ApprovalsPanel from '../components/settings/ApprovalsPanel.vue'
import ModelTraceList from '../components/models/ModelTraceList.vue'
import TraceDetailModal from '../components/models/TraceDetailModal.vue'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

/**
 * 模型设置页（planv6.5 二期 §11.1；planv6.6 合并审批队列）：
 * 「模型 / MCP / 审批队列 / Trace」四个页签统一到一个入口，
 * 侧栏不再单独挂审批队列项。?tab=mcp|approvals|traces 深链直达
 * （/console/approvals、/console/traces 也重定向到此）。
 */
const route = useRoute()
const router = useRouter()
const { t } = useI18n()

type SettingsTab = 'models' | 'mcp' | 'approvals' | 'traces'
const tabFromQuery = (raw: unknown): SettingsTab => {
  if (raw === 'mcp') return 'mcp'
  if (raw === 'approvals') return 'approvals'
  if (raw === 'traces') return 'traces'
  return 'models'
}
const activeTab = ref<SettingsTab>(tabFromQuery(route.query.tab))
const detailTraceId = ref<string | undefined>()

watch(
  () => route.query.tab,
  (tab) => {
    activeTab.value = tabFromQuery(tab)
  }
)

function onTabChange(tab: string) {
  activeTab.value = tabFromQuery(tab)
  // tab 状态进 URL：可刷新保位、可分享深链
  void router.replace({
    query: { ...route.query, tab: activeTab.value === 'models' ? undefined : activeTab.value }
  })
}

function openDetail(traceId: string) {
  detailTraceId.value = traceId
}
</script>
