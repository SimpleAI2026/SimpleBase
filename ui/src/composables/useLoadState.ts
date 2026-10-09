import { computed, getCurrentScope, onScopeDispose, ref, type ComputedRef, type Ref } from 'vue'
import { toast } from 'vue-sonner'
import { errorMessage } from '@/utils/format'

/** 占位出现延迟。快请求在此之前结束则不闪骨架；消失不延迟。 */
export const LOAD_PLACEHOLDER_DELAY_MS = 160

export interface UseLoadStateOptions {
  /** 非 Error 拒绝时的文案。 */
  fallback?: string
}

export interface LoadRunOptions {
  /**
   * 切换到另一条记录时丢掉旧内容，走首次加载占位。
   * 默认保留已有数据，只标记为刷新。
   */
  replace?: boolean
  fallback?: string
}

export interface LoadState {
  pending: Ref<boolean>
  settled: Ref<boolean>
  error: Ref<string>
  hasData: Ref<boolean>
  /** 首次加载且已过延迟，应显示骨架。 */
  showSkeleton: ComputedRef<boolean>
  /** 请求已成功结束且没有可展示数据。 */
  showEmpty: ComputedRef<boolean>
  /** 首次失败且没有可留的数据。 */
  showError: ComputedRef<boolean>
  /** 已有数据时再次请求。 */
  refreshing: ComputedRef<boolean>
  /**
   * task 返回这次是否有可展示数据，并自行写入列表。
   * 失败时 toast；没有旧数据时同时进入 showError。
   */
  run: (task: () => Promise<boolean>, options?: LoadRunOptions) => Promise<void>
  /** 本地直接进入空或有数据，例如新建的空白会话。 */
  settle: (ok: boolean) => void
  /** 回到未请求，用于 SQL 模式切换。 */
  reset: () => void
}

export function useLoadState(options: UseLoadStateOptions = {}): LoadState {
  const pending = ref(false)
  const settled = ref(false)
  const error = ref('')
  const hasData = ref(false)
  const delayed = ref(false)
  let generation = 0
  let timer: ReturnType<typeof setTimeout> | null = null

  function clearTimer() {
    if (timer != null) {
      clearTimeout(timer)
      timer = null
    }
  }

  const showSkeleton = computed(() => pending.value && !hasData.value && delayed.value)
  const showEmpty = computed(
    () => settled.value && !pending.value && !error.value && !hasData.value
  )
  const showError = computed(
    () => settled.value && !pending.value && error.value.length > 0 && !hasData.value
  )
  const refreshing = computed(() => pending.value && hasData.value)

  function settle(ok: boolean) {
    generation += 1
    clearTimer()
    delayed.value = false
    pending.value = false
    settled.value = true
    error.value = ''
    hasData.value = ok
  }

  function reset() {
    generation += 1
    clearTimer()
    delayed.value = false
    pending.value = false
    settled.value = false
    error.value = ''
    hasData.value = false
  }

  async function run(task: () => Promise<boolean>, runOptions: LoadRunOptions = {}) {
    const gen = ++generation
    const keep = hasData.value && !runOptions.replace
    if (runOptions.replace) hasData.value = false
    pending.value = true
    error.value = ''
    delayed.value = false
    clearTimer()
    timer = setTimeout(() => {
      if (gen === generation) delayed.value = true
    }, LOAD_PLACEHOLDER_DELAY_MS)
    try {
      const ok = await task()
      if (gen !== generation) return
      hasData.value = ok
      error.value = ''
    } catch (e) {
      if (gen !== generation) return
      const msg = errorMessage(e, runOptions.fallback || options.fallback || '加载失败')
      toast.error(msg)
      if (keep) {
        error.value = ''
      } else {
        hasData.value = false
        error.value = msg
      }
    } finally {
      if (gen !== generation) return
      clearTimer()
      delayed.value = false
      pending.value = false
      settled.value = true
    }
  }

  if (getCurrentScope()) onScopeDispose(clearTimer)

  return {
    pending,
    settled,
    error,
    hasData,
    showSkeleton,
    showEmpty,
    showError,
    refreshing,
    run,
    settle,
    reset
  }
}
