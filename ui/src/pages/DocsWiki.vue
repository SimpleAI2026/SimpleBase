<template>
  <div class="flex min-h-[calc(100vh-var(--header-height))] flex-col bg-background">
    <div v-if="!docCatalog.length" class="px-4 py-12">
      <Alert>
        <AlertTitle>未能加载文档</AlertTitle>
        <AlertDescription>
          Vite 未匹配到仓库 <code>docs/&lt;module&gt;/*.md</code>
          （已扫描 {{ loadedMarkdownCount }} 个文件）。请确认源文件存在后重新构建。
        </AlertDescription>
      </Alert>
    </div>

    <template v-else>
      <div class="sticky top-[var(--header-height)] z-10 border-b bg-background/85 backdrop-blur-xl">
        <DocsModuleTabs v-model="moduleId" :modules="docCatalog" />
      </div>

      <div class="mx-auto flex min-h-0 w-full max-w-[1200px] flex-1 items-start gap-6 px-4 py-6 max-md:flex-col max-md:px-3 max-md:pb-6 md:px-6">
        <DocsSidebar
          v-if="!isMobile && currentModule"
          :module-id="currentModule.id"
          :module-title="currentModule.title"
          :pages="currentModule.pages"
          :active-slug="slug"
        />
        <div v-else-if="currentModule" class="mb-3 w-full">
          <Select :model-value="slug" @update:model-value="onMobilePage">
            <SelectTrigger class="w-full">
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
        <div v-else class="flex-1 px-4 py-12">
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
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import DocsModuleTabs from '../components/docs/DocsModuleTabs.vue'
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

function onMobilePage(v: unknown) {
  const slugVal = String(v)
  if (slugVal === 'index') router.push(`/docs/${moduleId.value}`)
  else router.push(`/docs/${moduleId.value}/${slugVal}`)
}
</script>
