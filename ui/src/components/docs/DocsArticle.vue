<template>
  <div class="flex min-w-0 w-full flex-1 items-start gap-7">
  <article class="min-w-0 w-full flex-1 bg-white py-1 dark:bg-transparent">
    <div class="mb-7 flex flex-wrap items-center justify-between gap-3">
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
      <span class="ml-auto text-xs text-muted-foreground" data-docs-progress>第 {{ pageIndex }} / {{ pageCount }} 篇</span>
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
    <SbAsyncRegion
      block="lines"
      class="min-h-40"
      :pending="pending"
      :show-skeleton="showSkeleton"
      :show-empty="showEmpty"
      :show-error="showError"
      :refreshing="refreshing"
      :error="docError"
      @retry="reload"
    >
      <template #empty>
        <div class="py-6">
          <Alert>
            <AlertTitle>这篇文档没有正文</AlertTitle>
            <AlertDescription>docs/{{ page.filePath }}</AlertDescription>
          </Alert>
        </div>
      </template>
      <div class="docs-md" v-html="html" @click="onContentClick" />
    </SbAsyncRegion>
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
  <DocsToc :headings="rendered.toc" />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { toast } from 'vue-sonner'
import { ExternalLinkIcon } from '@lucide/vue'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import { useLoadState } from '@/composables/useLoadState'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import type { DocPage } from '../../docs/catalog'
import { githubBlobUrl, loadMarkdown } from '../../docs/catalog'
import { renderMarkdown, type DocHeading } from '../../docs/render'
import DocsToc from './DocsToc.vue'

const props = defineProps<{
  moduleId: string
  moduleTitle: string
  page: DocPage
  prev?: DocPage | null
  next?: DocPage | null
  pageIndex?: number
  pageCount?: number
}>()

const route = useRoute()
const title = computed(() => props.page.title)
const raw = ref<string | null>(null)
const {
  pending,
  showSkeleton,
  showEmpty,
  showError,
  refreshing,
  error: docError,
  run
} = useLoadState()
const rendered = computed(() => {
  if (!raw.value?.trim()) return { html: '', toc: [] as DocHeading[] }
  try { return renderMarkdown(raw.value, props.moduleId) }
  catch { return { html: '<p>文档渲染失败</p>', toc: [] as DocHeading[] } }
})
const html = computed(() => rendered.value.html)
const githubUrl = computed(() => githubBlobUrl(props.page.filePath))
let generation = 0

function loadDoc(filePath: string) {
  const current = ++generation
  return run(async () => {
    if (current !== generation) return false
    raw.value = null
    let body: string | null
    try {
      body = await loadMarkdown(filePath)
    } catch {
      throw new Error(`文档加载失败：docs/${filePath}`)
    }
    if (current !== generation) return false
    if (body === null) throw new Error(`文档文件缺失：docs/${filePath}`)
    raw.value = body
    if (route.hash) {
      await nextTick()
      if (current !== generation) return body.trim().length > 0
      document.getElementById(decodeURIComponent(route.hash.slice(1)))?.scrollIntoView()
    }
    return body.trim().length > 0
  }, { replace: true, fallback: `文档加载失败：docs/${filePath}` })
}

function reload() {
  void loadDoc(props.page.filePath)
}

watch(() => props.page.filePath, (filePath) => {
  void loadDoc(filePath)
}, { immediate: true })

async function onContentClick(event: MouseEvent) {
  const button = (event.target as Element).closest<HTMLButtonElement>('[data-copy]')
  if (!button) return
  const code = button.closest('.docs-code')?.querySelector('code')?.textContent || ''
  try { await navigator.clipboard.writeText(code); toast.success('代码已复制') }
  catch { toast.error('复制失败，请手动选择代码') }
}

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
.docs-md :deep(.docs-anchor) {
  margin-left: 0.35em;
  opacity: 0;
  font-size: 0.8em;
  text-decoration: none;
}
.docs-md :deep(:is(h2, h3, h4):hover .docs-anchor),
.docs-md :deep(.docs-anchor:focus-visible) { opacity: 1; }
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
.docs-md :deep(.docs-code) {
  margin: 1.2rem 0;
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--muted);
  color: var(--foreground);
}
.docs-md :deep(.docs-code-bar) {
  display: flex;
  justify-content: space-between;
  padding: 0.4rem 0.9rem;
  border-bottom: 1px solid var(--border);
  font-size: 0.75rem;
  color: var(--muted-foreground);
}
.docs-md :deep([data-copy]) { cursor: pointer; color: var(--primary); }
.docs-md :deep(pre) {
  margin: 0;
  overflow: auto;
  padding: 16px 18px;
  font-family: var(--font-mono);
  font-size: 13px;
  line-height: 1.65;
}
.docs-md :deep(pre code) {
  background: transparent;
  border: none;
  padding: 0;
  color: inherit;
  font-size: inherit;
}
.docs-md :deep(.docs-table-scroll) { overflow-x: auto; margin: 1.3rem 0; }
.docs-md :deep(table) {
  width: 100%;
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
.docs-md :deep(blockquote[data-callout='tip']),
.docs-md :deep(blockquote[data-callout='提示']) { border-left-color: var(--chart-2); }
.docs-md :deep(blockquote[data-callout='warning']),
.docs-md :deep(blockquote[data-callout='注意']) { border-left-color: var(--chart-4); }
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
