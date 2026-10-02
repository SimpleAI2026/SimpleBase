<template>
  <aside
    class="sticky top-[calc(var(--header-height)+var(--docs-tabs-h)+var(--docs-pad-y))] h-[calc(100svh-var(--header-height)-var(--docs-tabs-h)-var(--docs-pad-y)-var(--docs-pad-y))] w-[240px] shrink-0 self-start overflow-hidden border-r border-border/60 pr-3"
  >
    <ScrollArea class="h-full">
      <nav class="flex flex-col gap-0.5 py-1">
        <div class="px-3 pt-1 pb-3 text-xs font-semibold tracking-wider text-muted-foreground uppercase">
          {{ moduleTitle }}
        </div>
        <router-link
          v-for="p in pages"
          :key="p.slug"
          class="relative rounded-md px-3 py-2 text-sm leading-5 text-foreground/70 no-underline transition-colors hover:bg-muted/70 hover:text-foreground"
          :class="p.slug === activeSlug ? 'bg-primary/8 font-medium text-primary before:absolute before:inset-y-1.5 before:left-0 before:w-0.5 before:rounded-full before:bg-primary hover:bg-primary/8 hover:text-primary' : ''"
          :to="pageTo(p.slug)"
        >
          {{ p.title }}
        </router-link>
      </nav>
    </ScrollArea>
  </aside>
</template>

<script setup lang="ts">
import { ScrollArea } from '@/components/ui/scroll-area'
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
