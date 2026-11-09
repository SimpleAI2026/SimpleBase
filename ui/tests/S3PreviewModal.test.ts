import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import S3PreviewModal from '@/components/modal/S3PreviewModal.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

const stubs = {
  ...uiStubs,
  ToggleGroup: {
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template:
      '<div><button type="button" class="to-source" @click="$emit(\'update:modelValue\', \'source\')">s</button><button type="button" class="to-rendered" @click="$emit(\'update:modelValue\', \'rendered\')">r</button><slot /></div>'
  }
}

function okText(text: string) {
  return vi.fn().mockResolvedValue({ ok: true, text: async () => text, blob: async () => new Blob([text]) })
}

function mountModal(record: { key: string; size: number; lastModified?: string } | null, open = true) {
  const pinia = createPinia()
  setActivePinia(pinia)
  return mount(S3PreviewModal, {
    props: { open, projectId: 'dev-shop', record },
    global: { plugins: [pinia], stubs }
  })
}

describe('S3PreviewModal', () => {
  beforeEach(() => {
    resetApiMocks()
    vi.stubGlobal('open', vi.fn())
    vi.stubGlobal('fetch', okText('hello world'))
    URL.createObjectURL = vi.fn(() => 'blob:x')
    URL.revokeObjectURL = vi.fn()
  })

  it('renders images through <img> without fetching content', async () => {
    const w = mountModal({ key: 'images/a.png', size: 10 })
    await flushPromises()
    const img = w.get('img')
    expect(img.attributes('src')).toBe('https://example.test/obj')
    expect(img.attributes('alt')).toContain('a.png')
    expect(fetch).not.toHaveBeenCalled()
    expect(w.text()).toContain('a.png')
    expect(w.text()).toContain('下载')
    expect(w.text()).not.toContain('复制内容')
  })

  it('loads plain text and supports copy', async () => {
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
    const w = mountModal({ key: 'app.log', size: 11 })
    await flushPromises()
    expect(fetch).toHaveBeenCalledWith('https://example.test/obj', expect.objectContaining({ signal: expect.anything() }))
    expect(w.get('pre').text()).toBe('hello world')
    const copy = w.findAll('button').find((b) => b.text().includes('复制内容'))!
    await copy.trigger('click')
    await flushPromises()
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('hello world')
    expect(toast.success).toHaveBeenCalledWith('已复制')
  })

  it('reports copy failure', async () => {
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockRejectedValue(new Error('nope')) } })
    const w = mountModal({ key: 'a.txt', size: 3 })
    await flushPromises()
    await w.findAll('button').find((b) => b.text().includes('复制内容'))!.trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('nope')
  })

  it('renders markdown and toggles to source', async () => {
    vi.stubGlobal('fetch', okText('# Title\n\n<script>alert(1)</script>'))
    const w = mountModal({ key: 'notes/README.md', size: 30 })
    await flushPromises()
    const rendered = w.get('[data-testid="s3-preview-markdown"]')
    expect(rendered.html()).toContain('<h1')
    expect(rendered.html()).not.toContain('<script>')
    await w.get('.to-source').trigger('click')
    expect(w.find('[data-testid="s3-preview-markdown"]').exists()).toBe(false)
    expect(w.get('pre').text()).toContain('# Title')
    await w.get('.to-rendered').trigger('click')
    expect(w.find('[data-testid="s3-preview-markdown"]').exists()).toBe(true)
  })

  it('skips fetching oversized text files', async () => {
    const w = mountModal({ key: 'big.log', size: 2 * 1024 * 1024 })
    await flushPromises()
    expect(fetch).not.toHaveBeenCalled()
    expect(w.text()).toContain('预览上限')
  })

  it('shows an empty state for unsupported types and still allows download', async () => {
    const w = mountModal({ key: 'pack.zip', size: 5 })
    await flushPromises()
    expect(w.text()).toContain('暂不支持预览该类型')
    expect(fetch).not.toHaveBeenCalled()
    expect(w.text()).toContain('下载')
  })

  it('shows an error state when presign or fetch fails and retries', async () => {
    api.s3.presign.mockRejectedValueOnce(new Error('presign boom'))
    const w = mountModal({ key: 'a.txt', size: 1 })
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('presign boom')
    api.s3.presign.mockResolvedValue({ url: 'https://example.test/obj' })
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, text: async () => '' }))
    await w.get('.alert').element.parentElement!.querySelector('button')!.click()
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('预览加载失败')
  })

  it('downloads through a blob link', async () => {
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const w = mountModal({ key: 'dir/a.txt', size: 3 })
    await flushPromises()
    await w.findAll('button').find((b) => b.text().trim() === '下载')!.trigger('click')
    await flushPromises()
    expect(URL.createObjectURL).toHaveBeenCalled()
    expect(click).toHaveBeenCalled()
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:x')
    click.mockRestore()
  })

  it('falls back to window.open when the download fetch fails', async () => {
    const w = mountModal({ key: 'p.png', size: 3 })
    await flushPromises()
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('cors')))
    await w.findAll('button').find((b) => b.text().trim() === '下载')!.trigger('click')
    await flushPromises()
    expect(window.open).toHaveBeenCalledWith('https://example.test/obj', '_blank')
    expect(toast.error).toHaveBeenCalledWith('下载失败，已尝试在新窗口打开')
  })

  it('reports a link error when download cannot presign at all', async () => {
    const w = mountModal({ key: 'p.zip', size: 3 })
    await flushPromises()
    api.s3.presign.mockRejectedValue(new Error('x'))
    ;(w.vm as unknown as { $: { setupState: { url: string } } }).$.setupState.url = ''
    await w.findAll('button').find((b) => b.text().trim() === '下载')!.trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('生成链接失败')
  })

  it('closes via footer button, aborts and resets when closed', async () => {
    const w = mountModal({ key: 'a.txt', size: 1 })
    await flushPromises()
    await w.findAll('button').find((b) => b.text().trim() === '关闭')!.trigger('click')
    expect(w.emitted('update:open')?.[0]).toEqual([false])
    await w.setProps({ open: false })
    expect(w.find('.sb-modal').exists()).toBe(false)
    await w.setProps({ open: true, record: { key: 'b.png', size: 1 } })
    await flushPromises()
    expect(api.s3.presign).toHaveBeenLastCalledWith('dev-shop', 'b.png')
    w.unmount()
  })

  it('cleans up the blob object URL when closed after a download', async () => {
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const w = mountModal({ key: 'dir/a.txt', size: 3 })
    await flushPromises()
    await w.findAll('button').find((b) => b.text().trim() === '下载')!.trigger('click')
    await flushPromises()
    vi.mocked(URL.createObjectURL).mockReturnValue('blob:pending')
    await w.findAll('button').find((b) => b.text().trim() === '下载')!.trigger('click')
    await flushPromises()
    // 第二次下载留下未被 revoke 的 objectUrl 时，关闭会补一次清理
    ;(w.vm as unknown as { $: { setupState: { url: string } } }).$.setupState.url = ''
    await w.findAll('button').find((b) => b.text().trim() === '下载')!.trigger('click')
    await flushPromises()
    await w.setProps({ open: false })
    await flushPromises()
    expect(URL.revokeObjectURL).toHaveBeenCalled()
    click.mockRestore()
  })

  it('presigns again for download when the preview URL is missing', async () => {
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const w = mountModal({ key: 'p2.png', size: 3 })
    await flushPromises()
    api.s3.presign.mockClear()
    ;(w.vm as unknown as { $: { setupState: { url: string } } }).$.setupState.url = ''
    await w.findAll('button').find((b) => b.text().trim() === '下载')!.trigger('click')
    await flushPromises()
    expect(api.s3.presign).toHaveBeenCalledWith('dev-shop', 'p2.png')
    expect(click).toHaveBeenCalled()
    click.mockRestore()
  })

  it('renders nothing meaningful without a record', async () => {
    const w = mountModal(null)
    await flushPromises()
    expect(api.s3.presign).not.toHaveBeenCalled()
    w.unmount()
  })
})
