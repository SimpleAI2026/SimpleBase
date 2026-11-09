<template>
  <TooltipProvider>
    <router-view />
    <Toaster rich-colors position="top-center" />
  </TooltipProvider>
</template>
<script setup lang="ts">
import { onMounted, onUnmounted, watch } from 'vue'
import { Toaster } from '@/components/ui/sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import { useSettingsStore } from './stores/settings'

const settings = useSettingsStore()

let mql: MediaQueryList | null = null
function onSystemTheme() {
  if (settings.theme === 'system') settings.applyThemeToDom()
}

onMounted(() => {
  settings.applyThemeToDom()
  mql = window.matchMedia('(prefers-color-scheme: dark)')
  mql.addEventListener('change', onSystemTheme)
})
onUnmounted(() => {
  mql?.removeEventListener('change', onSystemTheme)
})
watch(
  () => settings.theme,
  () => settings.applyThemeToDom()
)
</script>
