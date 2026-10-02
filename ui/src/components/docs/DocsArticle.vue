<template>
  <article class="mx-auto min-w-0 w-full max-w-[820px] flex-1 bg-white py-1 dark:bg-transparent">
    <div class="mb-8 flex flex-wrap items-center justify-between gap-3">
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
            <BreadcrumbPage class="font-medium">{{ title }}</BreadcrumbPage>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>
      <a
        v-if="githubUrl"
        class="inline-flex shrink-0 items-center gap-1 rounded-md border border-border bg-background px-2.5 py-1 text-xs text-muted-foreground no-underline transition-colors hover:border-primary/40 hover:bg-primary/5 hover:text-primary"
        :href="githubUrl"
        target="_blank"
        rel="noopener noreferrer"
      >
        在 GitHub 上查看
        <ExternalLinkIcon class="size-3.5" />
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
    <div v-if="prev || next" class="mt-12 grid gap-4 border-t border-border/70 pt-8 sm:grid-cols-2">
      <router-link
        v-if="prev"
        class="group flex min-w-0 flex-col gap-1 rounded-xl border border-border/70 bg-white px-5 py-4 no-underline transition-all hover:-translate-y-0.5 hover:border-primary/40 hover:shadow-md dark:bg-card"
        :to="linkFor(prev.slug)"
      >
        <span class="text-xs text-muted-foreground">上一篇</span>
        <span class="truncate text-sm font-medium text-primary">← {{ prev.title }}</span>
      </router-link>
      <router-link
        v-if="next"
        class="group flex min-w-0 flex-col gap-1 rounded-xl border border-border/70 bg-white px-5 py-4 no-underline transition-all hover:-translate-y-0.5 hover:border-primary/40 hover:shadow-md sm:col-start-2 sm:items-end sm:text-right dark:bg-card"
        :to="linkFor(next.slug)"
      >
        <span class="text-xs text-muted-foreground">下一篇</span>
        <span class="truncate text-sm font-medium text-primary">{{ next.title }} →</span>
      </router-link>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { ExternalLinkIcon } from '@lucide/vue'
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
.docs-md {
  color: var(--foreground);
  font-size: 15px;
  line-height: 1.8;
  word-break: break-word;
}
.docs-md :deep(> :first-child) {
  margin-top: 0;
}
.docs-md :deep(h1),
.docs-md :deep(h2),
.docs-md :deep(h3),
.docs-md :deep(h4) {
  font-family: var(--font-heading);
  font-weight: 700;
  line-height: 1.35;
  letter-spacing: -0.01em;
  color: var(--foreground);
  scroll-margin-top: calc(var(--header-height) + var(--docs-tabs-h, 2.75rem) + 0.75rem);
}
.docs-md :deep(h1) {
  font-size: 2rem;
  margin: 0 0 1.25rem;
  padding-bottom: 0.75rem;
  border-bottom: 1px solid var(--border);
}
.docs-md :deep(h2) {
  font-size: 1.45rem;
  margin: 2.5rem 0 1rem;
  padding-bottom: 0.45rem;
  border-bottom: 1px solid color-mix(in srgb, var(--border) 70%, transparent);
}
.docs-md :deep(h3) {
  font-size: 1.2rem;
  margin: 1.9rem 0 0.7rem;
}
.docs-md :deep(h4) {
  font-size: 1.02rem;
  margin: 1.5rem 0 0.5rem;
}
.docs-md :deep(p),
.docs-md :deep(ul),
.docs-md :deep(ol) {
  margin: 0.85rem 0;
  color: color-mix(in srgb, var(--foreground) 88%, transparent);
}
.docs-md :deep(ul),
.docs-md :deep(ol) {
  padding-left: 1.5rem;
}
.docs-md :deep(ul) {
  list-style: disc;
}
.docs-md :deep(ol) {
  list-style: decimal;
}
.docs-md :deep(li) {
  margin: 0.35rem 0;
}
.docs-md :deep(li > ul),
.docs-md :deep(li > ol) {
  margin: 0.25rem 0;
}
.docs-md :deep(li::marker) {
  color: var(--primary);
}
.docs-md :deep(a) {
  color: var(--primary);
  font-weight: 500;
  text-decoration: underline;
  text-decoration-color: color-mix(in srgb, var(--primary) 30%, transparent);
  text-underline-offset: 3px;
  transition: text-decoration-color 0.15s;
}
.docs-md :deep(a:hover) {
  text-decoration-color: var(--primary);
}
.docs-md :deep(strong) {
  font-weight: 600;
  color: var(--foreground);
}
.docs-md :deep(:not(pre) > code) {
  font-family: var(--font-mono);
  font-size: 0.86em;
  color: color-mix(in srgb, var(--primary) 85%, var(--foreground));
  background: color-mix(in srgb, var(--primary) 7%, transparent);
  padding: 0.15em 0.42em;
  border-radius: 0.375rem;
}
.docs-md :deep(pre) {
  background: #f6f8fa;
  color: #1f2328;
  border: 1px solid #e5e7eb;
  padding: 16px 18px;
  margin: 1.2rem 0;
  border-radius: 10px;
  overflow: auto;
  font-family: var(--font-mono);
  font-size: 13px;
  line-height: 1.65;
}
:global(.dark) .docs-md :deep(pre) {
  background: var(--muted);
  color: var(--foreground);
  border-color: var(--border);
}
.docs-md :deep(pre code) {
  background: transparent;
  border: none;
  padding: 0;
  color: inherit;
  font-size: inherit;
}
.docs-md :deep(table) {
  display: block;
  width: 100%;
  overflow-x: auto;
  border-collapse: separate;
  border-spacing: 0;
  margin: 1.3rem 0;
  font-size: 0.875rem;
  border: 1px solid var(--border);
  border-radius: 10px;
}
.docs-md :deep(th),
.docs-md :deep(td) {
  padding: 10px 14px;
  text-align: left;
  border-bottom: 1px solid var(--border);
}
.docs-md :deep(th + th),
.docs-md :deep(td + td) {
  border-left: 1px solid var(--border);
}
.docs-md :deep(tbody tr:last-child td) {
  border-bottom: none;
}
.docs-md :deep(th) {
  background: color-mix(in srgb, var(--muted) 70%, transparent);
  font-weight: 600;
  white-space: nowrap;
}
.docs-md :deep(tbody tr:hover) {
  background: color-mix(in srgb, var(--muted) 45%, transparent);
}
.docs-md :deep(blockquote) {
  margin: 1.2rem 0;
  padding: 0.75rem 1.1rem;
  border-left: 3px solid var(--primary);
  border-radius: 0 10px 10px 0;
  background: color-mix(in srgb, var(--primary) 6%, transparent);
  color: var(--muted-foreground);
}
.docs-md :deep(blockquote p) {
  margin: 0.25rem 0;
  color: inherit;
}
.docs-md :deep(hr) {
  border: none;
  border-top: 1px solid var(--border);
  margin: 2rem 0;
}
.docs-md :deep(img) {
  display: block;
  max-width: 100%;
  margin: 1.2rem auto;
  border-radius: 10px;
  border: 1px solid var(--border);
}
</style>
