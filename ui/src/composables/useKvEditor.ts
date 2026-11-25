import { watch } from 'vue'
import { toast } from 'vue-sonner'
import { api } from '@/services/api'
import { t } from '@/locales'
import type { KvExecBody } from '@/services/types'
import type { AsyncRegionState } from '@/components/async-region-state'
import { errorMessage } from '@/utils/format'
import { useLoadState } from '@/composables/useLoadState'

export interface UseKvEditorOptions<T> {
  projectId: () => string
  kvKey: () => string
  /** 加载失败的提示文案 key。 */
  fallback: string
  /** 读取当前值的命令（随 kvKey 等响应式输入变化，所以是函数）。 */
  read: () => KvExecBody
  /** 把读命令的返回值解析成编辑器的数据结构。 */
  parse: (result: unknown) => T
  /** 把解析结果写进编辑器的本地状态。 */
  apply: (value: T) => void
  /** 写命令成功后通知父组件（key 列表 / TTL 需要刷新）。 */
  changed: () => void
}

export interface KvMutateOptions {
  /** 失败提示的 i18n key。 */
  failKey: string
  /** 成功提示的 i18n key；省略不提示。需要带返回值的提示用 onSuccess。 */
  successKey?: string
  /** 写命令成功后、重新读取之前执行：清空输入、用返回值原地更新等。 */
  onSuccess?: (result: unknown) => void
  /** 成功后是否重新读取；默认 true。用返回值原地更新的编辑器传 false。 */
  reload?: boolean
}

/**
 * KV 类型编辑器（string / hash / list / set / zset）共用的骨架：
 * 加载态、按键（kvKey）自动重载，以及写命令固定的
 * 「失败提示 → 成功处理 → 重新读取 → 通知父组件」顺序。
 * 编辑器只声明读命令、解析方式与状态落点，不再各自手写 try / catch。
 */
export function useKvEditor<T>(opts: UseKvEditorOptions<T>) {
  const loading = useLoadState({ fallback: opts.fallback })

  function load() {
    return loading.run(async () => {
      opts.apply(opts.parse(await api.kv.exec(opts.projectId(), opts.read())))
      return true
    })
  }

  /** 执行一次写命令；永不 reject：失败只提示，成功后按选项提示、重新读取并通知父组件。 */
  async function mutate(body: KvExecBody, o: KvMutateOptions): Promise<void> {
    let result: unknown
    try {
      result = await api.kv.exec(opts.projectId(), body)
    } catch (e) {
      toast.error(errorMessage(e, t(o.failKey)))
      return
    }
    if (o.successKey) toast.success(t(o.successKey))
    o.onSuccess?.(result)
    if (o.reload !== false) await load()
    opts.changed()
  }

  watch(opts.kvKey, () => { void load() }, { immediate: true })

  // 传给 SbAsyncRegion 的 state：编辑器自己画空态，所以 showEmpty 恒为 false。
  const state: AsyncRegionState = {
    pending: loading.pending,
    showSkeleton: loading.showSkeleton,
    showEmpty: false,
    showError: loading.showError,
    refreshing: loading.refreshing,
    error: loading.error
  }
  return { state, load, mutate }
}
