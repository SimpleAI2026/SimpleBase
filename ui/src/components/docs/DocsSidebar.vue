<template>
  <aside
    class="sticky top-[calc(var(--header-height)+61px)] h-[calc(100svh-var(--header-height)-77px)] w-[248px] shrink-0 self-start overflow-hidden rounded-xl border bg-card shadow-sm"
  >
    <ScrollArea class="h-full">
      <nav class="flex flex-col gap-0.5 p-3">
        <div class="px-2.5 pt-1 pb-2 text-[11.5px] font-semibold tracking-wider text-muted-foreground uppercase">
          {{ moduleTitle }}
        </div>
        <router-link
          v-for="p in pages"
          :key="p.slug"
          class="rounded-lg px-2.5 py-1.5 text-[13.5px] text-foreground/75 no-underline transition-colors hover:bg-accent hover:text-foreground"
          :class="p.slug === activeSlug ? 'bg-primary/10 font-semibold text-primary hover:bg-primary/10 hover:text-primary' : ''"
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
