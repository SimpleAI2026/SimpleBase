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
  return { objects, folders, folderMeta: [] }
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

  describe('发布网站 / 外部访问 (planv6.5)', () => {
    const base = 'https://proj0001.simplebase.top'
    function sitePage(enabled: boolean, extra: Partial<{ publishable: boolean; reason: string }> = {}) {
      return {
        folders: ['blog/', 'docs/'],
        folderMeta: [
          { key: 'blog/', public: enabled, publicUrl: enabled ? base + '/blog/' : undefined },
          { key: 'docs/', public: false }
        ],
        objects: [
          { key: 'a.html', size: 1, public: true, publicUrl: enabled ? base + '/a.html' : undefined },
          { key: 'b.html', size: 1, public: true, publicInherited: '/' },
          { key: 'c.txt', size: 1 }
        ],
        site: { enabled, publishable: extra.publishable ?? true, baseUrl: enabled ? base : undefined, reason: extra.reason }
      }
    }

    it('hides the public columns when sites are not enabled on the instance', async () => {
      const { wrapper } = await mountWithApp(S3Manager)
      expect(wrapper.text()).not.toContain('外部访问')
      expect(wrapper.find('.sb-site-panel').exists()).toBe(false)
    })

    it('publishes the site and shows URLs once published', async () => {
      api.s3.listDetailed.mockResolvedValueOnce(sitePage(false))
      api.s3.listDetailed.mockResolvedValue(sitePage(true))
      const { wrapper } = await mountWithApp(S3Manager)
      expect(wrapper.text()).toContain('外部访问')
      expect(wrapper.text()).toContain('访问 URL')
      // 未发布：开关全部禁用
      const switches = wrapper.findAll('.sb-public-switch')
      expect(switches.length).toBe(5)
      expect(switches.every((s) => s.attributes('disabled') !== undefined)).toBe(true)
      await wrapper.get('.sb-site-publish').trigger('click')
      await flushPromises()
      expect(api.s3.setSite).toHaveBeenCalledWith(expect.any(String), true)
      expect(toast.success).toHaveBeenCalledWith('网站已发布')
      expect(wrapper.text()).toContain(base + '/a.html')
      expect(wrapper.text()).toContain(base + '/blog/')
      // 继承自整个桶的 b.html 禁用；c.txt 可切换
      const after = wrapper.findAll('.sb-public-switch')
      expect(after[3].attributes('disabled')).toBeDefined()
      expect(after[4].attributes('disabled')).toBeUndefined()
      expect(wrapper.html()).toContain('继承自整个存储桶')
    })

    it('toggles file and folder public access and rolls back on failure', async () => {
      api.s3.listDetailed.mockResolvedValue(sitePage(true))
      const { wrapper } = await mountWithApp(S3Manager)
      const switches = wrapper.findAll('.sb-public-switch')
      await switches[1].trigger('click') // docs/ folder → on
      await flushPromises()
      expect(api.s3.setPublic).toHaveBeenCalledWith(expect.any(String), 'docs/', 'folder', true)
      expect(toast.success).toHaveBeenCalledWith('已开启外部访问')
      await wrapper.findAll('.sb-public-switch')[2].trigger('click') // a.html → off
      await flushPromises()
      expect(api.s3.setPublic).toHaveBeenCalledWith(expect.any(String), 'a.html', 'file', false)
      expect(toast.success).toHaveBeenCalledWith('已关闭外部访问')
      api.s3.setPublic.mockRejectedValueOnce(new Error('nope'))
      await wrapper.findAll('.sb-public-switch')[4].trigger('click')
      await flushPromises()
      expect(toast.error).toHaveBeenCalledWith('nope')
      api.s3.setPublic.mockRejectedValueOnce('x')
      await wrapper.findAll('.sb-public-switch')[4].trigger('click')
      await flushPromises()
      expect(toast.error).toHaveBeenCalledWith('设置外部访问失败')
    })

    it('copies URLs, unpublishes and reports site errors', async () => {
      const writeText = vi.fn().mockResolvedValue(undefined)
      vi.stubGlobal('navigator', { clipboard: { writeText } })
      api.s3.listDetailed.mockResolvedValue(sitePage(true))
      const { wrapper } = await mountWithApp(S3Manager)
      await wrapper.findAll('.sb-copy-url')[0].trigger('click')
      await flushPromises()
      expect(writeText).toHaveBeenCalledWith(base + '/blog/')
      expect(toast.success).toHaveBeenCalledWith('访问 URL 已复制')
      await clickText(wrapper, '复制 URL')
      await flushPromises()
      expect(writeText).toHaveBeenCalledWith(base)
      writeText.mockRejectedValueOnce(new Error('clip'))
      await (wrapper.vm as any).copyUrl('x')
      expect(toast.error).toHaveBeenCalledWith('clip')
      await (wrapper.vm as any).copyUrl(undefined)
      api.s3.setSite.mockResolvedValueOnce({ enabled: false, publishable: true })
      await wrapper.get('.sb-site-unpublish').trigger('click')
      await flushPromises()
      expect(api.s3.setSite).toHaveBeenCalledWith(expect.any(String), false)
      expect(toast.success).toHaveBeenCalledWith('网站已下线')
      api.s3.setSite.mockRejectedValueOnce('boom')
      await (wrapper.vm as any).setSite(true)
      expect(toast.error).toHaveBeenCalledWith('操作失败')
    })

    it('explains why a project cannot publish', async () => {
      for (const [reason, text] of [
        ['system_project', '系统项目不支持发布网站'],
        ['project_id_not_dns_safe', '无法作为域名'],
        ['sites_disabled', 'sites.enabled'],
        ['other_reason', 'other_reason']
      ]) {
        api.s3.listDetailed.mockResolvedValue(sitePage(false, { publishable: false, reason }))
        const { wrapper } = await mountWithApp(S3Manager)
        expect(wrapper.get('.sb-site-reason').text()).toContain(text)
        expect(wrapper.get('.sb-site-publish').attributes('disabled')).toBeDefined()
      }
      api.s3.listDetailed.mockResolvedValue(sitePage(false, { publishable: false }))
      const { wrapper } = await mountWithApp(S3Manager)
      expect(wrapper.get('.sb-site-reason').text()).toBe('')
      const vm = wrapper.vm as any
      expect(vm.publicHint('blog/')).toContain('blog/')
      expect(vm.publicHint('')).toBe('请先发布网站')
    })

    it('falls back when folder meta is missing', async () => {
      api.s3.listDetailed.mockResolvedValue({ ...sitePage(true), folderMeta: undefined })
      const { wrapper } = await mountWithApp(S3Manager)
      expect(wrapper.text()).toContain('docs/')
      expect((wrapper.vm as any).publicHint('')).toBe('')
    })
  })
})
