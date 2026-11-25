import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth'
import { resetApiMocks } from '@/test/api-mock'
import { clickTab, dialogEl, installJsdomPolyfills, mountWithApp, realDialogStubs, dialogTitle, inDialog, waitDialog } from '@/test/helpers'
import SettingsModal from '@/components/SettingsModal.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

installJsdomPolyfills()

describe('SettingsModal', () => {
  beforeEach(() => {
    resetApiMocks()
  })

  it('opens as a 720px SbModal with tabs and closes from the shell', async () => {
    const { wrapper, pinia } = await mountWithApp(SettingsModal, { stubs: realDialogStubs })
    const auth = useAuthStore(pinia)
    expect(dialogEl()).toBeNull()
    auth.openSettings()
    await flushPromises()
    // 真实 SbModal：标题走 dialog-title 节点，宽度走 CSS 变量，hideFooter 时不渲染页脚。
    await waitDialog()
    expect(dialogTitle()).toBe('设置')
    expect(dialogEl()!.getAttribute('style')).toContain('--sb-modal-max-w: 720px')
    expect(document.querySelector('[data-slot="dialog-footer"]')).toBeNull()
    expect(inDialog().text()).toContain('连接、存储与备份')
    expect(inDialog().text()).toContain('连接')
    expect(inDialog().text()).not.toContain('外观')
    const scroller = inDialog().get('[class*="max-h-[min(calc(100vh-14rem),720px)]"]')
    expect(scroller.classes().join(' ')).toContain('max-h-[min(calc(100vh-14rem),720px)]')
    expect(scroller.classes()).toContain('px-1')
    // 「模型」页已迁至 /console/models（planv6.5），弹窗只留三页签
    const tabs = inDialog().findAll('[role="tab"]').map((t) => t.attributes('id')!.split('-trigger-')[1])
    expect(tabs).toEqual(['connection', 'storage', 'backups'])

    // 真实 TabsTrigger：点「存储」切到 storage，再点回「连接」
    await clickTab(wrapper, 'storage')
    expect(auth.settingsTab).toBe('storage')
    await clickTab(wrapper, 'connection')
    expect(auth.settingsTab).toBe('connection')

    const vm = wrapper.vm as any
    vm.onTab('appearance')
    expect(auth.settingsTab).toBe('connection')
    vm.onTab('models')
    expect(auth.settingsTab).toBe('connection')
    vm.onTab('providers')
    expect(auth.settingsTab).toBe('connection')
    vm.onTab('nope')
    expect(auth.settingsTab).toBe('connection')
    vm.onOpen(true)
    expect(auth.settingsOpen).toBe(true)
    vm.onOpen(false)
    expect(auth.settingsOpen).toBe(false)
  })

  it('keeps the settings max-width inside the viewport-capped shell', async () => {
    const { pinia } = await mountWithApp(SettingsModal, { attachTo: document.body, stubs: realDialogStubs })
    const auth = useAuthStore(pinia)
    expect(dialogEl()).toBeNull()
    auth.openSettings()
    await flushPromises()
    // 真实 Dialog：断言打在真实 DOM 的 class / 内联样式 / data-slot 上
    const content = dialogEl()!
    const cls = content.getAttribute('class') || ''
    expect(cls).toContain('max-h-[calc(100vh-2rem)]')
    expect(cls).toContain('flex')
    expect(cls).toContain('sm:max-w-[var(--sb-modal-max-w)]')
    expect(cls).not.toContain('sm:max-w-5xl')
    expect(content.getAttribute('style') || '').toContain('--sb-modal-max-w: 720px')
    const body = document.body.querySelector<HTMLElement>('[data-slot="sb-modal-body"]')!
    expect(Array.from(body.classList)).toContain('overflow-y-auto')
    expect(body.textContent).toContain('连接')
    expect(body.textContent).not.toContain('默认供应商')
    expect(document.body.querySelector('[data-slot="dialog-header"]')!.textContent).toContain('设置')
    expect(document.body.querySelector('[data-slot="dialog-footer"]')).toBeNull()
  })

  it('401 no longer forces the connection tab (planv5.0 §2.2)', async () => {
    const { wrapper, pinia } = await mountWithApp(SettingsModal, { stubs: realDialogStubs })
    const auth = useAuthStore(pinia)
    // 未打开设置时 401：只弹登录框，不强制打开设置弹窗
    auth.markUnauthorized()
    await flushPromises()
    expect(auth.settingsOpen).toBe(false)
    expect(auth.loginOpen).toBe(true)

    // 已打开设置（storage）时 401：设置保持原 tab，不被强制切到 connection
    auth.openSettings({ tab: 'storage' })
    auth.markUnauthorized()
    await flushPromises()
    expect(auth.settingsOpen).toBe(true)
    expect(auth.settingsTab).toBe('storage')
    expect(dialogEl()).not.toBeNull()
  })
})
