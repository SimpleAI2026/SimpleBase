<template>
  <SbModal
    :open="auth.settingsOpen"
    :title="t('components.settingsModal.title')"
    :description="t('components.settingsModal.description')"
    :max-width="720"
    :hide-footer="true"
    @update:open="onOpen"
  >
    <SbTabsPanel
      :items="tabItems"
      :model-value="auth.settingsTab"
      variant="line"
      content-class="max-h-[min(calc(100vh-14rem),720px)] overflow-y-auto px-1 -mx-1 pt-4"
      @update:model-value="onTab"
    >
      <template #connection><ConnectionPanel /></template>
      <template #models><ModelDefaultsPanel /></template>
      <template #storage><StoragePanel /></template>
      <template #backups><BackupPanel /></template>
    </SbTabsPanel>
  </SbModal>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import SbModal from './modal/SbModal.vue'
import SbTabsPanel, { type SbTabsPanelItem } from './SbTabsPanel.vue'
import ConnectionPanel from './settings/ConnectionPanel.vue'
import ModelDefaultsPanel from './settings/ModelDefaultsPanel.vue'
import StoragePanel from './settings/StoragePanel.vue'
import BackupPanel from './settings/BackupPanel.vue'
import { useAuthStore, type SettingsTab } from '../stores/auth'

const auth = useAuthStore()
const { t } = useI18n()
const TAB_KEYS: { value: SettingsTab; labelKey: string }[] = [
  { value: 'connection', labelKey: 'components.settingsModal.tabConnection' },
  { value: 'models', labelKey: 'components.settingsModal.tabModels' },
  { value: 'storage', labelKey: 'components.settingsModal.tabStorage' },
  { value: 'backups', labelKey: 'components.settingsModal.tabBackups' }
]
const tabItems = computed<SbTabsPanelItem[]>(() =>
  TAB_KEYS.map((item) => ({ value: item.value, label: t(item.labelKey) }))
)
function onOpen(v: boolean) {
  if (v) auth.openSettings()
  else auth.closeSettings()
}
function onTab(v: string | number) {
  const tab = String(v)
  const hit = TAB_KEYS.find((item) => item.value === tab)
  if (hit) auth.settingsTab = hit.value
}
</script>
