<template>
  <aside
    class="sticky top-[calc(var(--header-height)+var(--docs-tabs-h)+var(--docs-pad-y))] hidden h-[calc(100svh-var(--header-height)-var(--docs-tabs-h)-var(--docs-pad-y)-var(--docs-pad-y))] w-56 shrink-0 self-start overflow-hidden border-r border-border/60 pr-5 md:block"
  >
    <ScrollArea class="h-full">
      <nav class="flex flex-col gap-1 py-1 pr-1">
        <template v-for="p in pages" :key="p.slug">
          <p v-if="p.group && p.group !== previousGroup(p)" class="px-3 pb-1.5 pt-5 text-[11px] font-semibold tracking-[0.08em] text-muted-foreground/80 first:pt-2">{{ p.group }}</p>
          <router-link
            class="relative rounded-lg px-3 py-2 text-sm leading-5 text-muted-foreground no-underline transition-colors hover:bg-muted/70 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
            :class="p.slug === activeSlug ? 'bg-accent/80 font-medium text-accent-foreground before:absolute before:inset-y-2 before:left-0 before:w-0.5 before:rounded-full before:bg-primary hover:bg-accent/80 hover:text-accent-foreground' : ''"
            :aria-current="p.slug === activeSlug ? 'page' : undefined"
            :to="pageTo(p.slug)"
          >{{ p.title }}</router-link>
        </template>
      </nav>
    </ScrollArea>
  </aside>
</template>

<script setup lang="ts">
import { ScrollArea } from '@/components/ui/scroll-area'
import type { DocPage } from '../../docs/catalog'

const props = defineProps<{
  moduleId: string
  pages: DocPage[]
  activeSlug: string
}>()

function previousGroup(page: DocPage): string | undefined {
  const i = props.pages.indexOf(page)
  return i > 0 ? props.pages[i - 1].group : undefined
}

function pageTo(slug: string) {
  if (slug === 'index') return `/docs/${props.moduleId}`
  return `/docs/${props.moduleId}/${slug}`
}
</script>
