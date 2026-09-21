import { flushPromises } from '@vue/test-utils'
import axios from 'axios'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks, setIsMock } from '../test/api-mock'
import { clickText, mountWithApp } from '../test/helpers'

vi.mock('../services/api', async () => {
  const m = await import('../test/api-mock')
  return {
    api: m.api,
    get isMock() {
      return m.getIsMock()
    }
  }
})

vi.mock('axios', () => ({
  default: { post: vi.fn() }
}))

vi.mock('../services/http', () => ({
  getApiKey: () => 'sb_key',
  baseURL: 'http://api.test'
}))

import S3Manager from './S3Manager.vue'

function fileNamed(name: string, size = 4) {
  const f = new File(['abcd'], name, { type: 'text/plain' })
  Object.defineProperty(f, 'size', { value: size })
  return f
}

async function changeFile(wrapper: Awaited<ReturnType<typeof mountWithApp>>['wrapper'], file: File) {
  const input = wrapper.get('input[type="file"]')
  Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
  await input.trigger('change')
  await flushPromises()
}

describe('S3Manager', () => {
  beforeEach(() => {
    resetApiMocks()
    setIsMock(false)
    vi.mocked(axios.post).mockReset()
    vi.mocked(axios.post).mockResolvedValue({ data: {} })
    vi.stubGlobal('open', vi.fn())
  })

  it('lists objects and opens / deletes them', async () => {
    api.s3.list.mockResolvedValue([{ key: 'a.txt', size: 12, lastModified: '2024-01-01T00:00:00Z' }])
    const { wrapper } = await mountWithApp(S3Manager)
    expect(wrapper.text()).toContain('a.txt')
    await clickText(wrapper, '打开')
    await flushPromises()
    expect(window.open).toHaveBeenCalledWith('https://example.test/obj', '_blank')
    await clickText(wrapper, '删除')
    await flushPromises()
    expect(api.s3.remove).toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalledWith('删除成功')
  })

  it('opens the file picker from the toolbar upload button and posts the file', async () => {
    api.s3.list.mockResolvedValue([{ key: 'a.txt', size: 12, lastModified: '2024-01-01T00:00:00Z' }])
    const { wrapper } = await mountWithApp(S3Manager)
    const input = wrapper.get('input[type="file"]')
    const click = vi.fn(() => {
      Object.defineProperty(input.element, 'files', {
        value: [fileNamed('picked.txt')],
        configurable: true
      })
      return input.trigger('change')
    })
    ;(input.element as HTMLInputElement).click = click

    await clickText(wrapper, '上传对象')
    expect(click).toHaveBeenCalled()
    await flushPromises()

    expect(axios.post).toHaveBeenCalled()
    const [url, body] = vi.mocked(axios.post).mock.calls[0]
    expect(String(url)).toContain('/s3/objects')
    expect(body).toBeInstanceOf(FormData)
  })

  it('handles list / delete / presign errors and empty upload trigger', async () => {
    api.s3.list.mockRejectedValueOnce(new Error('list boom'))
    const { wrapper } = await mountWithApp(S3Manager)
    expect(toast.error).toHaveBeenCalledWith('list boom')
    api.s3.list.mockResolvedValueOnce([])
    await clickText(wrapper, '刷新')
    await flushPromises()
    const click = vi.fn()
    const input = wrapper.get('input[type="file"]')
    ;(input.element as HTMLInputElement).click = click
    await wrapper.get('.empty-action').trigger('click')
    expect(click).toHaveBeenCalled()

    api.s3.list.mockResolvedValue([{ key: 'b', size: 1 }])
    api.s3.presign.mockRejectedValueOnce('x')
    api.s3.remove.mockRejectedValueOnce(new Error('rm'))
    await clickText(wrapper, '刷新')
    await flushPromises()
    await clickText(wrapper, '打开')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('生成链接失败')
    await clickText(wrapper, '删除')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('rm')
  })

  it('validates keys and uploads via axios with progress', async () => {
    const { wrapper } = await mountWithApp(S3Manager)
    await changeFile(wrapper, fileNamed('ok.txt'))
    expect(axios.post).toHaveBeenCalled()
    const cfg = vi.mocked(axios.post).mock.calls[0][2] as { onUploadProgress: (e: { loaded: number; total: number }) => void }
    cfg.onUploadProgress({ loaded: 50, total: 100 })

    vi.mocked(axios.post).mockRejectedValueOnce({ response: { data: { error: { message: 'denied' } } } })
    await changeFile(wrapper, fileNamed('fail.txt'))
    expect(toast.error).toHaveBeenCalledWith('denied')

    vi.mocked(axios.post).mockRejectedValueOnce({ message: 'net' })
    await changeFile(wrapper, fileNamed('fail2.txt'))
    expect(toast.error).toHaveBeenCalledWith('net')

    await changeFile(wrapper, fileNamed('../secret'))
    expect(toast.warning).toHaveBeenCalled()
    await changeFile(wrapper, fileNamed('a\\b'))
    await changeFile(wrapper, fileNamed('a\0b'))
    await changeFile(wrapper, fileNamed('a'.repeat(1025)))
    await changeFile(wrapper, fileNamed('ok.txt', 11 * 1024 * 1024))
    expect(toast.warning).toHaveBeenCalled()
  })

  it('uses mock upload path when isMock is on', async () => {
    setIsMock(true)
    const { wrapper } = await mountWithApp(S3Manager)
    api.s3.upload.mockResolvedValueOnce({ key: 'm.txt', size: 1 })
    await changeFile(wrapper, fileNamed('m.txt'))
    expect(api.s3.upload).toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalled()

    api.s3.upload.mockRejectedValueOnce(new Error('up'))
    await changeFile(wrapper, fileNamed('m2.txt'))
    expect(toast.error).toHaveBeenCalledWith('up')
  })

  it('reloads when the project changes and ignores empty file picks', async () => {
    const { wrapper, pinia } = await mountWithApp(S3Manager)
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [], configurable: true })
    await input.trigger('change')
    const { useProjectStore } = await import('../stores/project')
    useProjectStore(pinia).setProject('00000000-0000-0000-0000-000000000003')
    await flushPromises()
    expect(api.s3.list.mock.calls.length).toBeGreaterThan(1)
  })
})
