<template>
  <SbModal
    :open="auth.settingsOpen"
    title="设置"
    description="连接、默认模型与厂商 API Key"
    :max-width="720"
    :hide-footer="true"
    @update:open="onOpen"
  >
    <Tabs :model-value="auth.settingsTab" class="w-full" @update:model-value="onTab">
      <TabsList variant="line" class="w-full justify-start">
        <TabsTrigger value="connection">连接</TabsTrigger>
        <TabsTrigger value="models">模型</TabsTrigger>
        <TabsTrigger value="providers">供应商</TabsTrigger>
      </TabsList>
      <div class="max-h-[min(calc(100vh-14rem),720px)] overflow-y-auto px-1 -mx-1 pt-4">
        <TabsContent value="connection">
          <ConnectionPanel />
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
  if (tab === 'connection' || tab === 'models' || tab === 'providers') {
    auth.settingsTab = tab as SettingsTab
  }
}
</script>
