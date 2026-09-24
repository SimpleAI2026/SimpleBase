<template>
  <div class="flex flex-col gap-4 py-1">
    <SidebarGroup v-for="group in menuGroups" :key="group.key" class="gap-1.5">
      <SidebarGroupLabel
        class="h-6 px-2 text-[11px] font-medium tracking-[0.08em] text-muted-foreground/70 uppercase"
      >
        {{ group.label }}
      </SidebarGroupLabel>
      <SidebarGroupContent>
        <SidebarMenu class="gap-1.5">
          <SidebarMenuItem v-for="item in group.items" :key="item.name">
            <SidebarMenuButton
              as-child
              :is-active="selectedKey === item.name"
              :tooltip="item.title"
              class="h-11 rounded-lg px-2.5 data-[active=true]:bg-accent/80 data-[active=true]:text-accent-foreground data-[active=true]:shadow-sm sm:h-9"
            >
              <router-link
                :to="{ name: item.name }"
                class="relative items-center gap-2.5"
                @click="emit('navigate')"
              >
                <span
                  v-if="selectedKey === item.name"
                  class="absolute top-1/2 -left-0.5 h-5 w-[3px] -translate-y-1/2 rounded-full bg-primary"
                  aria-hidden="true"
                />
                <component :is="item.icon" class="size-4 shrink-0 opacity-80" />
                <span class="truncate">{{ item.title }}</span>
                <span
                  v-if="item.badge"
                  class="ml-auto rounded-full bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium text-primary"
                >
                  {{ item.badge }}
                </span>
              </router-link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  </div>
</template>
<script setup lang="ts">
import { computed, type Component } from 'vue'
import { useRoute } from 'vue-router'
import {
  BotIcon,
  CloudUploadIcon,
  CodeIcon,
  DatabaseIcon,
  FileTextIcon,
  LayoutDashboardIcon,
  TimerIcon,
  UsersIcon
} from '@lucide/vue'
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem
} from '@/components/ui/sidebar'
import router from '../router'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()

const iconMap: Record<string, Component> = {
  dashboard: LayoutDashboardIcon,
  databases: DatabaseIcon,
  s3: CloudUploadIcon,
  gofunctions: CodeIcon,
  'cron-jobs': TimerIcon,
  agents: BotIcon,
  logs: FileTextIcon,
  users: UsersIcon
}

/** 侧栏分组：key 稳定，label 展示用；items 按组内顺序。 */
interface MenuGroup {
  key: string
  label: string
  items: MenuItem[]
}

interface MenuItem {
  name: string
  title: string
  icon: Component
  /** 角标（如 super 专属），可选 */
  badge?: string
}

/** 分类定义（planv3.0 侧栏美化）：工作台 / 数据 / 自动化 / 运维。 */
const GROUP_DEFS: { key: string; label: string; names: string[] }[] = [
  { key: 'workspace', label: '工作台', names: ['dashboard'] },
  { key: 'data', label: '数据', names: ['databases', 's3'] },
  { key: 'automation', label: '自动化', names: ['gofunctions', 'cron-jobs', 'agents'] },
  { key: 'ops', label: '运维', names: ['logs', 'users'] }
]

function isVisible(name: string): boolean {
  const r = router.getRoutes().find((rr) => rr.name === name)
  if (!r || r.meta?.hidden || !iconMap[name]) return false
  const need = r.meta?.requiresRole as string | undefined
  if (need === 'superadminl1' && !auth.isSuper) return false
  return true
}

function toItem(name: string): MenuItem {
  const r = router.getRoutes().find((rr) => rr.name === name)
  const item: MenuItem = {
    name,
    title: ((r?.meta?.title as string) || name) as string,
    icon: iconMap[name]
  }
  if (name === 'users' && auth.isSuper) item.badge = '★'
  return item
}

/** 扁平列表（兼容既有测试与引用）。 */
const menuItems = computed<MenuItem[]>(() => {
  const out: MenuItem[] = []
  for (const g of GROUP_DEFS) {
    for (const n of g.names) {
      if (isVisible(n)) out.push(toItem(n))
    }
  }
  return out
})

/** 按分类分组（仅含可见项；空组不渲染）。 */
const menuGroups = computed<MenuGroup[]>(() =>
  GROUP_DEFS.map((g) => ({
    key: g.key,
    label: g.label,
    items: g.names.filter(isVisible).map(toItem)
  })).filter((g) => g.items.length > 0)
)

const emit = defineEmits<{ (e: 'navigate'): void }>()
const currentRoute = useRoute()
const selectedKey = computed(() => (currentRoute.name as string) || 'dashboard')
</script>
