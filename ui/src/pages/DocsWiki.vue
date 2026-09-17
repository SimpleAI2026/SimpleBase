<template>
  <div class="docs-wiki">
    <div class="docs-wiki-head">
      <h1 class="docs-wiki-title">使用文档</h1>
      <p class="docs-wiki-sub">产品使用说明（只读 Wiki）</p>
    </div>

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
      <a-empty v-else description="未找到文档" />
    </div>
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
  getPage
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

function syncFromRoute() {
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
  background: var(--sb-surface, #faf9f5);
  border: 1px solid var(--sb-border, #e8e6e0);
  border-radius: 10px;
  padding: 16px 20px 8px;
  min-height: calc(100vh - 140px);
}
.docs-wiki-head {
  margin-bottom: 8px;
}
.docs-wiki-title {
  margin: 0;
  font-size: 22px;
  font-family: var(--sb-font-serif, Georgia, serif);
}
.docs-wiki-sub {
  margin: 4px 0 12px;
  color: var(--sb-text-secondary, #6b7280);
  font-size: 13px;
}
.docs-wiki-body {
  display: flex;
  align-items: flex-start;
  gap: 0;
  padding-top: 12px;
}
.docs-mobile-pages {
  margin-bottom: 8px;
}

@media (max-width: 768px) {
  .docs-wiki {
    padding: 12px;
  }
  .docs-wiki-body {
    flex-direction: column;
  }
}
</style>
