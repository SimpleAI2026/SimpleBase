import { watch } from 'vue'
import type { WatchSource } from 'vue'

export interface UseProjectReloadOptions {
  /** 切换项目时先于 load 执行：重置页码、关弹窗、清展开态等页面私有状态。 */
  onSwitch?: () => void
}

/**
 * 首次加载 + 切换项目重载。
 *
 * `ProjectScope` 的 `:key` 只重挂插槽 DOM，页面的 `<script setup>` 不会重跑，
 * 所以页面必须自己监听项目变化；这里把「setup 里同步触发一次 load + 项目变了先重置再 load」收成一处。
 */
export function useProjectReload(
  projectId: WatchSource<string>,
  load: () => unknown,
  opts: UseProjectReloadOptions = {}
): void {
  watch(
    projectId,
    () => {
      opts.onSwitch?.()
      void load()
    },
    { immediate: true }
  )
}
