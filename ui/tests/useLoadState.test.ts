import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { toast } from 'vue-sonner'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { LOAD_PLACEHOLDER_DELAY_MS, useLoadState } from '@/composables/useLoadState'

describe('useLoadState', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('stays quiet before the placeholder delay and shows a skeleton after it', async () => {
    const state = useLoadState()
    let finish: (ok: boolean) => void = () => undefined
    const pending = state.run(
      () =>
        new Promise<boolean>((resolve) => {
          finish = resolve
        })
    )
    expect(state.pending.value).toBe(true)
    expect(state.showSkeleton.value).toBe(false)
    expect(state.showEmpty.value).toBe(false)
    expect(state.showError.value).toBe(false)

    await vi.advanceTimersByTimeAsync(LOAD_PLACEHOLDER_DELAY_MS - 1)
    expect(state.showSkeleton.value).toBe(false)

    await vi.advanceTimersByTimeAsync(1)
    expect(state.showSkeleton.value).toBe(true)
    expect(state.showEmpty.value).toBe(false)

    finish(false)
    await pending
    expect(state.pending.value).toBe(false)
    expect(state.showSkeleton.value).toBe(false)
    expect(state.showEmpty.value).toBe(true)
    expect(state.hasData.value).toBe(false)
  })

  it('never shows a skeleton when the request finishes before the delay', async () => {
    const state = useLoadState()
    await state.run(async () => false)
    expect(state.showSkeleton.value).toBe(false)
    expect(state.showEmpty.value).toBe(true)
    expect(state.settled.value).toBe(true)
  })

  it('marks non-empty results and keeps rows while refreshing', async () => {
    const state = useLoadState({ fallback: '加载失败' })
    await state.run(async () => true)
    expect(state.hasData.value).toBe(true)
    expect(state.showEmpty.value).toBe(false)

    let finish: (ok: boolean) => void = () => undefined
    const again = state.run(
      () =>
        new Promise<boolean>((resolve) => {
          finish = resolve
        })
    )
    expect(state.refreshing.value).toBe(true)
    expect(state.showSkeleton.value).toBe(false)
    expect(state.hasData.value).toBe(true)

    await vi.advanceTimersByTimeAsync(LOAD_PLACEHOLDER_DELAY_MS)
    expect(state.showSkeleton.value).toBe(false)

    finish(true)
    await again
    expect(state.refreshing.value).toBe(false)
    expect(state.hasData.value).toBe(true)
  })

  it('shows an error without an empty state on the first failure', async () => {
    const state = useLoadState({ fallback: '加载失败' })
    await state.run(async () => {
      throw new Error('boom')
    })
    expect(state.showError.value).toBe(true)
    expect(state.showEmpty.value).toBe(false)
    expect(state.error.value).toBe('boom')
    expect(toast.error).toHaveBeenCalledWith('boom')
  })

  it('keeps existing rows when a refresh fails and toasts the fallback', async () => {
    const state = useLoadState({ fallback: '加载失败' })
    await state.run(async () => true)
    await state.run(async () => {
      throw 'nope'
    })
    expect(state.hasData.value).toBe(true)
    expect(state.showError.value).toBe(false)
    expect(state.showEmpty.value).toBe(false)
    expect(toast.error).toHaveBeenCalledWith('加载失败')
  })

  it('prefers the per-run fallback and drops rows when replace is set', async () => {
    const state = useLoadState({ fallback: '默认' })
    await state.run(async () => true)

    let finish: (ok: boolean) => void = () => undefined
    const replaced = state.run(
      () =>
        new Promise<boolean>((resolve) => {
          finish = resolve
        }),
      { replace: true }
    )
    expect(state.hasData.value).toBe(false)
    expect(state.refreshing.value).toBe(false)
    expect(state.pending.value).toBe(true)
    finish(true)
    await replaced
    expect(state.hasData.value).toBe(true)

    await state.run(
      async () => {
        throw 1
      },
      { replace: true, fallback: '这次失败' }
    )
    expect(state.hasData.value).toBe(false)
    expect(state.showError.value).toBe(true)
    expect(state.error.value).toBe('这次失败')
    expect(toast.error).toHaveBeenCalledWith('这次失败')
  })

  it('settle and reset move between empty, data, and idle', () => {
    const state = useLoadState()
    state.settle(false)
    expect(state.settled.value).toBe(true)
    expect(state.showEmpty.value).toBe(true)
    expect(state.pending.value).toBe(false)

    state.settle(true)
    expect(state.hasData.value).toBe(true)
    expect(state.showEmpty.value).toBe(false)

    state.reset()
    expect(state.settled.value).toBe(false)
    expect(state.hasData.value).toBe(false)
    expect(state.showEmpty.value).toBe(false)
    expect(state.showError.value).toBe(false)
  })

  it('ignores a stale run that finishes after a newer one', async () => {
    const state = useLoadState()
    let finishFirst: (ok: boolean) => void = () => undefined
    const first = state.run(
      () =>
        new Promise<boolean>((resolve) => {
          finishFirst = resolve
        })
    )
    await state.run(async () => false)
    finishFirst(true)
    await first
    expect(state.hasData.value).toBe(false)
    expect(state.showEmpty.value).toBe(true)
  })

  it('ignores a stale rejection and does not clear the newer timer', async () => {
    const state = useLoadState()
    let failFirst: (reason: unknown) => void = () => undefined
    const first = state.run(
      () =>
        new Promise<boolean>((_resolve, reject) => {
          failFirst = reject
        })
    )
    let finishSecond: (ok: boolean) => void = () => undefined
    const second = state.run(
      () =>
        new Promise<boolean>((resolve) => {
          finishSecond = resolve
        })
    )
    failFirst(new Error('stale'))
    await first
    expect(state.pending.value).toBe(true)
    expect(toast.error).not.toHaveBeenCalledWith('stale')

    await vi.advanceTimersByTimeAsync(LOAD_PLACEHOLDER_DELAY_MS)
    expect(state.showSkeleton.value).toBe(true)
    finishSecond(true)
    await second
    expect(state.hasData.value).toBe(true)
  })

  it('clears the placeholder timer when the scope is disposed', async () => {
    let showSkeleton: { value: boolean } | undefined
    const Comp = defineComponent({
      setup() {
        const state = useLoadState()
        showSkeleton = state.showSkeleton
        void state.run(() => new Promise<boolean>(() => undefined))
        return () => h('div')
      }
    })
    const wrapper = mount(Comp)
    await flushPromises()
    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(LOAD_PLACEHOLDER_DELAY_MS)
    expect(showSkeleton?.value).toBe(false)
  })
})
