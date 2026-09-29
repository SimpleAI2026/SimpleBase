import { toast } from 'vue-sonner'
import { describe, expect, it, vi } from 'vitest'
import { useAsyncAction } from './useAsyncAction'

describe('useAsyncAction', () => {
  it('stores success data and toasts', async () => {
    const onSuccess = vi.fn()
    const action = useAsyncAction(async (n: unknown) => Number(n) * 2, {
      successMsg: 'ok',
      onSuccess
    })
    const result = await action.run(3)
    expect(result).toBe(6)
    expect(action.data.value).toBe(6)
    expect(action.loading.value).toBe(false)
    expect(onSuccess).toHaveBeenCalledWith(6)
    expect(toast.success).toHaveBeenCalledWith('ok')
  })

  it('captures Error messages and fallbacks', async () => {
    const failing = useAsyncAction(async () => {
      throw new Error('boom')
    })
    expect(await failing.run()).toBeUndefined()
    expect(failing.error.value).toBe('boom')
    expect(toast.error).toHaveBeenCalledWith('boom')

    const fallback = useAsyncAction(
      async () => {
        throw 'nope'
      },
      { fallbackMsg: '失败了' }
    )
    await fallback.run()
    expect(fallback.error.value).toBe('失败了')

    const withErrorMsg = useAsyncAction(
      async () => {
        throw 1
      },
      { errorMsg: '自定义错误' }
    )
    await withErrorMsg.run()
    expect(withErrorMsg.error.value).toBe('自定义错误')

    const bare = useAsyncAction(async () => {
      throw { nope: true }
    })
    await bare.run()
    expect(bare.error.value).toBe('操作失败')
  })
})
