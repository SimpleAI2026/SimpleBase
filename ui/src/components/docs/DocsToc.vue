<template>
  <aside v-if="headings.length >= 3" aria-label="本页目录" class="hidden w-48 shrink-0 lg:block">
    <nav class="sticky top-[calc(var(--header-height)+var(--docs-tabs-h)+2rem)] max-h-[calc(100svh-var(--header-height)-var(--docs-tabs-h)-4rem)] overflow-y-auto border-l border-border/70 pl-3 text-xs">
      <p class="mb-3 font-semibold text-muted-foreground">本页目录</p>
      <a v-for="heading in headings" :key="heading.id" :href="`#${heading.id}`"
        class="mb-2 block truncate border-l-2 py-1 text-muted-foreground hover:text-foreground"
        :class="[heading.level === 3 ? 'pl-4' : 'pl-2', active === heading.id ? 'border-primary font-semibold text-primary' : 'border-transparent']"
        :title="heading.text" @click="active = heading.id">{{ heading.text }}</a>
    </nav>
  </aside>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch, nextTick } from 'vue'
import type { DocHeading } from '../../docs/render'

const props = defineProps<{ headings: DocHeading[] }>()
const active = ref('')
let observer: IntersectionObserver | null = null
watch(() => props.headings, async (headings) => {
  observer?.disconnect()
  observer = null
  active.value = headings[0]?.id || ''
  if (headings.length < 3 || typeof IntersectionObserver === 'undefined') return
  await nextTick()
  observer = new IntersectionObserver((entries) => {
    for (const entry of entries) {
      if (entry.isIntersecting) active.value = entry.target.id
    }
  }, { rootMargin: '-25% 0px -65% 0px' })
  for (const heading of headings) {
    const element = document.getElementById(heading.id)
    if (element) observer.observe(element)
  }
}, { immediate: true })
onBeforeUnmount(() => observer?.disconnect())
</script>
