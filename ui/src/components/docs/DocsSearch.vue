<template>
  <Dialog v-model:open="open">
    <button
      type="button"
      class="inline-flex h-9 min-w-0 items-center gap-2 rounded-lg border border-border/80 bg-muted/35 px-2.5 text-sm text-muted-foreground transition-colors hover:border-border hover:bg-muted/60 hover:text-foreground sm:w-56 sm:px-3 lg:w-64"
      :aria-label="t('components.docsSearch.open')"
      @click="showSearch"
    >
      <SearchIcon class="size-4 shrink-0" />
      <span class="hidden truncate sm:inline">{{ t('components.docsSearch.open') }}</span>
      <kbd class="ml-auto hidden rounded border border-border/80 bg-background px-1.5 py-0.5 font-sans text-[10px] leading-3 text-muted-foreground/80 lg:inline">⌘ K</kbd>
    </button>
    <DialogContent :show-close-button="false" class="top-[10vh] translate-y-0! gap-0 overflow-hidden p-0 shadow-2xl sm:max-w-2xl">
      <DialogTitle class="sr-only">{{ t('components.docsSearch.dialogTitle') }}</DialogTitle>
      <DialogDescription class="sr-only">{{ t('components.docsSearch.dialogDescription') }}</DialogDescription>
      <div class="flex h-14 shrink-0 items-center gap-3 border-b border-border/80 px-4">
        <SearchIcon class="size-5 shrink-0 text-muted-foreground" />
        <input
          ref="input"
          v-model="query"
          type="search"
          class="h-full min-w-0 flex-1 bg-transparent text-base outline-none placeholder:text-muted-foreground/70"
          :placeholder="t('components.docsSearch.placeholder')"
          :aria-label="t('components.docsSearch.inputAria')"
          role="combobox"
          aria-autocomplete="list"
          aria-controls="docs-search-results"
          :aria-activedescendant="activeDescendant"
          @keydown="onInputKeydown"
        />
        <kbd class="hidden rounded-md border border-border bg-muted px-1.5 py-0.5 font-sans text-[10px] text-muted-foreground sm:inline">ESC</kbd>
      </div>
      <div id="docs-search-results" class="min-h-40 overflow-y-auto p-2" role="listbox" :aria-label="t('components.docsSearch.resultsAria')">
        <div v-if="!query.trim()" class="flex min-h-36 flex-col items-center justify-center gap-2 px-6 text-center">
          <SearchIcon class="size-6 text-muted-foreground/50" />
          <p class="text-sm text-muted-foreground">{{ t('components.docsSearch.startHint') }}</p>
        </div>
        <p v-else-if="pending" class="flex min-h-36 items-center justify-center text-sm text-muted-foreground">{{ t('components.docsSearch.indexing') }}</p>
        <template v-else>
          <button
            v-for="(item, index) in displayResults"
            :id="`docs-search-result-${index}`"
            :key="resultKey(item, index)"
            :data-search-index="index"
            type="button"
            class="block w-full rounded-lg px-3 py-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
            :class="index === activeIndex ? 'bg-accent/70 text-accent-foreground' : 'hover:bg-muted/70'"
            role="option"
            :aria-selected="index === activeIndex"
            @mouseenter="activeIndex = index"
            @click="navigate(item)"
          >
            <span class="mb-1 flex items-center gap-2 text-[11px] font-medium text-muted-foreground">
              <span>{{ moduleTitle(item) }}</span>
              <span aria-hidden="true">/</span>
              <span class="truncate">{{ item.page.title }}</span>
            </span>
            <span class="block truncate text-sm font-medium text-foreground">{{ item.heading || item.page.title }}</span>
            <span class="mt-1 block truncate text-xs text-muted-foreground">{{ item.snippet }}</span>
          </button>
          <div v-if="!displayResults.length" class="flex min-h-36 items-center justify-center text-sm text-muted-foreground">{{ t('components.docsSearch.noMatch') }}</div>
        </template>
      </div>
      <div class="flex h-9 shrink-0 items-center gap-4 border-t border-border/70 bg-muted/25 px-4 text-[11px] text-muted-foreground">
        <span><kbd class="mr-1 font-sans">↑↓</kbd>{{ t('components.docsSearch.footerNavigate') }}</span>
        <span><kbd class="mr-1 font-sans">↵</kbd>{{ t('components.docsSearch.footerSelect') }}</span>
        <span class="ml-auto hidden sm:inline"><kbd class="mr-1 font-sans">esc</kbd>{{ t('components.docsSearch.footerClose') }}</span>
      </div>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { SearchIcon } from '@lucide/vue'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { getModule, searchDocs, type DocSearchResult } from '../../docs/catalog'

const { t } = useI18n()
const router = useRouter()
const open = ref(false)
const input = ref<HTMLInputElement | null>(null)
const query = ref('')
const pending = ref(false)
const results = ref<DocSearchResult[]>([])
const activeIndex = ref(0)
let generation = 0

const displayResults = computed(() => {
  const seen = new Set<string>()
  return results.value.filter((item) => {
    const key = `${item.page.filePath}#${item.hash || ''}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  }).slice(0, 12)
})
const activeDescendant = computed(() => displayResults.value.length ? `docs-search-result-${activeIndex.value}` : undefined)

function showSearch() {
  open.value = true
}

function onKeydown(event: KeyboardEvent) {
  const target = event.target
  const editing = target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement || target instanceof HTMLElement && target.isContentEditable
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k' || event.key === '/' && !editing) {
    event.preventDefault()
    showSearch()
  }
}

watch(open, async (value) => {
  if (value) {
    await nextTick()
    input.value?.focus()
    return
  }
  generation++
  query.value = ''
  results.value = []
  pending.value = false
  activeIndex.value = 0
})

watch(query, async (value) => {
  const current = ++generation
  activeIndex.value = 0
  if (!value.trim()) {
    results.value = []
    pending.value = false
    return
  }
  pending.value = true
  const found = await searchDocs(value)
  if (current === generation) {
    results.value = found
    pending.value = false
  }
})

function resultKey(item: DocSearchResult, index: number) {
  return `${item.page.filePath}-${item.hash || index}`
}

function moduleTitle(item: DocSearchResult) {
  return getModule(item.page.moduleId)?.title || item.page.moduleId
}

function navigate(item: DocSearchResult) {
  const { moduleId, slug } = item.page
  void router.push(`/docs/${moduleId}${slug === 'index' ? '' : `/${slug}`}${item.hash ? `#${encodeURIComponent(item.hash)}` : ''}`)
  open.value = false
}

async function onInputKeydown(event: KeyboardEvent) {
  if (!displayResults.value.length) return
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault()
    const delta = event.key === 'ArrowDown' ? 1 : -1
    activeIndex.value = (activeIndex.value + delta + displayResults.value.length) % displayResults.value.length
    await nextTick()
    const active = document.getElementById(activeDescendant.value || '')
    if (active && typeof active.scrollIntoView === 'function') active.scrollIntoView({ block: 'nearest' })
  } else if (event.key === 'Enter') {
    event.preventDefault()
    navigate(displayResults.value[activeIndex.value])
  }
}

onMounted(() => window.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))
</script>
