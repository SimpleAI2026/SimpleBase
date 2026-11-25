import type { MaybeRef } from 'vue'

/**
 * `useLoadState()` 返回值里 `SbAsyncRegion` 关心的六个字段。
 * 字段可以是 ref，所以宿主可以把整个 `useLoadState()` 结果直接作为 `state` 传给区域组件。
 */
export interface AsyncRegionState {
  pending: MaybeRef<boolean>
  showSkeleton: MaybeRef<boolean>
  showEmpty: MaybeRef<boolean>
  showError: MaybeRef<boolean>
  refreshing: MaybeRef<boolean>
  error: MaybeRef<string>
}
