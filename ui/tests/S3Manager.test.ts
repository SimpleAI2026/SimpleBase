import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { clickText, mountWithApp } from '@/test/helpers'
import type { S3Object } from '@/services/types'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})
import S3Manager from '@/pages/S3Manager.vue'

function fileNamed(name: string, size = 4) {
  const f = new File(['abcd'], name, { type: 'text/plain' })
  Object.defineProperty(f, 'size', { value: size })
  return f
}
function page(objects: S3Object[], folders: string[] = []) {
  return { objects, folders }
}
async function changeFile(wrapper: Awaited<ReturnType<typeof mountWithApp>>['wrapper'], file: File) {
  const input = wrapper.get('input[type="file"]')
  Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
  await input.trigger('change')
  await flushPromises()
}

describe('S3Manager (对象存储)', () => {
  beforeEach(() => {
    resetApiMocks()
    vi.stubGlobal('open', vi.fn())
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, text: async () => 'body', blob: async () => new Blob(['x']) }))
  })

  it('lists objects, previews them in a modal and deletes them', async () => {
    api.s3.listDetailed.mockResolvedValue(page([{ key: 'a.txt', size: 12, lastModified: '2024-01-01T00:00:00Z' }]))
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, text: async () => 'hello a' }))
    const { wrapper } = await mountWithApp(S3Manager)
    expect(wrapper.text()).toContain('a.txt')
    await clickText(wrapper, '打开')
    await flushPromises()
    expect(api.s3.presign).toHaveBeenCalledWith(expect.any(String), 'a.txt')
    expect(wrapper.find('.sb-modal').exists()).toBe(true)
    expect(wrapper.find('.sb-modal').text()).toContain('hello a')
    await wrapper.get('.sb-modal-close').trigger('click')
    await clickText(wrapper, '删除')
    await flushPromises()
    expect(api.s3.remove).toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalledWith('删除成功')
    await wrapper.get('.pager-next').trigger('click')
  })

  it('opens the file picker and uploads', async () => {
    const { wrapper } = await mountWithApp(S3Manager)
    const input = wrapper.get('input[type="file"]')
    const click = vi.fn(() => {
      Object.defineProperty(input.element, 'files', { value: [fileNamed('picked.txt')], configurable: true })
      return input.trigger('change')
    })
    ;(input.element as HTMLInputElement).click = click
    await clickText(wrapper, '上传对象')
    expect(click).toHaveBeenCalled()
    await flushPromises()
    expect(api.s3.upload).toHaveBeenCalled()
  })

  it('handles list / delete / presign errors and empty upload trigger', async () => {
    api.s3.listDetailed.mockRejectedValueOnce(new Error('list boom'))
    const { wrapper } = await mountWithApp(S3Manager)
    expect(toast.error).toHaveBeenCalledWith('list boom')
    api.s3.listDetailed.mockResolvedValueOnce(page([]))
    await clickText(wrapper, '刷新')
    await flushPromises()
    const click = vi.fn()
    const input = wrapper.get('input[type="file"]')
    ;(input.element as HTMLInputElement).click = click
    await wrapper.get('.empty-action').trigger('click')
    expect(click).toHaveBeenCalled()
    api.s3.listDetailed.mockResolvedValue(page([{ key: 'b', size: 1 }]))
    api.s3.presign.mockRejectedValueOnce('x')
    api.s3.remove.mockRejectedValueOnce(new Error('rm'))
    await clickText(wrapper, '刷新')
    await flushPromises()
    await clickText(wrapper, '打开')
    await flushPromises()
    await clickText(wrapper, '删除')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('rm')
  })

  it('validates keys and reports upload progress', async () => {
    const { wrapper } = await mountWithApp(S3Manager)
    await changeFile(wrapper, fileNamed('ok.txt'))
    const onProgress = api.s3.upload.mock.calls[0][3] as (pct: number) => void
    onProgress(50)
    api.s3.upload.mockRejectedValueOnce(new Error('denied'))
    await changeFile(wrapper, fileNamed('fail.txt'))
    expect(toast.error).toHaveBeenCalledWith('denied')
    api.s3.upload.mockRejectedValueOnce('net')
    await changeFile(wrapper, fileNamed('fail2.txt'))
    expect(toast.error).toHaveBeenCalledWith('上传失败')
    await changeFile(wrapper, fileNamed('../secret'))
    await changeFile(wrapper, fileNamed('a\\b'))
    await changeFile(wrapper, fileNamed('a\0b'))
    await changeFile(wrapper, fileNamed('a'.repeat(1025)))
    await changeFile(wrapper, fileNamed('ok.txt', 51 * 1024 * 1024))
    expect(toast.warning).toHaveBeenCalled()
  })

  it('reloads when the project changes and ignores empty file picks', async () => {
    const { wrapper, pinia } = await mountWithApp(S3Manager)
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [], configurable: true })
    await input.trigger('change')
    const { useProjectStore } = await import('@/stores/project')
    useProjectStore(pinia).setProject('other-proj')
    await flushPromises()
    expect(api.s3.listDetailed.mock.calls.length).toBeGreaterThan(1)
  })

  it('filters prefixes, opens folders and creates private links', async () => {
    api.s3.listDetailed.mockResolvedValue(page([{ key: 'images/a.png', size: 8, lastModified: '2024-01-01T00:00:00Z' }], ['images/icons/']))
    const { wrapper } = await mountWithApp(S3Manager)
    expect(wrapper.text()).toContain('images/icons/')
    await clickText(wrapper, 'images/icons/')
    expect(api.s3.listDetailed).toHaveBeenCalledWith(expect.any(String), 'images/icons/')
    const prefix = wrapper.get('.ig-input')
    await prefix.setValue('images/')
    await prefix.trigger('keydown.enter')
    await flushPromises()
    expect(api.s3.listDetailed).toHaveBeenCalledWith(expect.any(String), 'images/')
    await clickText(wrapper, '复制私链')
    await flushPromises()
    expect(api.s3.createLink).toHaveBeenCalledWith(expect.any(String), 'images/a.png')
  })

  it('handles non-Error responses and upload errors', async () => {
    const { wrapper } = await mountWithApp(S3Manager)
    api.s3.listDetailed.mockRejectedValueOnce('list')
    await wrapper.vm.load()
    api.s3.remove.mockRejectedValueOnce('rm')
    await wrapper.vm.remove('k')
    api.s3.upload.mockRejectedValueOnce(new Error('up'))
    await changeFile(wrapper, fileNamed('m2.txt'))
    expect(toast.error).toHaveBeenCalledWith('up')
  })

  it('downloads blobs and reports download/link failures', async () => {
    Object.assign(URL, { createObjectURL: vi.fn(() => 'blob:test'), revokeObjectURL: vi.fn() })
    const { wrapper } = await mountWithApp(S3Manager)
    await wrapper.vm.download({ key: 'docs/a.txt', size: 1 })
    expect(api.s3.download).toHaveBeenCalled()
    expect(URL.revokeObjectURL).toHaveBeenCalled()
    api.s3.download.mockRejectedValueOnce(new Error('download'))
    await wrapper.vm.download({ key: 'a', size: 1 })
    api.s3.createLink.mockRejectedValueOnce(new Error('link'))
    await wrapper.vm.copyPrivateLink({ key: 'a', size: 1 })
    expect(toast.error).toHaveBeenCalledWith('download')
    expect(toast.error).toHaveBeenCalledWith('link')
  })

  it('validates key variants and previews images', async () => {
    const { wrapper: w } = await mountWithApp(S3Manager)
    const vm = w.vm as any
    expect(vm.validateKey('')).toBeTruthy()
    expect(vm.validateKey('../x')).toBeTruthy()
    expect(vm.validateKey('ok.txt')).toBe('')
    vm.openPreview({ key: 'a.png', size: 1 })
    await flushPromises()
    expect(api.s3.presign).toHaveBeenCalled()
    await changeFile(w, fileNamed('m.txt'))
    expect(api.s3.upload).toHaveBeenCalled()
  })
})
