<template>
  <SbModal
    :open="auth.settingsOpen"
    title="设置"
    description="主题、连接、默认模型与厂商 API Key"
    :max-width="900"
    hide-footer
    @update:open="onOpen"
  >
    <Tabs :model-value="auth.settingsTab" class="w-full" @update:model-value="onTab">
      <TabsList variant="line" class="w-full justify-start">
        <TabsTrigger value="connection">连接</TabsTrigger>
        <TabsTrigger value="appearance">外观</TabsTrigger>
        <TabsTrigger value="models">模型</TabsTrigger>
        <TabsTrigger value="providers">供应商</TabsTrigger>
      </TabsList>
      <div class="max-h-[min(80vh,720px)] overflow-y-auto pt-4">
        <TabsContent value="connection">
          <ConnectionPanel />
        </TabsContent>
        <TabsContent value="appearance">
          <SettingsPanel section="appearance" />
        </TabsContent>
        <TabsContent value="models">
          <SettingsPanel section="models" />
        </TabsContent>
        <TabsContent value="providers">
          <SettingsPanel section="providers" />
        </TabsContent>
      </div>
    </Tabs>
  </SbModal>
</template>

<script setup lang="ts">
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import SbModal from './modal/SbModal.vue'
import ConnectionPanel from './settings/ConnectionPanel.vue'
import SettingsPanel from './settings/SettingsPanel.vue'
import { useAuthStore, type SettingsTab } from '../stores/auth'

const auth = useAuthStore()

function onOpen(v: boolean) {
  if (v) auth.openSettings()
  else auth.closeSettings()
}

function onTab(v: string | number) {
  const tab = String(v)
  if (tab === 'connection' || tab === 'appearance' || tab === 'models' || tab === 'providers') {
    auth.settingsTab = tab as SettingsTab
  }
}
</script>
