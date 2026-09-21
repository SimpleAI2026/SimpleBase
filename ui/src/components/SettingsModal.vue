<template>
  <SbModal
    :open="open"
    title="设置"
    description="连接、主题、默认模型与厂商 API Key"
    :max-width="900"
    hide-footer
    body-class="min-w-0 max-h-[min(70vh,640px)] overflow-y-auto"
    @update:open="onOpen"
  >
    <Tabs :model-value="tab" class="gap-3" @update:model-value="onTab">
      <TabsList variant="line" class="h-9 w-full justify-start gap-1 rounded-none bg-transparent">
        <TabsTrigger value="connection">连接</TabsTrigger>
        <TabsTrigger value="appearance">外观</TabsTrigger>
        <TabsTrigger value="models">模型</TabsTrigger>
        <TabsTrigger value="providers">供应商</TabsTrigger>
      </TabsList>
      <TabsContent value="connection">
        <ConnectionPanel />
      </TabsContent>
      <TabsContent value="appearance">
        <AppSettingsPanel section="appearance" />
      </TabsContent>
      <TabsContent value="models">
        <AppSettingsPanel section="models" />
      </TabsContent>
      <TabsContent value="providers">
        <AppSettingsPanel section="providers" />
      </TabsContent>
    </Tabs>
  </SbModal>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import SbModal from './modal/SbModal.vue'
import ConnectionPanel from './settings/ConnectionPanel.vue'
import AppSettingsPanel from './settings/AppSettingsPanel.vue'
import { useAuthStore, type SettingsTab } from '../stores/auth'

const authStore = useAuthStore()
const open = computed(() => authStore.settingsOpen)
const tab = computed(() => authStore.settingsTab)

function onOpen(v: boolean) {
  if (v) authStore.openSettings()
  else authStore.closeSettings()
}

function onTab(v: string | number) {
  const next = String(v)
  if (next === 'connection' || next === 'appearance' || next === 'models' || next === 'providers') {
    authStore.settingsTab = next as SettingsTab
  }
}
</script>
