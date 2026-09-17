<template>
  <ScrollArea class="h-[calc(100vh-120px)] w-[260px] shrink-0 border-r bg-muted/40">
    <nav class="flex flex-col gap-1 px-3 py-4">
      <div class="px-2.5 pb-2.5 text-[12px] font-semibold tracking-wide text-muted-foreground uppercase">
        {{ moduleTitle }}
      </div>
      <router-link
        v-for="p in pages"
        :key="p.slug"
        class="rounded-md border-l-[3px] border-transparent px-2.5 py-1.5 text-sm text-foreground no-underline hover:bg-muted"
        :class="p.slug === activeSlug ? 'border-l-primary bg-primary/10 font-semibold text-primary' : ''"
        :to="pageTo(p.slug)"
      >
        {{ p.title }}
      </router-link>
    </nav>
  </ScrollArea>
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
