import { ref, type Ref } from 'vue'
import { toast } from 'vue-sonner'

interface AsyncActionOptions {
  successMsg?: string
  errorMsg?: string
  fallbackMsg?: string
  onSuccess?: (data: unknown) => void
}

export interface AsyncAction<T> {
  run: (...args: unknown[]) => Promise<T | undefined>
  loading: Ref<boolean>
  data: Ref<T | null>
  error: Ref<string>
}

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
      if (opts.successMsg) toast.success(opts.successMsg)
      opts.onSuccess?.(result)
      return result
    } catch (e) {
      const msg =
        (e instanceof Error && e.message) || opts.errorMsg || opts.fallbackMsg || '操作失败'
      error.value = msg
      toast.error(msg)
      return undefined
    } finally {
      loading.value = false
    }
  }

  return { run, loading, data, error }
}
