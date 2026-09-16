<template>
  <a-menu
    class="sb-menu"
    :theme="menuTheme"
    mode="inline"
    :selectedKeys="[selectedKey]"
    @click="onClick"
  >
    <a-menu-item v-for="item in menuItems" :key="item.name">
      <component :is="item.icon" />
      <span>{{ item.title }}</span>
    </a-menu-item>
  </a-menu>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useSettingsStore } from '../stores/settings'
import {
  AppstoreOutlined,
  DatabaseOutlined,
  ConsoleSqlOutlined,
  TableOutlined,
  CloudUploadOutlined,
  CodeOutlined,
  RobotOutlined,
  SettingOutlined,
  FileTextOutlined
} from '@ant-design/icons-vue'
import type { Component } from 'vue'
import router from '../router'

/** 菜单项从路由 meta 派生（单一数据源）；hidden 的路由不进菜单（如 FaaS：后端无此模块） */
const iconMap: Record<string, Component> = {
  dashboard: AppstoreOutlined,
  databases: DatabaseOutlined,
  sql: ConsoleSqlOutlined,
  data: TableOutlined,
  s3: CloudUploadOutlined,
  faas: CodeOutlined,
  llm: RobotOutlined,
  settings: SettingOutlined,
  logs: FileTextOutlined
}

interface MenuItem {
  name: string
  title: string
  icon: Component
}

const menuItems = computed<MenuItem[]>(() =>
  router
    .getRoutes()
    .filter((r) => !r.meta?.hidden && r.name && iconMap[r.name as string])
    .sort((a, b) => orderOf(a.name as string) - orderOf(b.name as string))
    .map((r) => ({
      name: r.name as string,
      title: (r.meta?.title as string) || (r.name as string),
      icon: iconMap[r.name as string]
    }))
)

const routeOrder = ['dashboard', 'databases', 'sql', 'data', 's3', 'faas', 'llm', 'settings', 'logs']
function orderOf(name: string) {
  const i = routeOrder.indexOf(name)
  return i === -1 ? 99 : i
}

const settings = useSettingsStore()
const menuTheme = computed(() => (settings.effectiveTheme === 'dark' ? 'dark' : 'light'))

const emit = defineEmits<{ (e: 'navigate'): void }>()
const currentRoute = useRoute()
const routerInstance = useRouter()
const selectedKey = computed(() => (currentRoute.name as string) || 'dashboard')

function onClick(e: { key: string }) {
  routerInstance.push({ name: e.key })
  emit('navigate')
}
</script>
<style scoped>
.sb-menu {
  background: transparent !important;
  border-right: 0 !important;
  padding-top: 8px;
}
.sb-menu :deep(.ant-menu-item) {
  border-radius: var(--sb-radius-sm) !important;
  height: 40px !important;
  line-height: 40px !important;
  color: var(--sb-text-secondary) !important;
  transition: var(--sb-transition);
}
.sb-menu :deep(.ant-menu-item-selected) {
  background: var(--sb-primary-light) !important;
  color: var(--sb-primary) !important;
  font-weight: 600;
}
.sb-menu :deep(.ant-menu-item-selected .anticon) {
  color: var(--sb-primary) !important;
}
.sb-menu :deep(.ant-menu-item:hover:not(.ant-menu-item-selected)) {
  color: var(--sb-primary) !important;
  background: rgba(31, 30, 29, 0.05) !important;
}
</style>
