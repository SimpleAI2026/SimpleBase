<template>
  <article class="min-w-0 max-w-[900px] flex-1 px-2 pb-12 pt-4 md:px-6">
    <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
      <Breadcrumb>
        <BreadcrumbList>
          <BreadcrumbItem>
            <BreadcrumbLink as-child>
              <router-link to="/docs">使用文档</router-link>
            </BreadcrumbLink>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            <BreadcrumbPage>{{ moduleTitle }}</BreadcrumbPage>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            <BreadcrumbPage>{{ title }}</BreadcrumbPage>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>
      <a
        v-if="githubUrl"
        class="text-[13px] text-muted-foreground no-underline hover:text-primary"
        :href="githubUrl"
        target="_blank"
        rel="noopener noreferrer"
      >
        在 GitHub 上查看
      </a>
    </div>
    <div v-if="loadError" class="py-6">
      <Alert>
        <AlertTitle>{{ loadError }}</AlertTitle>
        <AlertDescription>源文件未打进前端产物，页面不会空白。</AlertDescription>
      </Alert>
    </div>
    <div v-else-if="!html" class="py-6">
      <Alert>
        <AlertTitle>这篇文档没有正文</AlertTitle>
        <AlertDescription>docs/{{ page.filePath }}</AlertDescription>
      </Alert>
    </div>
    <div v-else class="docs-md" v-html="html" />
    <div v-if="prev || next" class="mt-8 flex justify-between border-t pt-4">
      <router-link v-if="prev" class="text-sm text-primary no-underline hover:underline" :to="linkFor(prev.slug)">
        ← {{ prev.title }}
      </router-link>
      <span v-else />
      <router-link v-if="next" class="text-sm text-primary no-underline hover:underline" :to="linkFor(next.slug)">
        {{ next.title }} →
      </router-link>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import type { DocPage } from '../../docs/catalog'
import { githubBlobUrl, loadMarkdownRaw } from '../../docs/catalog'
import { renderMarkdown } from '../../docs/render'

const props = defineProps<{
  moduleId: string
  moduleTitle: string
  page: DocPage
  prev?: DocPage | null
  next?: DocPage | null
}>()

const title = computed(() => props.page.title)
const raw = computed(() => loadMarkdownRaw(props.page.filePath))
const loadError = computed(() =>
  raw.value == null ? `文档文件缺失：docs/${props.page.filePath}` : null
)
const html = computed(() => {
  if (raw.value == null) return ''
  if (!raw.value.trim()) return ''
  try {
    return renderMarkdown(raw.value, props.moduleId)
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err)
    return `<p>文档渲染失败：${msg}</p>`
  }
})
const githubUrl = computed(() => githubBlobUrl(props.page.filePath))

function linkFor(slug: string) {
  if (slug === 'index') return `/docs/${props.moduleId}`
  return `/docs/${props.moduleId}/${slug}`
}
</script>

<style scoped>
.docs-md :deep(h1) {
  font-size: 1.75rem;
  margin: 0 0 1rem;
  border-bottom: 1px solid var(--border);
  padding-bottom: 0.4rem;
}
.docs-md :deep(h2) {
  font-size: 1.35rem;
  margin: 1.6rem 0 0.75rem;
  border-bottom: 1px solid var(--border);
  padding-bottom: 0.3rem;
}
.docs-md :deep(h3) {
  font-size: 1.15rem;
  margin: 1.25rem 0 0.5rem;
}
.docs-md :deep(p),
.docs-md :deep(ul),
.docs-md :deep(ol) {
  line-height: 1.7;
  margin: 0.6rem 0;
  color: var(--foreground);
}
.docs-md :deep(a) {
  color: var(--primary);
}
.docs-md :deep(code) {
  font-family: var(--font-mono);
  font-size: 0.9em;
  background: var(--muted);
  padding: 0.1em 0.35em;
  border-radius: 4px;
}
.docs-md :deep(pre) {
  background: #1e1e1e;
  color: #e5e5e5;
  padding: 12px 14px;
  border-radius: 8px;
  overflow: auto;
  font-size: 13px;
  line-height: 1.5;
}
.docs-md :deep(pre code) {
  background: transparent;
  padding: 0;
  color: inherit;
}
.docs-md :deep(table) {
  border-collapse: collapse;
  width: 100%;
  margin: 1rem 0;
  font-size: 14px;
}
.docs-md :deep(th),
.docs-md :deep(td) {
  border: 1px solid var(--border);
  padding: 8px 10px;
  text-align: left;
}
.docs-md :deep(blockquote) {
  margin: 1rem 0;
  padding: 0.25rem 0 0.25rem 1rem;
  border-left: 4px solid var(--border);
  color: var(--muted-foreground);
}
</style>
