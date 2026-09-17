<template>
  <article class="docs-article">
    <div class="docs-article-bar">
      <a-breadcrumb>
        <a-breadcrumb-item>
          <router-link to="/docs">使用文档</router-link>
        </a-breadcrumb-item>
        <a-breadcrumb-item>{{ moduleTitle }}</a-breadcrumb-item>
        <a-breadcrumb-item>{{ title }}</a-breadcrumb-item>
      </a-breadcrumb>
      <a
        v-if="githubUrl"
        class="docs-github"
        :href="githubUrl"
        target="_blank"
        rel="noopener noreferrer"
      >
        在 GitHub 上查看
      </a>
    </div>
    <div v-if="loadError" class="docs-missing">
      <a-result status="warning" :title="loadError" sub-title="源文件未打进前端产物，页面不会空白。" />
    </div>
    <div v-else-if="!html" class="docs-missing">
      <a-result status="info" title="这篇文档没有正文" :sub-title="`docs/${page.filePath}`" />
    </div>
    <div v-else class="docs-md" v-html="html" />
    <div v-if="prev || next" class="docs-pager">
      <router-link v-if="prev" class="docs-pager-link" :to="linkFor(prev.slug)">
        ← {{ prev.title }}
      </router-link>
      <span v-else />
      <router-link v-if="next" class="docs-pager-link" :to="linkFor(next.slug)">
        {{ next.title }} →
      </router-link>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
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
  return renderMarkdown(raw.value, props.moduleId)
})
const githubUrl = computed(() => githubBlobUrl(props.page.filePath))

function linkFor(slug: string) {
  if (slug === 'index') return `/docs/${props.moduleId}`
  return `/docs/${props.moduleId}/${slug}`
}
</script>

<style scoped>
.docs-article {
  flex: 1;
  min-width: 0;
  padding: 16px 8px 48px 24px;
  max-width: 900px;
}
.docs-article-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}
.docs-github {
  font-size: 13px;
  color: var(--sb-text-secondary, #6b7280);
  text-decoration: none;
}
.docs-github:hover {
  color: var(--sb-primary, #d97757);
}
.docs-missing {
  padding: 24px 0 48px;
}
.docs-pager {
  display: flex;
  justify-content: space-between;
  margin-top: 32px;
  padding-top: 16px;
  border-top: 1px solid var(--sb-border, #e8e6e0);
}
.docs-pager-link {
  color: var(--sb-primary, #d97757);
  text-decoration: none;
  font-size: 14px;
}
.docs-pager-link:hover {
  text-decoration: underline;
}

/* GitHub-wiki-ish prose */
.docs-md :deep(h1) {
  font-size: 1.75rem;
  margin: 0 0 1rem;
  border-bottom: 1px solid var(--sb-border, #e8e6e0);
  padding-bottom: 0.4rem;
}
.docs-md :deep(h2) {
  font-size: 1.35rem;
  margin: 1.6rem 0 0.75rem;
  border-bottom: 1px solid var(--sb-border, #eee);
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
  color: var(--sb-text, #1f2937);
}
.docs-md :deep(a) {
  color: var(--sb-primary, #d97757);
}
.docs-md :deep(code) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.9em;
  background: rgba(0, 0, 0, 0.05);
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
  border: 1px solid var(--sb-border, #e8e6e0);
  padding: 8px 10px;
  text-align: left;
}
.docs-md :deep(blockquote) {
  margin: 1rem 0;
  padding: 0.25rem 0 0.25rem 1rem;
  border-left: 4px solid var(--sb-border, #ddd);
  color: var(--sb-text-secondary, #6b7280);
}

@media (max-width: 768px) {
  .docs-article {
    padding: 12px 0 32px;
  }
}
</style>
