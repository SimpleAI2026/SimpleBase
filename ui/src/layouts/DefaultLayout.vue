<template>
  <SidebarProvider>
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <div class="flex h-12 items-center gap-2.5 overflow-hidden px-2 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0">
          <div class="flex size-8 shrink-0 items-center justify-center rounded-md bg-primary font-serif text-[17px] font-semibold text-primary-foreground">
            S
          </div>
          <span class="truncate font-serif text-[17px] font-bold tracking-tight group-data-[collapsible=icon]:hidden">
            SimpleBase
          </span>
        </div>
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupContent>
            <NavMenu />
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarRail />
    </Sidebar>

    <SidebarInset>
      <header class="flex h-[var(--header-height)] shrink-0 items-center justify-between gap-3 border-b bg-background/85 px-5 backdrop-blur-xl md:px-5">
        <div class="flex min-w-0 items-center gap-3">
          <SidebarTrigger />
          <Breadcrumb>
            <BreadcrumbList>
              <BreadcrumbItem>
                <BreadcrumbPage>{{ currentTitle }}</BreadcrumbPage>
              </BreadcrumbItem>
            </BreadcrumbList>
          </Breadcrumb>
        </div>
        <div class="flex items-center gap-2.5 sm:gap-3.5">
          <GlobalProjectSwitcher />
          <Badge v-if="isMock" variant="warning">Mock 数据</Badge>
          <Tooltip>
            <TooltipTrigger as-child>
              <Button variant="ghost" size="icon" @click="authStore.openDrawer()">
                <span class="relative inline-flex">
                  <SettingsIcon />
                  <span
                    v-if="authStore.lastUnauthorizedAt > 0"
                    class="absolute -top-0.5 -right-0.5 size-2 rounded-full bg-destructive"
                  />
                </span>
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              {{ authStore.lastUnauthorizedAt > 0 ? 'API Key 无效，点击配置' : '连接设置' }}
            </TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger as-child>
              <Button variant="ghost" size="icon" as-child>
                <router-link to="/docs" target="_blank" rel="noopener noreferrer" class="inline-flex items-center gap-1">
                  <BookOpenIcon />
                  <span class="hidden text-xs lg:inline">使用文档</span>
                </router-link>
              </Button>
            </TooltipTrigger>
            <TooltipContent>使用文档</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger as-child>
              <Button variant="ghost" size="icon" as-child>
                <a href="https://github.com/linkxzhou/SimpleBase" target="_blank" rel="noopener noreferrer">
                  <GithubMark />
                </a>
              </Button>
            </TooltipTrigger>
            <TooltipContent>GitHub</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger as-child>
              <Button variant="ghost" size="icon" @click="reload">
                <RefreshCwIcon />
              </Button>
            </TooltipTrigger>
            <TooltipContent>刷新页面</TooltipContent>
          </Tooltip>
        </div>
      </header>

      <div class="mx-auto w-full max-w-[var(--content-max-width)] flex-1 p-2.5 md:p-[18px]">
        <router-view v-slot="{ Component }">
          <transition name="sb-fade" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </div>
    </SidebarInset>

    <ApiKeyDrawer />
  </SidebarProvider>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { BookOpenIcon, RefreshCwIcon, SettingsIcon } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Breadcrumb, BreadcrumbItem, BreadcrumbList, BreadcrumbPage } from '@/components/ui/breadcrumb'
import { Button } from '@/components/ui/button'
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarInset,
  SidebarProvider,
  SidebarRail,
  SidebarTrigger,
} from '@/components/ui/sidebar'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { isMock } from '../services/api'
import { useAuthStore } from '../stores/auth'
import NavMenu from '../components/NavMenu.vue'
import ApiKeyDrawer from '../components/ApiKeyDrawer.vue'
import GlobalProjectSwitcher from '../components/GlobalProjectSwitcher.vue'
import GithubMark from '../components/icons/GithubMark.vue'
import router from '../router'

const route = useRoute()
const authStore = useAuthStore()

const currentTitle = computed(() => {
  const name = route.name as string
  if (!name) return 'SimpleBase'
  const r = router.getRoutes().find((rr) => rr.name === name)
  return (r?.meta?.title as string) || 'SimpleBase'
})

function reload() {
  location.reload()
}
</script>
