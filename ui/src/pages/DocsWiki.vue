<template>
  <div class="relative flex min-h-[calc(100svh-var(--header-height))] flex-col overflow-x-clip bg-background [--docs-pad-y:2.25rem] [--docs-tabs-h:3.5rem] max-md:[--docs-pad-y:1.25rem]">
    <div v-if="!docCatalog.length" class="mx-auto w-full max-w-[1440px] px-4 py-12 sm:px-6 lg:px-8">
      <Alert>
        <AlertTitle>{{ t('pages.docsWiki.loadFailedTitle') }}</AlertTitle>
        <AlertDescription>{{ t('pages.docsWiki.loadFailedDesc', { count: loadedMarkdownCount }) }}</AlertDescription>
      </Alert>
    </div>

    <template v-else>
      <div
        data-docs-tabs
        class="sticky top-[var(--header-height)] z-20 border-b border-border/70 bg-background/85 backdrop-blur-xl"
      >
        <Tabs :model-value="moduleId" class="gap-0" @update:model-value="onModuleTab">
          <div class="mx-auto w-full max-w-[1440px] px-4 sm:px-6 lg:px-8">
            <TabsList
              variant="line"
              class="h-[var(--docs-tabs-h)] w-full justify-start gap-2 overflow-x-auto rounded-none border-b-0 bg-transparent p-0 group-data-[orientation=horizontal]/tabs:h-[var(--docs-tabs-h)]"
            >
              <TabsTrigger
                v-for="m in docCatalog"
                :key="m.id"
                :value="m.id"
                class="h-[var(--docs-tabs-h)] flex-none cursor-pointer px-3 text-sm font-medium text-muted-foreground transition-colors duration-200 after:h-0.5 after:rounded-t-full after:bg-primary hover:bg-muted/50 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:ring-offset-2 data-[state=active]:bg-transparent data-[state=active]:font-semibold data-[state=active]:text-primary group-data-[orientation=horizontal]/tabs:after:bottom-0 motion-reduce:transition-none sm:px-4"
              >
                {{ m.title }}
              </TabsTrigger>
            </TabsList>
          </div>
        </Tabs>
      </div>

      <div class="mx-auto flex min-h-0 w-full max-w-[1440px] flex-1 items-start gap-8 px-4 py-[var(--docs-pad-y)] max-md:flex-col sm:px-6 lg:px-8 xl:gap-10">
        <DocsSidebar
          v-if="currentModule"
          :module-id="currentModule.id"
          :pages="currentModule.pages"
          :active-slug="slug"
        />
        <div v-if="currentModule" class="w-full md:hidden">
          <p id="docs-page-label" class="mb-2 text-[11px] font-semibold tracking-[0.08em] text-muted-foreground">{{ t('pages.docsWiki.selectPage') }}</p>
          <Select :model-value="slug" @update:model-value="onMobilePage">
            <SelectTrigger class="h-10 w-full rounded-lg border-border/80 bg-card shadow-none" aria-labelledby="docs-page-label">
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
          :page-index="pageIndex + 1"
          :page-count="currentModule.pages.length"
        />
        <div v-else class="min-w-0 flex-1">
          <Alert>
            <AlertTitle>{{ t('pages.docsWiki.notFoundTitle') }}</AlertTitle>
            <AlertDescription>{{ missingHint }}</AlertDescription>
          </Alert>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
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
  loadedMarkdownCount,
  resolveLegacyDocPath
} from '../docs/catalog'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const moduleId = ref(defaultModuleId())
const slug = ref(defaultSlug(moduleId.value))
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
  if (m && s) return t('pages.docsWiki.noDocAt', { module: m, slug: s })
  if (m) return t('pages.docsWiki.noModule', { module: m })
  return t('pages.docsWiki.pickFromSidebar')
})

function syncFromRoute() {
  if (!docCatalog.length) return
  const m = (route.params.module as string) || defaultModuleId()
  const s = (route.params.slug as string) || defaultSlug(m)
  const legacy = resolveLegacyDocPath(m, s)
  if (legacy) {
    router.replace(legacy)
    return
  }
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
  document.title = t('pages.docsWiki.documentTitle', {
    page: page.title,
    module: getModule(m)?.title || t('pages.docsWiki.documentFallbackModule')
  })
  if (route.params.slug === 'index') {
    router.replace(`/docs/${m}`)
  }
}

watch(() => [route.params.module, route.params.slug], syncFromRoute, { immediate: true })

/** Tab 数量多于一屏时，把激活项滚到横向可视区中间。 */
function revealActiveTab() {
  const el = document.querySelector<HTMLElement>('[data-docs-tabs] [data-state="active"]')
  if (el && typeof el.scrollIntoView === 'function') {
    el.scrollIntoView({ inline: 'center', block: 'nearest' })
  }
}

watch(moduleId, () => { void nextTick(revealActiveTab) }, { flush: 'post' })
onMounted(() => { void nextTick(revealActiveTab) })

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
