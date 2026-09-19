<template>
  <article class="min-w-0 w-full max-w-[880px] flex-1 rounded-xl border bg-card px-5 pt-6 pb-10 shadow-sm md:px-9 md:pt-7">
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
        class="inline-flex items-center gap-1 rounded-md border px-2.5 py-1 text-[12.5px] text-muted-foreground no-underline transition-colors hover:border-primary/40 hover:bg-primary/5 hover:text-primary"
        :href="githubUrl"
        target="_blank"
        rel="noopener noreferrer"
      >
        在 GitHub 上查看 ↗
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
    <div v-if="prev || next" class="mt-10 flex justify-between gap-3 border-t pt-6">
      <router-link
        v-if="prev"
        class="flex max-w-[48%] flex-col gap-0.5 rounded-lg border px-4 py-2.5 no-underline transition-colors hover:border-primary/40 hover:bg-primary/5"
        :to="linkFor(prev.slug)"
      >
        <span class="text-[11.5px] text-muted-foreground">← 上一篇</span>
        <span class="truncate text-sm font-medium text-primary">{{ prev.title }}</span>
      </router-link>
      <span v-else />
      <router-link
        v-if="next"
        class="flex max-w-[48%] flex-col items-end gap-0.5 rounded-lg border px-4 py-2.5 text-right no-underline transition-colors hover:border-primary/40 hover:bg-primary/5"
        :to="linkFor(next.slug)"
      >
        <span class="text-[11.5px] text-muted-foreground">下一篇 →</span>
        <span class="truncate text-sm font-medium text-primary">{{ next.title }}</span>
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
.docs-md :deep(h1),
.docs-md :deep(h2),
.docs-md :deep(h3),
.docs-md :deep(h4) {
  font-family: var(--font-heading);
  font-weight: 700;
  scroll-margin-top: calc(var(--header-height) + 72px);
}
.docs-md :deep(h1) {
  font-size: 1.8rem;
  margin: 0 0 1.1rem;
  border-bottom: 1px solid var(--border);
  padding-bottom: 0.5rem;
}
.docs-md :deep(h2) {
  font-size: 1.4rem;
  margin: 1.8rem 0 0.8rem;
  border-bottom: 1px solid var(--border);
  padding-bottom: 0.35rem;
}
.docs-md :deep(h3) {
  font-size: 1.18rem;
  margin: 1.4rem 0 0.55rem;
}
.docs-md :deep(h4) {
  font-size: 1rem;
  margin: 1.1rem 0 0.45rem;
}
.docs-md :deep(p),
.docs-md :deep(ul),
.docs-md :deep(ol) {
  line-height: 1.8;
  margin: 0.65rem 0;
  color: var(--foreground);
}
.docs-md :deep(ul),
.docs-md :deep(ol) {
  padding-left: 1.4rem;
}
.docs-md :deep(li) {
  margin: 0.25rem 0;
}
.docs-md :deep(li::marker) {
  color: var(--primary);
}
.docs-md :deep(a) {
  color: var(--primary);
  text-decoration: none;
  text-decoration-color: rgb(217 119 87 / 0.4);
  text-underline-offset: 3px;
}
.docs-md :deep(a:hover) {
  text-decoration: underline;
}
.docs-md :deep(strong) {
  font-weight: 600;
}
.docs-md :deep(code) {
  font-family: var(--font-mono);
  font-size: 0.88em;
  background: var(--muted);
  border: 1px solid var(--border);
  padding: 0.12em 0.4em;
  border-radius: 5px;
}
.docs-md :deep(pre) {
  background: #1e1e1e;
  color: #e5e5e5;
  border: 1px solid rgb(255 255 255 / 0.08);
  box-shadow: 0 1px 2px rgb(0 0 0 / 0.08);
  padding: 14px 16px;
  margin: 1rem 0;
  border-radius: 10px;
  overflow: auto;
  font-size: 13px;
  line-height: 1.6;
}
.docs-md :deep(pre code) {
  background: transparent;
  border: none;
  padding: 0;
  color: inherit;
}
.docs-md :deep(table) {
  border-collapse: collapse;
  width: 100%;
  margin: 1.1rem 0;
  font-size: 13.5px;
}
.docs-md :deep(th),
.docs-md :deep(td) {
  border: 1px solid var(--border);
  padding: 8px 12px;
  text-align: left;
}
.docs-md :deep(th) {
  background: var(--muted);
  font-weight: 600;
}
.docs-md :deep(tbody tr:hover) {
  background: var(--muted);
}
.docs-md :deep(blockquote) {
  margin: 1rem 0;
  padding: 0.6rem 1rem;
  border-left: 3px solid var(--primary);
  border-radius: 0 8px 8px 0;
  background: rgb(217 119 87 / 0.06);
  color: var(--muted-foreground);
}
.docs-md :deep(blockquote p) {
  margin: 0.25rem 0;
}
.docs-md :deep(hr) {
  border: none;
  border-top: 1px solid var(--border);
  margin: 1.6rem 0;
}
.docs-md :deep(img) {
  max-width: 100%;
  border-radius: 10px;
  border: 1px solid var(--border);
}
</style>
