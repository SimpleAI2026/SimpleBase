import { describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { useProjectReload } from '@/composables/useProjectReload'

describe('useProjectReload', () => {
  it('setup 阶段同步触发一次 load，且 onSwitch 先于 load 执行', () => {
    const calls: string[] = []
    const projectId = ref('p1')
    effectScope().run(() => {
      useProjectReload(projectId, () => calls.push('load'), { onSwitch: () => calls.push('switch') })
    })
    expect(calls).toEqual(['switch', 'load'])
  })

  it('切换项目：先重置页面状态再重载；同一项目不重复触发', async () => {
    const page = ref(3)
    const seen: Array<{ id: string; page: number }> = []
    const projectId = ref('p1')
    effectScope().run(() => {
      useProjectReload(projectId, () => seen.push({ id: projectId.value, page: page.value }), {
        onSwitch: () => { page.value = 1 }
      })
    })
    expect(seen).toEqual([{ id: 'p1', page: 1 }])
    projectId.value = 'p2'
    await nextTick()
    expect(seen.at(-1)).toEqual({ id: 'p2', page: 1 })
    projectId.value = 'p2'
    await nextTick()
    expect(seen).toHaveLength(2)
  })

  it('支持 getter 作为来源；不要求 onSwitch；load 返回的 Promise 失败不会变成未处理拒绝', async () => {
    const projectId = ref('a')
    const load = vi.fn(() => Promise.resolve())
    const scope = effectScope()
    scope.run(() => useProjectReload(() => projectId.value, load))
    projectId.value = 'b'
    await nextTick()
    expect(load).toHaveBeenCalledTimes(2)
    scope.stop()
    projectId.value = 'c'
    await nextTick()
    expect(load).toHaveBeenCalledTimes(2)
  })
})
