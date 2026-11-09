<template>
  <aside v-if="headings.length >= 3" :aria-label="t('components.docsToc.aria')" class="hidden w-52 shrink-0 lg:block">
    <nav class="sticky top-[calc(var(--header-height)+var(--docs-tabs-h)+var(--docs-pad-y))] max-h-[calc(100svh-var(--header-height)-var(--docs-tabs-h)-var(--docs-pad-y)-var(--docs-pad-y))] overflow-y-auto border-l border-border/70 pl-4 text-xs">
      <p class="mb-3 text-[11px] font-semibold tracking-[0.08em] text-muted-foreground/80">{{ t('components.docsToc.title') }}</p>
      <a v-for="heading in headings" :key="heading.id" :href="`#${heading.id}`"
        class="-ml-px mb-1 block truncate rounded-r-md border-l-2 py-1.5 leading-4 text-muted-foreground transition-colors hover:bg-muted/50 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
        :class="[heading.level === 3 ? 'pl-5' : 'pl-3', active === heading.id ? 'border-primary bg-primary/5 font-medium text-primary' : 'border-transparent']"
        :aria-current="active === heading.id ? 'location' : undefined"
        :title="heading.text" @click="active = heading.id">{{ heading.text }}</a>
    </nav>
  </aside>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import type { DocHeading } from '../../docs/render'

const props = defineProps<{ headings: DocHeading[] }>()
const { t } = useI18n()
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
