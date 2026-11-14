<template>
  <ProjectScope>
    <PageContainer :subtitle="t('pages.models.subtitle')">
      <Tabs :model-value="activeTab" @update:model-value="onTabChange">
        <TabsList>
          <TabsTrigger value="models">{{ t('pages.models.tabModels') }}</TabsTrigger>
          <TabsTrigger value="traces">{{ t('pages.models.tabTraces') }}</TabsTrigger>
        </TabsList>
        <TabsContent value="models">
          <ModelDefaultsPanel />
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
import ModelTraceList from '../components/models/ModelTraceList.vue'
import TraceDetailModal from '../components/models/TraceDetailModal.vue'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

/**
 * 模型设置页（planv6.5 二期 §11.1）：
 * 「模型」与「Trace」合并为单一入口，tab 切换；
 * ?tab=traces 深链直达（/console/traces 也重定向到此）。
 * 模型页签原样复用 ModelDefaultsPanel；Trace 页签为分组列表 + 详情 Modal 弹窗。
 */
const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const activeTab = ref<'models' | 'traces'>(route.query.tab === 'traces' ? 'traces' : 'models')
const detailTraceId = ref<string | undefined>()

watch(
  () => route.query.tab,
  (tab) => {
    activeTab.value = tab === 'traces' ? 'traces' : 'models'
  }
)

function onTabChange(tab: string) {
  activeTab.value = tab === 'traces' ? 'traces' : 'models'
  // tab 状态进 URL：可刷新保位、可分享深链
  void router.replace({
    query: { ...route.query, tab: activeTab.value === 'traces' ? 'traces' : undefined }
  })
}

function openDetail(traceId: string) {
  detailTraceId.value = traceId
}
</script>
