<template>
  <a-config-provider :theme="antdTheme">
    <router-view />
  </a-config-provider>
</template>
<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch } from 'vue'
import { theme as antdThemeApi } from 'ant-design-vue'
import { antdDarkThemeToken, antdThemeToken } from './styles/tokens'
import { useSettingsStore } from './stores/settings'

const settings = useSettingsStore()
const { darkAlgorithm, defaultAlgorithm } = antdThemeApi

const antdTheme = computed(() => {
  const dark = settings.effectiveTheme === 'dark'
  const base = dark ? antdDarkThemeToken : antdThemeToken
  return {
    ...base,
    algorithm: dark ? darkAlgorithm : defaultAlgorithm
  }
})

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
