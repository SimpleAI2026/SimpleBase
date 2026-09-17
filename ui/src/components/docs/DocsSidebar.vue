<template>
  <nav class="docs-sidebar">
    <div class="docs-sidebar-title">{{ moduleTitle }}</div>
    <ul>
      <li v-for="p in pages" :key="p.slug">
        <router-link
          class="docs-side-link"
          :class="{ active: p.slug === activeSlug }"
          :to="pageTo(p.slug)"
        >
          {{ p.title }}
        </router-link>
      </li>
    </ul>
  </nav>
</template>

<script setup lang="ts">
import type { DocPage } from '../../docs/catalog'

const props = defineProps<{
  moduleId: string
  moduleTitle: string
  pages: DocPage[]
  activeSlug: string
}>()

function pageTo(slug: string) {
  if (slug === 'index') return `/docs/${props.moduleId}`
  return `/docs/${props.moduleId}/${slug}`
}
</script>

<style scoped>
.docs-sidebar {
  width: 240px;
  flex-shrink: 0;
  padding: 16px 12px 24px 0;
  border-right: 1px solid var(--sb-border, #e8e6e0);
  min-height: 360px;
}
.docs-sidebar-title {
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--sb-text-muted, #9ca3af);
  padding: 0 10px 10px;
}
ul {
  list-style: none;
  margin: 0;
  padding: 0;
}
.docs-side-link {
  display: block;
  padding: 7px 10px;
  border-radius: 6px;
  color: var(--sb-text, #1f2937);
  text-decoration: none;
  font-size: 14px;
  border-left: 3px solid transparent;
}
.docs-side-link:hover {
  background: rgba(0, 0, 0, 0.04);
}
.docs-side-link.active {
  background: rgba(217, 119, 87, 0.1);
  color: var(--sb-primary, #d97757);
  border-left-color: var(--sb-primary, #d97757);
  font-weight: 600;
}

@media (max-width: 768px) {
  .docs-sidebar {
    width: 100%;
    border-right: none;
    border-bottom: 1px solid var(--sb-border, #e8e6e0);
    min-height: 0;
    padding-right: 0;
  }
}
</style>
