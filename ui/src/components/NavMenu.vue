<template>
  <SidebarMenu>
    <SidebarMenuItem v-for="item in menuItems" :key="item.name">
      <SidebarMenuButton
        as-child
        :is-active="selectedKey === item.name"
        :tooltip="item.title"
      >
        <router-link :to="{ name: item.name }" @click="emit('navigate')">
          <component :is="item.icon" />
          <span>{{ item.title }}</span>
        </router-link>
      </SidebarMenuButton>
    </SidebarMenuItem>
  </SidebarMenu>
</template>
<script setup lang="ts">
import { computed, type Component } from 'vue'
import { useRoute } from 'vue-router'
import {
  BotIcon,
  CloudUploadIcon,
  DatabaseIcon,
  FileTextIcon,
  LayoutDashboardIcon,
  SettingsIcon,
} from '@lucide/vue'
import { SidebarMenu, SidebarMenuButton, SidebarMenuItem } from '@/components/ui/sidebar'
import router from '../router'

const iconMap: Record<string, Component> = {
  dashboard: LayoutDashboardIcon,
  databases: DatabaseIcon,
  s3: CloudUploadIcon,
  agents: BotIcon,
  settings: SettingsIcon,
  logs: FileTextIcon,
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
      icon: iconMap[r.name as string],
    }))
)

const routeOrder = ['dashboard', 'databases', 's3', 'agents', 'logs', 'settings']
function orderOf(name: string) {
  const i = routeOrder.indexOf(name)
  return i === -1 ? 99 : i
}

const emit = defineEmits<{ (e: 'navigate'): void }>()
const currentRoute = useRoute()
const selectedKey = computed(() => (currentRoute.name as string) || 'dashboard')
</script>
