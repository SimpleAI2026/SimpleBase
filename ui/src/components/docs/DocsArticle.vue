<template>
  <div class="flex min-w-0 w-full flex-1 items-start justify-center gap-8 xl:gap-12">
  <article class="min-w-0 w-full max-w-[820px] flex-1 py-1">
    <div class="mb-8 flex min-h-7 flex-wrap items-center gap-3">
      <Breadcrumb class="min-w-0 flex-1">
        <BreadcrumbList>
          <BreadcrumbItem>
            <BreadcrumbLink as-child>
              <router-link to="/docs" class="text-muted-foreground no-underline transition-colors hover:text-foreground">{{ t('components.docsArticle.breadcrumb') }}</router-link>
            </BreadcrumbLink>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            <BreadcrumbPage>{{ moduleTitle }}</BreadcrumbPage>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>
      <div class="ml-auto flex shrink-0 items-center gap-3">
        <span class="hidden text-xs text-muted-foreground sm:inline" data-docs-progress>{{ t('components.docsArticle.progress', { index: pageIndex, total: pageCount }) }}</span>
        <a
          v-if="githubUrl"
          class="inline-flex shrink-0 items-center gap-1.5 rounded-lg px-2 py-1.5 text-xs text-muted-foreground no-underline transition-colors hover:bg-muted/70 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
          :href="githubUrl"
          target="_blank"
          rel="noopener noreferrer"
        >
          {{ t('components.docsArticle.viewOnGithub') }}
          <ExternalLinkIcon class="size-3.5" />
        </a>
      </div>
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
            <AlertTitle>{{ t('components.docsArticle.noBodyTitle') }}</AlertTitle>
            <AlertDescription>docs/{{ page.filePath }}</AlertDescription>
          </Alert>
        </div>
      </template>
      <div class="docs-md" v-html="html" @click="onContentClick" />
    </SbAsyncRegion>
    <div v-if="prev || next" class="mt-14 grid gap-3 border-t border-border/70 pt-7 sm:grid-cols-2">
      <router-link
        v-if="prev"
        class="group flex min-w-0 flex-col gap-1.5 rounded-xl border border-border/70 bg-card/40 px-5 py-4 no-underline transition-colors hover:border-primary/30 hover:bg-primary/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
        :to="linkFor(prev.slug)"
      >
        <span class="text-xs text-muted-foreground">{{ t('components.docsArticle.prev') }}</span>
        <span class="truncate text-sm font-medium text-foreground transition-colors group-hover:text-primary">← {{ prev.title }}</span>
      </router-link>
      <router-link
        v-if="next"
        class="group flex min-w-0 flex-col gap-1.5 rounded-xl border border-border/70 bg-card/40 px-5 py-4 no-underline transition-colors hover:border-primary/30 hover:bg-primary/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/60 sm:col-start-2 sm:items-end sm:text-right"
        :to="linkFor(next.slug)"
      >
        <span class="text-xs text-muted-foreground">{{ t('components.docsArticle.next') }}</span>
        <span class="truncate text-sm font-medium text-foreground transition-colors group-hover:text-primary">{{ next.title }} →</span>
      </router-link>
    </div>
  </article>
  <DocsToc :headings="rendered.toc" />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
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

const { t } = useI18n()
const route = useRoute()
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
  catch {
    return { html: `<p>${t('components.docsArticle.renderFailed')}</p>`, toc: [] as DocHeading[] }
  }
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
      throw new Error(t('components.docsArticle.loadFailed', { path: filePath }))
    }
    if (current !== generation) return false
    if (body === null) throw new Error(t('components.docsArticle.fileMissing', { path: filePath }))
    raw.value = body
    if (route.hash) {
      await nextTick()
      if (current !== generation) return body.trim().length > 0
      document.getElementById(decodeURIComponent(route.hash.slice(1)))?.scrollIntoView()
    }
    return body.trim().length > 0
  }, { replace: true, fallback: t('components.docsArticle.loadFailed', { path: filePath }) })
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
  try {
    await navigator.clipboard.writeText(code)
    toast.success(t('components.docsArticle.codeCopied'))
  } catch {
    toast.error(t('components.docsArticle.copyFailed'))
  }
}

function linkFor(slug: string) {
  if (slug === 'index') return `/docs/${props.moduleId}`
  return `/docs/${props.moduleId}/${slug}`
}
</script>

<style scoped>
.docs-md {
  color: var(--foreground);
  font-size: 16px;
  line-height: 1.8;
  overflow-wrap: break-word;
}
.docs-md :deep(> :first-child) {
  margin-top: 0;
}
.docs-md :deep(h1),
.docs-md :deep(h2),
.docs-md :deep(h3),
.docs-md :deep(h4) {
  font-family: var(--font-sans);
  font-weight: 700;
  line-height: 1.3;
  letter-spacing: -0.025em;
  color: var(--foreground);
  scroll-margin-top: calc(var(--header-height) + var(--docs-tabs-h, 3.5rem) + 1rem);
}
.docs-md :deep(h1) {
  font-size: clamp(1.85rem, 4vw, 2.35rem);
  margin: 0 0 1.75rem;
  padding-bottom: 1.25rem;
  border-bottom: 1px solid var(--border);
}
.docs-md :deep(h2) {
  font-size: 1.45rem;
  margin: 3rem 0 1rem;
  padding-bottom: 0.65rem;
  border-bottom: 1px solid color-mix(in srgb, var(--border) 75%, transparent);
}
.docs-md :deep(h3) {
  font-size: 1.16rem;
  margin: 2rem 0 0.75rem;
  letter-spacing: -0.015em;
}
.docs-md :deep(h4) {
  font-size: 1rem;
  margin: 1.5rem 0 0.5rem;
  letter-spacing: -0.01em;
}
.docs-md :deep(.docs-anchor) {
  margin-left: 0.45em;
  opacity: 0;
  font-size: 0.75em;
  font-weight: 500;
  text-decoration: none;
  transition: opacity 0.15s;
}
.docs-md :deep(:is(h2, h3, h4):hover .docs-anchor),
.docs-md :deep(.docs-anchor:focus-visible) { opacity: 1; }
.docs-md :deep(p),
.docs-md :deep(ul),
.docs-md :deep(ol) {
  margin: 1rem 0;
  color: color-mix(in srgb, var(--foreground) 88%, transparent);
}
.docs-md :deep(ul),
.docs-md :deep(ol) {
  padding-left: 1.5rem;
}
.docs-md :deep(ul) { list-style: disc; }
.docs-md :deep(ol) { list-style: decimal; }
.docs-md :deep(li) {
  margin: 0.4rem 0;
  padding-left: 0.15rem;
}
.docs-md :deep(li > ul),
.docs-md :deep(li > ol) { margin: 0.3rem 0; }
.docs-md :deep(li::marker) {
  color: color-mix(in srgb, var(--primary) 75%, var(--muted-foreground));
}
.docs-md :deep(a) {
  color: var(--primary);
  font-weight: 500;
  text-decoration: underline;
  text-decoration-color: color-mix(in srgb, var(--primary) 35%, transparent);
  text-underline-offset: 3px;
  transition: text-decoration-color 0.15s;
}
.docs-md :deep(a:hover) { text-decoration-color: var(--primary); }
.docs-md :deep(strong) {
  font-weight: 650;
  color: var(--foreground);
}
.docs-md :deep(:not(pre) > code) {
  font-family: var(--font-mono);
  font-size: 0.85em;
  color: color-mix(in srgb, var(--primary) 82%, var(--foreground));
  background: color-mix(in srgb, var(--primary) 7%, transparent);
  padding: 0.16em 0.4em;
  border: 1px solid color-mix(in srgb, var(--primary) 9%, transparent);
  border-radius: 0.35rem;
}
.docs-md :deep(.docs-code) {
  margin: 1.5rem 0;
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: color-mix(in srgb, var(--muted) 65%, var(--background));
  box-shadow: 0 1px 2px color-mix(in srgb, var(--foreground) 4%, transparent);
  color: var(--foreground);
}
.docs-md :deep(.docs-code-bar) {
  display: flex;
  min-height: 2.5rem;
  align-items: center;
  justify-content: space-between;
  padding: 0.35rem 0.65rem 0.35rem 1rem;
  border-bottom: 1px solid var(--border);
  background: color-mix(in srgb, var(--muted) 55%, transparent);
  font-size: 0.7rem;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--muted-foreground);
}
.docs-md :deep([data-copy]) {
  cursor: pointer;
  border-radius: 0.4rem;
  padding: 0.3rem 0.55rem;
  font-size: 0.75rem;
  font-weight: 500;
  letter-spacing: normal;
  text-transform: none;
  color: var(--muted-foreground);
  transition: background-color 0.15s, color 0.15s;
}
.docs-md :deep([data-copy]:hover) {
  background: var(--muted);
  color: var(--foreground);
}
.docs-md :deep(pre) {
  margin: 0;
  overflow: auto;
  padding: 1.1rem 1.25rem;
  font-family: var(--font-mono);
  font-size: 13px;
  line-height: 1.7;
  tab-size: 2;
}
.docs-md :deep(pre code) {
  background: transparent;
  border: none;
  padding: 0;
  color: inherit;
  font-size: inherit;
}
.docs-md :deep(.docs-table-scroll) {
  overflow-x: auto;
  margin: 1.5rem 0;
  border: 1px solid var(--border);
  border-radius: 12px;
}
.docs-md :deep(table) {
  width: 100%;
  border-collapse: separate;
  border-spacing: 0;
  margin: 0;
  font-size: 0.875rem;
}
.docs-md :deep(th),
.docs-md :deep(td) {
  padding: 0.75rem 0.9rem;
  text-align: left;
  vertical-align: top;
  border-bottom: 1px solid var(--border);
}
.docs-md :deep(th + th),
.docs-md :deep(td + td) { border-left: 1px solid var(--border); }
.docs-md :deep(tbody tr:last-child td) { border-bottom: none; }
.docs-md :deep(th) {
  background: color-mix(in srgb, var(--muted) 65%, transparent);
  font-weight: 600;
  white-space: nowrap;
}
.docs-md :deep(tbody tr:hover) {
  background: color-mix(in srgb, var(--muted) 35%, transparent);
}
.docs-md :deep(blockquote) {
  margin: 1.5rem 0;
  padding: 0.85rem 1.1rem;
  border-left: 3px solid var(--primary);
  border-radius: 0 10px 10px 0;
  background: color-mix(in srgb, var(--primary) 5%, transparent);
  color: var(--muted-foreground);
}
.docs-md :deep(blockquote[data-callout='tip']),
.docs-md :deep(blockquote[data-callout='提示']) {
  border-left-color: var(--success);
  background: color-mix(in srgb, var(--success) 6%, transparent);
}
.docs-md :deep(blockquote[data-callout='warning']),
.docs-md :deep(blockquote[data-callout='注意']) {
  border-left-color: var(--warning);
  background: color-mix(in srgb, var(--warning) 6%, transparent);
}
.docs-md :deep(blockquote p) {
  margin: 0.25rem 0;
  color: inherit;
}
.docs-md :deep(hr) {
  border: none;
  border-top: 1px solid var(--border);
  margin: 2.5rem 0;
}
.docs-md :deep(img) {
  display: block;
  max-width: 100%;
  height: auto;
  margin: 1.5rem auto;
  border-radius: 12px;
  border: 1px solid var(--border);
}
@media (max-width: 640px) {
  .docs-md { font-size: 15px; }
  .docs-md :deep(h2) { margin-top: 2.25rem; }
  .docs-md :deep(pre) { padding: 1rem; }
}
</style>
