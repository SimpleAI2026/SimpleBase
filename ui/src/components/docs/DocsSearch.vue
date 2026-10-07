<template>
  <div class="relative" @keydown.esc="open = false">
    <button type="button" class="rounded-md border border-border px-3 py-1.5 text-sm text-muted-foreground hover:text-foreground" aria-label="搜索文档" @click="showSearch">搜索文档 <kbd class="ml-2 text-xs">⌘ K</kbd></button>
    <div v-if="open" class="absolute right-0 z-30 mt-2 w-[min(90vw,28rem)] rounded-xl border border-border bg-background p-3 shadow-xl">
      <input ref="input" v-model="query" type="search" class="w-full rounded-md border border-border bg-background p-2 text-sm outline-primary" placeholder="搜索标题、内容或错误码" aria-label="输入文档搜索词" @keydown.enter="goFirst" />
      <p v-if="pending" class="p-2 text-sm text-muted-foreground">正在索引文档…</p>
      <div v-else-if="query.trim()" class="max-h-80 overflow-y-auto" role="listbox" aria-label="搜索结果">
        <button v-for="(item, index) in results" :key="`${item.page.filePath}-${item.hash || index}`" type="button" class="block w-full rounded-md px-2 py-2 text-left hover:bg-muted" role="option" :aria-selected="index === 0" @click="navigate(item)">
          <span class="block text-sm font-medium">{{ item.page.title }}{{ item.heading ? ` · ${item.heading}` : '' }}</span>
          <span class="block truncate text-xs text-muted-foreground">{{ item.snippet }}</span>
        </button>
        <p v-if="!results.length" class="p-2 text-sm text-muted-foreground">没有匹配的文档</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { searchDocs, type DocSearchResult } from '../../docs/catalog'

const router = useRouter()
const open = ref(false)
const input = ref<HTMLInputElement | null>(null)
const query = ref('')
const pending = ref(false)
const results = ref<DocSearchResult[]>([])
let generation = 0
async function showSearch() {
  open.value = true
  await nextTick()
  input.value?.focus()
}
function onKeydown(event: KeyboardEvent) {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k' || event.key === '/' && !(event.target instanceof HTMLInputElement || event.target instanceof HTMLTextAreaElement)) {
    event.preventDefault()
    void showSearch()
  }
}
watch(query, async (value) => {
  const current = ++generation
  if (!value.trim()) { results.value = []; return }
  pending.value = true
  const found = await searchDocs(value)
  if (current === generation) { results.value = found; pending.value = false }
})
function navigate(item: DocSearchResult) {
  const { moduleId, slug } = item.page
  void router.push(`/docs/${moduleId}${slug === 'index' ? '' : `/${slug}`}${item.hash ? `#${encodeURIComponent(item.hash)}` : ''}`)
  open.value = false
}
function goFirst() { if (results.value[0]) navigate(results.value[0]) }
onMounted(() => window.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))
</script>
