import type { ApiError } from '@/services/types'

/**
 * 「资源尚未建立」类错误码（planv6.0 §2）。
 *
 * 这类 404 的含义是「当前项目/实例上还没有这份存储」，属于**空**而不是**失败**：
 * 界面应当展示空态，而不是红色错误块 + toast。
 *
 * 只有明确登记的码才走空态。其他 404（例如指定 ID 不存在）仍然按错误处理，
 * 否则会把真正的失败吞掉。
 */
export const EMPTY_RESOURCE_CODES = ['kv_store_not_found']

/** 是否为「资源尚未建立」的 404：status 为 404 且 code 在登记表内。 */
export function isEmptyResourceError(e: unknown): boolean {
  if (!e || typeof e !== 'object') return false
  const err = e as ApiError
  if (err.status !== 404) return false
  return typeof err.code === 'string' && EMPTY_RESOURCE_CODES.includes(err.code)
}
