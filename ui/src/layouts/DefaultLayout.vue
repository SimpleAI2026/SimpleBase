<template>
  <SidebarProvider>
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <div class="flex h-12 items-center gap-2.5 overflow-hidden px-2 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0">
          <div class="flex size-8 shrink-0 items-center justify-center rounded-md bg-primary font-serif text-base font-semibold text-primary-foreground">
            S
          </div>
          <span class="truncate font-serif text-base font-bold tracking-tight group-data-[collapsible=icon]:hidden">
            SimpleBase
          </span>
          <SidebarTrigger class="ml-auto shrink-0 group-data-[collapsible=icon]:hidden" />
        </div>
      </SidebarHeader>
      <SidebarContent class="gap-0 pt-2">
        <NavMenu />
        <div class="mx-3 mt-3 flex flex-col gap-1 border-t border-sidebar-border pt-3 sm:hidden">
          <router-link to="/docs" target="_blank" rel="noopener noreferrer" class="inline-flex min-h-11 items-center gap-2 rounded-lg px-2.5 text-sm text-sidebar-foreground hover:bg-sidebar-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring">
            <BookOpenIcon aria-hidden="true" class="size-4" />{{ t('layout.docs') }}
          </router-link>
          <button type="button" class="inline-flex min-h-11 items-center gap-2 rounded-lg px-2.5 text-sm text-sidebar-foreground hover:bg-sidebar-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring" @click="toggleTheme">
            <SunIcon v-if="settingsStore.effectiveTheme === 'light'" aria-hidden="true" class="size-4" />
            <MoonIcon v-else aria-hidden="true" class="size-4" />{{ t('layout.toggleTheme') }}
          </button>
          <button type="button" class="inline-flex min-h-11 items-center gap-2 rounded-lg px-2.5 text-sm text-sidebar-foreground hover:bg-sidebar-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring" @click="authStore.openSettings()">
            <SettingsIcon aria-hidden="true" class="size-4" />{{ t('layout.settings') }}
            <span v-if="authStore.lastUnauthorizedAt > 0" class="size-1.5 rounded-full bg-destructive" :aria-label="t('layout.apiKeyInvalid')" />
          </button>
          <button type="button" class="inline-flex min-h-11 items-center gap-2 rounded-lg px-2.5 text-sm text-sidebar-foreground hover:bg-sidebar-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring" @click="reload">
            <RefreshCwIcon aria-hidden="true" class="size-4" />{{ t('layout.refreshPage') }}
          </button>
          <div class="flex items-center justify-between gap-2 px-2.5 py-1.5">
            <span class="text-sm text-sidebar-foreground">{{ t('layout.language') }}</span>
            <LocaleSelect />
          </div>
        </div>
      </SidebarContent>
      <!-- 折叠后顶部的收起按钮隐藏，这里在底部给一个展开入口；仅折叠态显示 -->
      <div class="hidden justify-center border-t border-sidebar-border p-2 group-data-[collapsible=icon]:flex">
        <SidebarTrigger />
      </div>
      <SidebarRail />
    </Sidebar>

    <SidebarInset>
      <header
        class="sticky top-0 z-20 flex min-h-[var(--header-height)] shrink-0 items-center gap-2 border-b border-border/70 bg-background/80 px-3 py-1 backdrop-blur-xl sm:gap-4 sm:px-5 sm:py-0 lg:px-6"
      >
        <div class="flex min-w-0 flex-1 items-center gap-2">
          <SidebarTrigger class="shrink-0 md:hidden" :aria-label="t('layout.openNavigation')" />
          <Breadcrumb class="hidden min-w-0 md:block">
            <BreadcrumbList class="gap-1.5">
              <BreadcrumbItem>
                <BreadcrumbPage class="truncate font-medium text-foreground">
                  {{ currentTitle }}
                </BreadcrumbPage>
              </BreadcrumbItem>
            </BreadcrumbList>
          </Breadcrumb>
        </div>

        <div class="flex min-w-0 items-center gap-1 sm:gap-2">
          <Tooltip>
            <TooltipTrigger as-child>
              <Button variant="outline" size="sm" class="hidden h-8 gap-1.5 rounded-lg px-2.5 sm:inline-flex sm:px-3" as-child>
                <router-link to="/docs" target="_blank" rel="noopener noreferrer" class="inline-flex items-center gap-1.5">
                  <BookOpenIcon data-icon="inline-start" class="size-3.5" />
                  <span class="hidden text-xs md:inline">{{ t('layout.docs') }}</span>
                </router-link>
              </Button>
            </TooltipTrigger>
            <TooltipContent>{{ t('layout.docs') }}</TooltipContent>
          </Tooltip>

          <div class="mx-0.5 hidden h-5 w-px bg-border/80 sm:block" />

          <GlobalProjectSwitcher v-if="authStore.isAuthenticated" />

          <div class="mx-0.5 hidden h-5 w-px bg-border/80 sm:block" />

          <div class="hidden items-center gap-0.5 rounded-lg border border-border/60 bg-muted/30 p-0.5 sm:flex">
            <Tooltip>
              <TooltipTrigger as-child>
                <Button variant="ghost" size="icon" class="size-7 rounded-md" :aria-label="themeToggleLabel" @click="toggleTheme">
                  <SunIcon v-if="settingsStore.effectiveTheme === 'light'" class="size-4" />
                  <MoonIcon v-else class="size-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{{ themeToggleLabel }}</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger as-child>
                <Button variant="ghost" size="icon" class="size-7 rounded-md" :aria-label="settingsLabel" @click="authStore.openSettings()">
                  <span class="relative inline-flex">
                    <SettingsIcon class="size-4" />
                    <span
                      v-if="authStore.lastUnauthorizedAt > 0"
                      class="absolute -top-0.5 -right-0.5 size-1.5 rounded-full bg-destructive"
                    />
                  </span>
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                {{ settingsLabel }}
              </TooltipContent>
            </Tooltip>
            <LocaleSelect class="border-0 bg-transparent shadow-none hover:bg-muted/60 focus-visible:ring-0 dark:bg-transparent" />
            <Tooltip>
              <TooltipTrigger as-child>
                <Button variant="ghost" size="icon" class="size-7 rounded-md" :aria-label="t('layout.refreshPage')" @click="reload">
                  <RefreshCwIcon class="size-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{{ t('layout.refreshPage') }}</TooltipContent>
            </Tooltip>
          </div>

          <UserMenu v-if="authStore.isAuthenticated" />
        </div>
      </header>

      <!-- 内边距走 --page-pad：按视口算高度的页面（云助手）也引用它，避免两处漂移 -->
      <div class="w-full flex-1" :style="{ padding: 'var(--page-pad)' }">
        <router-view v-slot="{ Component }">
          <transition name="sb-fade" mode="out-in">
            <component :is="Component" v-if="authStore.isAuthenticated" />
          </transition>
        </router-view>
      </div>
    </SidebarInset>

    <SettingsModal />
    <LoginModal :open="authStore.loginOpen || authStore.mustChangePassword" />
  </SidebarProvider>
</template>
<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { BookOpenIcon, MoonIcon, RefreshCwIcon, SettingsIcon, SunIcon } from '@lucide/vue'
import { Breadcrumb, BreadcrumbItem, BreadcrumbList, BreadcrumbPage } from '@/components/ui/breadcrumb'
import { Button } from '@/components/ui/button'
import {
  Sidebar,
  SidebarContent,
  SidebarHeader,
  SidebarInset,
  SidebarProvider,
  SidebarRail,
  SidebarTrigger,
} from '@/components/ui/sidebar'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { useAuthStore, type SettingsTab } from '../stores/auth'
import { useSettingsStore } from '../stores/settings'
import NavMenu from '../components/NavMenu.vue'
import LocaleSelect from '../components/LocaleSelect.vue'
import SettingsModal from '../components/SettingsModal.vue'
import LoginModal from '../components/modal/LoginModal.vue'
import UserMenu from '../components/UserMenu.vue'
import GlobalProjectSwitcher from '../components/GlobalProjectSwitcher.vue'
import router from '../router'

const SETTINGS_TABS: SettingsTab[] = ['connection', 'models', 'storage', 'backups']

const route = useRoute()
const vueRouter = useRouter()
const authStore = useAuthStore()
const settingsStore = useSettingsStore()
const { t } = useI18n()

// 主题二态切换：图标展示当前状态（浅色=太阳，深色=月亮），历史 system 值经 effectiveTheme 解析后首次点击即转为显式明暗
const themeToggleLabel = computed(() =>
  settingsStore.effectiveTheme === 'dark' ? t('layout.themeToLight') : t('layout.themeToDark')
)

/** 设置按钮的提示/无障碍标签：API Key 失效时改为引导配置。 */
const settingsLabel = computed(() =>
  authStore.lastUnauthorizedAt > 0 ? t('layout.apiKeyInvalidClick') : t('layout.settings')
)

function toggleTheme() {
  settingsStore.setTheme(settingsStore.effectiveTheme === 'dark' ? 'light' : 'dark')
}

onMounted(() => {
  void authStore.bootstrap()
})

watch(
  () => route.query.settings,
  (raw) => {
    if (raw === undefined || raw === null || raw === '') return
    const value = Array.isArray(raw) ? raw[0] : raw
    if (typeof value !== 'string') return
    const tab = SETTINGS_TABS.includes(value as SettingsTab) ? (value as SettingsTab) : undefined
    authStore.openSettings(tab ? { tab } : undefined)
    const query = { ...route.query }
    delete query.settings
    void vueRouter.replace({ path: route.path, query })
  },
  { immediate: true }
)

// 路由守卫：users 仅 superadminl1
watch(
  () => route.name,
  (name) => {
    const r = router.getRoutes().find((rr) => rr.name === name)
    const need = r?.meta?.requiresRole as string | undefined
    if (need === 'superadminl1' && authStore.isAuthenticated && !authStore.isSuper) {
      void vueRouter.replace({ name: 'agents' })
    }
  },
  { immediate: true }
)

const currentTitle = computed(() => {
  const name = route.name as string
  if (!name) return 'SimpleBase'
  const r = router.getRoutes().find((rr) => rr.name === name)
  const key = r?.meta?.titleKey as string | undefined
  if (key) return t(key)
  return (r?.meta?.title as string) || 'SimpleBase'
})

function reload() {
  location.reload()
}
</script>
