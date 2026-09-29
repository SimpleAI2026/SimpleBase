<template>
  <div class="relative flex min-h-[calc(100vh-var(--header-height))] flex-col overflow-x-clip bg-[radial-gradient(ellipse_at_top_left,oklch(0.88_0.09_210_/_0.22),transparent_45%),linear-gradient(to_bottom,var(--background),var(--background))] [--docs-pad-y:2rem] [--docs-tabs-h:3.25rem] max-md:[--docs-pad-y:1.25rem]">
    <div v-if="!docCatalog.length" class="mx-auto w-full max-w-[1200px] px-4 py-12 md:px-6">
      <Alert>
        <AlertTitle>未能加载文档</AlertTitle>
        <AlertDescription>
          Vite 未匹配到仓库 <code>docs/&lt;module&gt;/*.md</code>
          （已扫描 {{ loadedMarkdownCount }} 个文件）。请确认源文件存在后重新构建。
        </AlertDescription>
      </Alert>
    </div>

    <template v-else>
      <div
        data-docs-tabs
        class="sticky top-[var(--header-height)] z-10 border-b-0 bg-background/85 shadow-[0_1px_0_0_var(--border)] backdrop-blur-xl"
      >
        <Tabs :model-value="moduleId" class="gap-0" @update:model-value="onModuleTab">
          <div class="mx-auto w-full px-4 md:px-8 lg:px-10">
            <TabsList
              variant="line"
              class="h-[var(--docs-tabs-h)] w-full justify-start gap-1 overflow-x-auto rounded-none border-b-0 bg-transparent p-0 group-data-[orientation=horizontal]/tabs:h-[var(--docs-tabs-h)]"
            >
              <TabsTrigger
                v-for="m in docCatalog"
                :key="m.id"
                :value="m.id"
                class="h-[var(--docs-tabs-h)] flex-none cursor-pointer px-4 text-sm font-medium transition-colors duration-200 after:h-0.5 after:rounded-t-full after:bg-primary hover:bg-primary/5 hover:text-primary focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:ring-offset-2 data-active:bg-primary/5 data-active:font-semibold data-active:text-primary group-data-[orientation=horizontal]/tabs:after:bottom-0 motion-reduce:transition-none dark:data-active:text-primary"
              >
                {{ m.title }}
              </TabsTrigger>
            </TabsList>
          </div>
        </Tabs>
      </div>

      <div class="w-full px-4 pt-7 md:px-8 lg:px-10">
        <div class="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-primary/10 bg-card/70 px-5 py-4 shadow-sm backdrop-blur-sm sm:px-7">
          <div class="flex min-w-0 items-center gap-3">
            <span class="flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary ring-1 ring-primary/15" aria-hidden="true"><BookOpen class="size-5" /></span>
            <div class="min-w-0">
              <p class="text-[11px] font-semibold tracking-[0.16em] text-primary uppercase">SimpleBase · Documentation</p>
              <p class="truncate text-lg font-semibold tracking-tight text-foreground">{{ currentModule?.title || '使用文档' }}</p>
            </div>
          </div>
          <span v-if="currentModule" data-docs-progress class="rounded-full border border-primary/15 bg-primary/5 px-3 py-1 text-xs font-medium text-primary">第 {{ pageIndex + 1 }} / {{ currentModule.pages.length }} 篇</span>
        </div>
      </div>

      <div class="flex min-h-0 w-full flex-1 items-start gap-6 px-4 py-[var(--docs-pad-y)] max-md:flex-col md:px-8 lg:gap-8 lg:px-10">
        <DocsSidebar
          v-if="!isMobile && currentModule"
          :module-id="currentModule.id"
          :module-title="currentModule.title"
          :pages="currentModule.pages"
          :active-slug="slug"
        />
        <div v-else-if="currentModule" class="w-full rounded-xl border border-border/70 bg-card/80 p-4 shadow-sm">
          <p id="docs-page-label" class="mb-2 text-xs font-semibold tracking-wide text-muted-foreground">选择文档页面</p>
          <Select :model-value="slug" @update:model-value="onMobilePage">
            <SelectTrigger class="w-full bg-card" aria-labelledby="docs-page-label">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem v-for="p in pageOptions" :key="p.value" :value="p.value">{{ p.label }}</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>

        <DocsArticle
          v-if="currentModule && currentPage"
          :module-id="currentModule.id"
          :module-title="currentModule.title"
          :page="currentPage"
          :prev="prevPage"
          :next="nextPage"
        />
        <div v-else class="min-w-0 flex-1">
          <Alert>
            <AlertTitle>未找到文档</AlertTitle>
            <AlertDescription>{{ missingHint }}</AlertDescription>
          </Alert>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { BookOpen } from '@lucide/vue'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import DocsSidebar from '../components/docs/DocsSidebar.vue'
import DocsArticle from '../components/docs/DocsArticle.vue'
import {
  docCatalog,
  defaultModuleId,
  defaultSlug,
  getModule,
  getPage,
  loadedMarkdownCount
} from '../docs/catalog'

const route = useRoute()
const router = useRouter()

const moduleId = ref(defaultModuleId())
const slug = ref(defaultSlug(moduleId.value))
const isMobile = ref(window.innerWidth <= 768)

function onResize() {
  isMobile.value = window.innerWidth <= 768
}
onMounted(() => window.addEventListener('resize', onResize))
onBeforeUnmount(() => window.removeEventListener('resize', onResize))

const currentModule = computed(() => getModule(moduleId.value))
const currentPage = computed(() => getPage(moduleId.value, slug.value))

const pageOptions = computed(() =>
  (currentModule.value?.pages || []).map((p) => ({ label: p.title, value: p.slug }))
)

const pageIndex = computed(
  () => currentModule.value?.pages.findIndex((p) => p.slug === slug.value) ?? -1
)
const prevPage = computed(() => {
  const pages = currentModule.value?.pages
  if (!pages || pageIndex.value <= 0) return null
  return pages[pageIndex.value - 1]
})
const nextPage = computed(() => {
  const pages = currentModule.value?.pages
  if (!pages || pageIndex.value < 0 || pageIndex.value >= pages.length - 1) return null
  return pages[pageIndex.value + 1]
})

const missingHint = computed(() => {
  const m = (route.params.module as string) || ''
  const s = (route.params.slug as string) || ''
  if (m && s) return `没有文档 ${m}/${s}`
  if (m) return `没有模块 ${m}`
  return '请从左侧选择一篇文档'
})

function syncFromRoute() {
  if (!docCatalog.length) return
  const m = (route.params.module as string) || defaultModuleId()
  const s = (route.params.slug as string) || defaultSlug(m)
  if (!getModule(m)) {
    router.replace(`/docs/${defaultModuleId()}`)
    return
  }
  const page = getPage(m, s) || getPage(m, defaultSlug(m))
  if (!page) {
    router.replace(`/docs/${m}`)
    return
  }
  moduleId.value = m
  slug.value = page.slug
  if (route.params.slug === 'index') {
    router.replace(`/docs/${m}`)
  }
}

watch(() => [route.params.module, route.params.slug], syncFromRoute, { immediate: true })

watch(moduleId, (id, prev) => {
  if (id === prev) return
  if (route.params.module !== id) {
    router.push(`/docs/${id}`)
  }
})

function onModuleTab(v: string | number) {
  moduleId.value = String(v)
}

function onMobilePage(v: unknown) {
  const slugVal = String(v)
  if (slugVal === 'index') router.push(`/docs/${moduleId.value}`)
  else router.push(`/docs/${moduleId.value}/${slugVal}`)
}
</script>
