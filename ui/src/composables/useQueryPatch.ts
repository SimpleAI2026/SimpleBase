import type { Router } from 'vue-router'

/**
 * 合并更新 URL query：值为 undefined 表示删除该键。
 *
 * 排队串行：每次替换都基于上一次替换后的结果（`router.replace` 是异步的，
 * 并发写会各自读到旧 query 而互相覆盖）；失败静默。
 */
export function useQueryPatch(router: Router) {
  let chain: Promise<unknown> = Promise.resolve()

  return function replaceQuery(patch: Record<string, string | undefined>): void {
    if (!router?.replace) return
    chain = chain
      .then(() => {
        const q: Record<string, unknown> = { ...router.currentRoute.value.query }
        for (const [k, v] of Object.entries(patch)) {
          if (v === undefined) delete q[k]
          else q[k] = v
        }
        return router.replace({ query: q as never })
      })
      .catch(() => undefined)
  }
}
