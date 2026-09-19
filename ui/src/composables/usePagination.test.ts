import { ref } from 'vue'
import { describe, expect, it } from 'vitest'
import { usePagination } from './usePagination'

describe('usePagination', () => {
  it('slices items and clamps the page when the source shrinks', async () => {
    const source = ref([1, 2, 3, 4, 5])
    const pager = usePagination(source, 2)
    expect(pager.total.value).toBe(5)
    expect(pager.pageCount.value).toBe(3)
    expect(pager.items.value).toEqual([1, 2])
    pager.page.value = 3
    expect(pager.items.value).toEqual([5])
    source.value = [1]
    await Promise.resolve()
    expect(pager.page.value).toBe(1)
    expect(pager.items.value).toEqual([1])
  })
})
