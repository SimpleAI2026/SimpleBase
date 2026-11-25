import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { clickText, mountWithApp, closeDialog, dialogEl, installJsdomPolyfills, realDialogStubs, inDialog, confirmAction, realAlertDialogStubs } from '@/test/helpers'
import type { S3Object } from '@/services/types'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})
import S3Manager from '@/pages/S3Manager.vue'

/** S3Manager 在 <script setup> 内的内部方法（测试直接驱动错误分支）。 */
interface S3ManagerVM {
  load: () => Promise<void>
  remove: (key: string) => Promise<void>
}
const s3vm = (w: { vm: unknown }) => w.vm as S3ManagerVM

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

installJsdomPolyfills()

describe('S3Manager (对象存储)', () => {
  beforeEach(() => {
    resetApiMocks()
    vi.stubGlobal('open', vi.fn())
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, text: async () => 'body', blob: async () => new Blob(['x']) }))
  })

  it('lists objects, previews them in a modal and deletes them', async () => {
    api.s3.listDetailed.mockResolvedValue(page([{ key: 'a.txt', size: 12, lastModified: '2024-01-01T00:00:00Z' }]))
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, text: async () => 'hello a' }))
    const { wrapper } = await mountWithApp(S3Manager, { stubs: { ...realDialogStubs, ...realAlertDialogStubs } })
    expect(wrapper.text()).toContain('a.txt')
    await clickText(wrapper, '打开')
    await flushPromises()
    expect(api.s3.presign).toHaveBeenCalledWith(expect.any(String), 'a.txt')
    expect(dialogEl()).not.toBeNull()
    expect(inDialog().text()).toContain('hello a')
    await closeDialog()
    await confirmAction(wrapper, '删除')
    expect(api.s3.remove).toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalledWith('删除成功')
    await wrapper.get('.pager-next').trigger('click')
  })

  it('opens the file picker and uploads', async () => {
    const { wrapper } = await mountWithApp(S3Manager, { stubs: { ...realDialogStubs, ...realAlertDialogStubs } })
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
    const { wrapper } = await mountWithApp(S3Manager, { stubs: { ...realDialogStubs, ...realAlertDialogStubs } })
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
    await confirmAction(wrapper, '删除')
    expect(toast.error).toHaveBeenCalledWith('rm')
  })

  it('validates keys and reports upload progress', async () => {
    const { wrapper } = await mountWithApp(S3Manager, { stubs: { ...realDialogStubs, ...realAlertDialogStubs } })
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

  it('uploads into the current folder prefix (B7) but keeps root uploads for plain prefixes', async () => {
    api.s3.listDetailed.mockResolvedValue(page([], ['docs/']))
    const { wrapper } = await mountWithApp(S3Manager, { stubs: { ...realDialogStubs, ...realAlertDialogStubs } })
    await clickText(wrapper, 'docs/')
    await flushPromises()
    await changeFile(wrapper, fileNamed('a.txt'))
    expect(api.s3.upload.mock.calls.at(-1)?.[1]).toBe('docs/a.txt')
    const prefix = wrapper.get('.ig-input')
    await prefix.setValue('img')
    await changeFile(wrapper, fileNamed('b.txt'))
    expect(api.s3.upload.mock.calls.at(-1)?.[1]).toBe('b.txt')
  })

  it('X4：进入子目录显示面包屑，点击上级逐级返回', async () => {
    api.s3.listDetailed.mockResolvedValue(page([], ['a/']))
    const { wrapper } = await mountWithApp(S3Manager, { stubs: { ...realDialogStubs, ...realAlertDialogStubs } })
    expect(wrapper.find('.sb-s3-crumbs').exists()).toBe(false)
    await wrapper.get('.ig-input').setValue('a/b/')
    await flushPromises()
    const nav = wrapper.get('.sb-s3-crumbs')
    expect(nav.text()).toContain('根目录')
    expect(nav.text()).toContain('a')
    expect(nav.text()).toContain('b')
    await nav.findAll('button').find((b) => b.text() === 'a')!.trigger('click')
    await flushPromises()
    expect(api.s3.listDetailed).toHaveBeenLastCalledWith(expect.any(String), 'a/')
    await wrapper.get('.sb-crumb-root').trigger('click')
    await flushPromises()
    expect(api.s3.listDetailed).toHaveBeenLastCalledWith(expect.any(String), undefined)
    expect(wrapper.find('.sb-s3-crumbs').exists()).toBe(false)
  })

  it('X4：输入前缀 300ms 后自动查询，连续输入只发一次请求', async () => {
    vi.useFakeTimers()
    try {
      const { wrapper } = await mountWithApp(S3Manager, { stubs: { ...realDialogStubs, ...realAlertDialogStubs } })
      const before = api.s3.listDetailed.mock.calls.length
      const input = wrapper.get('.ig-input')
      await input.setValue('im')
      await input.setValue('img/')
      await vi.advanceTimersByTimeAsync(299)
      expect(api.s3.listDetailed.mock.calls.length).toBe(before)
      await vi.advanceTimersByTimeAsync(2)
      await flushPromises()
      expect(api.s3.listDetailed.mock.calls.length).toBe(before + 1)
      expect(api.s3.listDetailed).toHaveBeenLastCalledWith(expect.any(String), 'img/')
      wrapper.unmount()
    } finally {
      vi.useRealTimers()
    }
  })

  it('reloads when the project changes and ignores empty file picks', async () => {
    const { wrapper, pinia } = await mountWithApp(S3Manager, { stubs: { ...realDialogStubs, ...realAlertDialogStubs } })
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
    const { wrapper } = await mountWithApp(S3Manager, { stubs: { ...realDialogStubs, ...realAlertDialogStubs } })
    expect(wrapper.text()).toContain('images/icons/')
    await clickText(wrapper, 'images/icons/')
    expect(api.s3.listDetailed).toHaveBeenCalledWith(expect.any(String), 'images/icons/')
    const prefix = wrapper.get('.ig-input')
    await prefix.setValue('images/')
    await prefix.trigger('keydown.enter')
    await flushPromises()
    expect(api.s3.listDetailed).toHaveBeenCalledWith(expect.any(String), 'images/')
    // R3：私链复制收进预览弹窗，行内不再出现
    expect(wrapper.text()).not.toContain('复制私链')
    await clickText(wrapper, '打开')
    await flushPromises()
    expect(api.s3.presign).toHaveBeenCalled()
  })

  it('handles non-Error responses and upload errors', async () => {
    const { wrapper } = await mountWithApp(S3Manager, { stubs: { ...realDialogStubs, ...realAlertDialogStubs } })
    api.s3.listDetailed.mockRejectedValueOnce('list')
    await s3vm(wrapper).load()
    api.s3.remove.mockRejectedValueOnce('rm')
    await s3vm(wrapper).remove('k')
    api.s3.upload.mockRejectedValueOnce(new Error('up'))
    await changeFile(wrapper, fileNamed('m2.txt'))
    expect(toast.error).toHaveBeenCalledWith('up')
  })

  it('previews and downloads via S3PreviewModal (R3)', async () => {
    api.s3.listDetailed.mockResolvedValue(page([{ key: 'docs/a.txt', size: 8, lastModified: '2024-01-01T00:00:00Z' }]))
    const { wrapper } = await mountWithApp(S3Manager, {
      stubs: {
        S3PreviewModal: {
          props: ['open', 'projectId', 'record'],
          emits: ['update:open'],
          template: '<div class="preview-stub" v-if="open">{{ record?.key }}</div>'
        }
      }
    })
    await clickText(wrapper, '打开')
    await flushPromises()
    expect(wrapper.get('.preview-stub').text()).toContain('docs/a.txt')
    expect(wrapper.text()).not.toContain('复制私链')
    expect(wrapper.text()).not.toContain('下载')
  })

  it('validates key variants and previews images', async () => {
    const { wrapper: w } = await mountWithApp(S3Manager, { stubs: { ...realDialogStubs, ...realAlertDialogStubs } })
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
    function sitePage(publishable: boolean, extra: Partial<{ reason: string }> = {}) {
      return {
        folders: ['blog/', 'docs/'],
        folderMeta: [
          { key: 'blog/', public: publishable, publicUrl: publishable ? base + '/blog/' : undefined },
          { key: 'docs/', public: false }
        ],
        objects: [
          { key: 'a.html', size: 1, public: true, publicUrl: publishable ? base + '/a.html' : undefined },
          { key: 'b.html', size: 1, public: true, publicInherited: '/' },
          { key: 'c.txt', size: 1 }
        ],
        site: { publishable, baseUrl: publishable ? base : undefined, reason: extra.reason }
      }
    }

    it('hides the public columns when the site domain is not configured', async () => {
      const { wrapper } = await mountWithApp(S3Manager, { stubs: { ...realDialogStubs, ...realAlertDialogStubs } })
      expect(wrapper.text()).not.toContain('外部访问')
      expect(wrapper.find('.sb-site-panel').exists()).toBe(false)
    })

    it('shows URLs for public paths without a publish step', async () => {
      api.s3.listDetailed.mockResolvedValue(sitePage(true))
      const { wrapper } = await mountWithApp(S3Manager, { stubs: { ...realDialogStubs, ...realAlertDialogStubs } })
      expect(wrapper.text()).toContain('外部访问')
      expect(wrapper.text()).toContain('访问 URL')
      expect(wrapper.text()).toContain(base + '/a.html')
      expect(wrapper.text()).toContain(base + '/blog/')
      expect(wrapper.find('.sb-site-publish').exists()).toBe(false)
      expect(wrapper.find('.sb-site-unpublish').exists()).toBe(false)
      // 继承自整个桶的 b.html 禁用；c.txt 可切换
      const switches = wrapper.findAll('.sb-public-switch')
      expect(switches[3].attributes('disabled')).toBeDefined()
      expect(switches[4].attributes('disabled')).toBeUndefined()
      expect(wrapper.html()).toContain('继承自整个存储桶')
    })

    it('toggles file and folder public access and rolls back on failure', async () => {
      api.s3.listDetailed.mockResolvedValue(sitePage(true))
      const { wrapper } = await mountWithApp(S3Manager, { stubs: { ...realDialogStubs, ...realAlertDialogStubs } })
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

    it('copies public URLs from the table and reports clipboard errors', async () => {
      const writeText = vi.fn().mockResolvedValue(undefined)
      vi.stubGlobal('navigator', { clipboard: { writeText } })
      api.s3.listDetailed.mockResolvedValue(sitePage(true))
      const { wrapper } = await mountWithApp(S3Manager, { stubs: { ...realDialogStubs, ...realAlertDialogStubs } })
      await wrapper.findAll('.sb-copy-url')[0].trigger('click')
      await flushPromises()
      expect(writeText).toHaveBeenCalledWith(base + '/blog/')
      expect(toast.success).toHaveBeenCalledWith('访问 URL 已复制')
      await wrapper.findAll('.sb-copy-url')[1].trigger('click')
      await flushPromises()
      expect(writeText).toHaveBeenCalledWith(base + '/a.html')
      writeText.mockRejectedValueOnce(new Error('clip'))
      await (wrapper.vm as any).copyUrl('x')
      expect(toast.error).toHaveBeenCalledWith('clip')
      await (wrapper.vm as any).copyUrl(undefined)
    })

    it('keeps public switches disabled when the project cannot publish', async () => {
      api.s3.listDetailed.mockResolvedValue(sitePage(false, { reason: 'site_domain_unconfigured' }))
      const { wrapper } = await mountWithApp(S3Manager, { stubs: { ...realDialogStubs, ...realAlertDialogStubs } })
      expect(wrapper.find('.sb-site-panel').exists()).toBe(false)
      expect(wrapper.findAll('.sb-public-switch').every((s) => s.attributes('disabled') !== undefined)).toBe(true)
      const vm = wrapper.vm as any
      expect(vm.publicHint('blog/')).toContain('blog/')
      expect(vm.publicHint('')).toBe('当前项目不可发布站点')
    })

    it('shows the site address by mode and the site_unavailable hint (planv6.7 §1.6)', async () => {
      api.s3.listDetailed.mockResolvedValue({
        ...sitePage(true),
        site: { publishable: true, baseUrl: 'https://h.test/sites/proj0001', mode: 'path' }
      })
      const { wrapper } = await mountWithApp(S3Manager, { stubs: { ...realDialogStubs, ...realAlertDialogStubs } })
      expect(wrapper.find('.sb-site-mode').text()).toContain('站点地址：https://h.test/sites/proj0001')
      expect(wrapper.find('.sb-site-mode').text()).toContain('无法访问控制台登录态')

      api.s3.listDetailed.mockResolvedValue({
        ...sitePage(true),
        site: { publishable: true, baseUrl: base, mode: 'subdomain' }
      })
      const sub = await mountWithApp(S3Manager, { stubs: { ...realDialogStubs, ...realAlertDialogStubs } })
      expect(sub.wrapper.find('.sb-site-mode').text()).toContain('站点地址：' + base)
      expect(sub.wrapper.find('.sb-site-mode').text()).not.toContain('控制台登录态')

      api.s3.listDetailed.mockResolvedValue({
        ...sitePage(false, { reason: 'site_unavailable' }),
        site: { publishable: false, reason: 'site_unavailable', mode: 'path' }
      })
      const off = await mountWithApp(S3Manager, { stubs: { ...realDialogStubs, ...realAlertDialogStubs } })
      expect(off.wrapper.find('.sb-site-mode').exists()).toBe(false)
      expect((off.wrapper.vm as any).publicHint('')).toBe('站点功能未配置')
    })

    it('falls back when folder meta is missing', async () => {
      api.s3.listDetailed.mockResolvedValue({ ...sitePage(true), folderMeta: undefined })
      const { wrapper } = await mountWithApp(S3Manager, { stubs: { ...realDialogStubs, ...realAlertDialogStubs } })
      expect(wrapper.text()).toContain('docs/')
      expect((wrapper.vm as any).publicHint('')).toBe('')
    })
  })
})
