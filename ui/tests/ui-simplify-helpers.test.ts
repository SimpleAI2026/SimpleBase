import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { i18n } from '@/locales'
import SbRefreshButton from '@/components/SbRefreshButton.vue'
import { basename, downloadBlob } from '@/utils/download'
import { copyText } from '@/utils/clipboard'

// planv6.7 ui-simplify-ux-plan：S2 刷新按钮、B8 下载、C10 复制的公共实现。
describe('SbRefreshButton', () => {
  it('默认文案为「刷新」，点击上抛 click', async () => {
    const w = mount(SbRefreshButton, { global: { plugins: [i18n] } })
    expect(w.text()).toBe('刷新')
    expect(w.find('[data-slot="button"]').attributes('disabled')).toBeUndefined()
    await w.get('button').trigger('click')
    expect(w.emitted('click')).toHaveLength(1)
  })

  it('pending 时禁用并显示 Spinner；label 可覆盖', () => {
    const w = mount(SbRefreshButton, { props: { pending: true, label: '刷新数据' }, global: { plugins: [i18n] } })
    expect(w.text()).toBe('刷新数据')
    expect(w.get('button').attributes('disabled')).toBeDefined()
    expect(w.find('[role="status"], .animate-spin').exists()).toBe(true)
  })

  it('disabled 同样禁用', () => {
    const w = mount(SbRefreshButton, { props: { disabled: true }, global: { plugins: [i18n] } })
    expect(w.get('button').attributes('disabled')).toBeDefined()
  })
})

describe('downloadBlob', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('挂锚点点击后移除，并在下一个宏任务回收 URL', () => {
    vi.useFakeTimers()
    const create = vi.fn(() => 'blob:x')
    const revoke = vi.fn()
    Object.assign(URL, { createObjectURL: create, revokeObjectURL: revoke })
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined)
    downloadBlob(new Blob(['a']), 'a.txt')
    expect(click).toHaveBeenCalledTimes(1)
    expect(document.querySelector('a[download="a.txt"]')).toBeNull()
    expect(revoke).not.toHaveBeenCalled()
    vi.advanceTimersByTime(0)
    expect(revoke).toHaveBeenCalledWith('blob:x')
  })

  it('basename 取最后一段，空时回退', () => {
    expect(basename('docs/a.txt')).toBe('a.txt')
    expect(basename('docs/')).toBe('docs')
    expect(basename('')).toBe('download')
    expect(basename('/', 'file')).toBe('file')
  })
})

describe('copyText', () => {
  beforeEach(() => {
    // jsdom 没有 execCommand：默认让降级也失败，各用例再按需覆盖。
    Object.defineProperty(document, 'execCommand', { value: vi.fn(() => false), configurable: true })
  })

  it('成功时 toast 成功并返回 true；失败时 toast 失败并返回 false', async () => {
    const writeText = vi.fn().mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error('denied'))
    Object.assign(navigator, { clipboard: { writeText } })
    await expect(copyText('x', 'ok', 'fail')).resolves.toBe(true)
    expect(toast.success).toHaveBeenCalledWith('ok')
    await expect(copyText('y', 'ok', 'fail')).resolves.toBe(false)
    expect(toast.error).toHaveBeenCalledWith('fail')
  })

  it('按结果延迟求值文案，并处理同步抛错', async () => {
    const writeText = vi.fn()
      .mockResolvedValueOnce(undefined)
      .mockImplementationOnce(() => { throw new Error('denied') })
    Object.assign(navigator, { clipboard: { writeText } })
    const ok = vi.fn(() => 'ok')
    const fail = vi.fn(() => 'fail')
    await expect(copyText('x', ok, fail)).resolves.toBe(true)
    expect(ok).toHaveBeenCalledTimes(1)
    expect(fail).not.toHaveBeenCalled()
    await expect(copyText('y', ok, fail)).resolves.toBe(false)
    expect(ok).toHaveBeenCalledTimes(1)
    expect(fail).toHaveBeenCalledTimes(1)
  })

  it('Clipboard API 被拒时降级 execCommand：降级成功算成功，降级也失败才提示失败', async () => {
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockRejectedValue(new Error('denied')) } })
    const exec = vi.fn().mockReturnValueOnce(true).mockReturnValueOnce(false)
    Object.defineProperty(document, 'execCommand', { value: exec, configurable: true })
    await expect(copyText('a', 'ok', 'fail')).resolves.toBe(true)
    expect(toast.success).toHaveBeenCalledWith('ok')
    expect(document.querySelector('textarea')).toBeNull()
    await expect(copyText('b', 'ok2', (e) => `fail:${(e as Error).message}`)).resolves.toBe(false)
    expect(toast.error).toHaveBeenCalledWith('fail:denied')
    expect(toast.success).not.toHaveBeenCalledWith('ok2')
  })

  it('execCommand 抛错同样视为失败，并清理临时 textarea', async () => {
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockRejectedValue(new Error('denied')) } })
    Object.defineProperty(document, 'execCommand', { value: vi.fn(() => { throw new Error('boom') }), configurable: true })
    await expect(copyText('a', 'ok', 'fail')).resolves.toBe(false)
    expect(document.querySelector('textarea')).toBeNull()
  })
})
