import { computed, ref, watch, type Ref } from 'vue'

/** 客户端分页：替代 antd Table pagination 配置 */
export function usePagination<T>(source: Ref<T[]>, pageSize = 10) {
  const page = ref(1)
  const total = computed(() => source.value.length)
  const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))
  const items = computed(() => {
    const start = (page.value - 1) * pageSize
    return source.value.slice(start, start + pageSize)
  })

  watch(total, () => {
    if (page.value > pageCount.value) page.value = pageCount.value
  })

  return { page, pageSize, total, pageCount, items }
}
