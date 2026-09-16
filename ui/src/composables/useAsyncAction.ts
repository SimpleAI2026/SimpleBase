import { ref, type Ref } from 'vue'
import { message } from 'ant-design-vue'

interface AsyncActionOptions {
  /** 成功提示；缺省不提示 */
  successMsg?: string
  /** 失败提示；缺省取 error.message，再缺省 fallback */
  errorMsg?: string
  /** 失败兜底文案 */
  fallbackMsg?: string
  /** 成功后是否自动刷新（返回值传给 data） */
  onSuccess?: (data: unknown) => void
}

export interface AsyncAction<T> {
  run: (...args: unknown[]) => Promise<T | undefined>
  loading: Ref<boolean>
  data: Ref<T | null>
  error: Ref<string>
}

/**
 * 统一异步模式：loading + try/catch + message 提示。
 * 替代各页手写的 try/catch/message 模板（原 20+ 处重复）。
 *
 * 用法：
 *   const { run: load, loading } = useAsyncAction(api.db.rows, { fallbackMsg: '数据加载失败' })
 *   onMounted(() => load())
 */
export function useAsyncAction<T>(
  fn: (...args: unknown[]) => Promise<T>,
  opts: AsyncActionOptions = {}
): AsyncAction<T> {
  const loading = ref(false)
  const data = ref<T | null>(null) as Ref<T | null>
  const error = ref('')

  async function run(...args: unknown[]): Promise<T | undefined> {
    loading.value = true
    error.value = ''
    try {
      const result = await fn(...args)
      data.value = result
      if (opts.successMsg) message.success(opts.successMsg)
      opts.onSuccess?.(result)
      return result
    } catch (e) {
      const msg =
        (e instanceof Error && e.message) || opts.errorMsg || opts.fallbackMsg || '操作失败'
      error.value = msg
      message.error(msg)
      return undefined
    } finally {
      loading.value = false
    }
  }

  return { run, loading, data, error }
}
