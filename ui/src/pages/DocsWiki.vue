<template>
  <div class="docs-wiki">
    <div v-if="!docCatalog.length" class="docs-error">
      <a-result status="warning" title="未能加载文档">
        <template #subTitle>
          Vite 未匹配到仓库 <code>docs/&lt;module&gt;/*.md</code>
          （已扫描 {{ loadedMarkdownCount }} 个文件）。请确认源文件存在后重新构建。
        </template>
      </a-result>
    </div>

    <template v-else>
      <DocsModuleTabs v-model="moduleId" :modules="docCatalog" />

      <div class="docs-wiki-body">
        <template v-if="!isMobile">
          <DocsSidebar
            v-if="currentModule"
            :module-id="currentModule.id"
            :module-title="currentModule.title"
            :pages="currentModule.pages"
            :active-slug="slug"
          />
        </template>
        <a-select
          v-else-if="currentModule"
          class="docs-mobile-pages"
          :value="slug"
          style="width: 100%; margin-bottom: 12px"
          :options="pageOptions"
          @change="onMobilePage"
        />

        <DocsArticle
          v-if="currentModule && currentPage"
          :module-id="currentModule.id"
          :module-title="currentModule.title"
          :page="currentPage"
          :prev="prevPage"
          :next="nextPage"
        />
        <div v-else class="docs-error">
          <a-result
            status="404"
            title="未找到文档"
            :sub-title="missingHint"
          />
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
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

<style scoped>
.docs-wiki {
  display: flex;
  flex-direction: column;
  min-height: calc(100vh - var(--sb-header-height, 56px));
  background: var(--sb-surface, #ffffff);
}
.docs-wiki-body {
  display: flex;
  align-items: flex-start;
  gap: 0;
  flex: 1;
  min-height: 0;
}
.docs-mobile-pages {
  margin-bottom: 8px;
}
.docs-error {
  flex: 1;
  padding: 24px 16px 48px;
}

@media (max-width: 768px) {
  .docs-wiki-body {
    flex-direction: column;
    padding: 0 12px 24px;
  }
}
</style>
