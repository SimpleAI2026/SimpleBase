import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { installJsdomPolyfills } from '@/test/helpers'
import SbModal from '@/components/modal/SbModal.vue'

installJsdomPolyfills()

/**
 * SbModal 用真实 reka Dialog：内容 teleport 到 body，断言打在真实 DOM 属性上
 * （role / aria-* / data-slot / 内联样式变量 / 按钮事件），而不是桩的 class。
 */
const stubs = { teleport: false }

function mountModal(props: Record<string, unknown>, slots: Record<string, string> = {}) {
  return mount(SbModal, { attachTo: document.body, props: { open: true, ...props } as never, slots, global: { stubs } })
}

const dialog = () => document.body.querySelector<HTMLElement>('[role="dialog"]')
const slot = (name: string) => document.body.querySelector<HTMLElement>(`[data-slot="${name}"]`)
const buttons = () => Array.from(document.body.querySelectorAll<HTMLElement>('button'))
const buttonByText = (text: string) => buttons().find((b) => b.textContent?.trim() === text)

describe('SbModal', () => {
  it('open=false 不渲染；open=true 渲染带标题的对话框', async () => {
    const closed = mountModal({ open: false, title: 'T' })
    await flushPromises()
    expect(dialog()).toBeNull()
    closed.unmount()

    const w = mountModal({ title: '标题' })
    await flushPromises()
    expect(dialog()).not.toBeNull()
    expect(slot('dialog-title')!.textContent).toBe('标题')
    // 真实 a11y 关系：aria-labelledby 指向标题节点
    expect(dialog()!.getAttribute('aria-labelledby')).toBe(slot('dialog-title')!.id)
    w.unmount()
  })

  it('maxWidth / minWidth 通过 CSS 变量下发：数字按 px，字符串原样', async () => {
    const a = mountModal({ title: 'Sized', maxWidth: 900, minWidth: 480 })
    await flushPromises()
    const style = dialog()!.getAttribute('style') || ''
    expect(style).toContain('--sb-modal-max-w: 900px')
    expect(style).toContain('--sb-modal-min-w: 480px')
    const cls = dialog()!.getAttribute('class') || ''
    expect(cls).toContain('sm:max-w-[var(--sb-modal-max-w)]')
    expect(cls).toContain('sm:min-w-[var(--sb-modal-min-w)]')
    expect(cls).not.toContain('sm:max-w-5xl')
    a.unmount()

    const b = mountModal({ title: 'Css', maxWidth: '40rem', minWidth: '20rem' })
    await flushPromises()
    const styleB = dialog()!.getAttribute('style') || ''
    expect(styleB).toContain('--sb-modal-max-w: 40rem')
    expect(styleB).toContain('--sb-modal-min-w: 20rem')
    b.unmount()
  })

  it('width 的各档位都能渲染，okButtonProps.disabled 作用在「确定」上', async () => {
    for (const width of [480, 520, 600, 700, 800, 900, 'auto'] as const) {
      const w = mountModal({ title: 'T', width, okButtonProps: { disabled: width === 480 } })
      await flushPromises()
      expect(dialog(), String(width)).not.toBeNull()
      expect(buttonByText('确定')!.hasAttribute('disabled'), String(width)).toBe(width === 480)
      w.unmount()
    }
  })

  it('始终渲染 description 供 aria-describedby；没有描述时用标题兜底并视觉隐藏', async () => {
    const withDesc = mountModal({ title: 'T', description: 'D' })
    await flushPromises()
    const describedBy = dialog()!.getAttribute('aria-describedby')!
    expect(document.getElementById(describedBy)!.textContent).toBe('D')
    expect(document.getElementById(describedBy)!.classList.contains('sr-only')).toBe(false)
    withDesc.unmount()

    const withoutDesc = mountModal({ title: 'T' })
    await flushPromises()
    const el = document.getElementById(dialog()!.getAttribute('aria-describedby')!)!
    expect(el.textContent).toBe('T')
    expect(el.classList.contains('sr-only')).toBe(true)
    withoutDesc.unmount()
  })

  it('hideFooter 时不渲染底栏与默认按钮', async () => {
    const w = mountModal({ title: 'No footer', hideFooter: true })
    await flushPromises()
    expect(slot('dialog-footer')).toBeNull()
    expect(buttonByText('确定')).toBeUndefined()
    expect(buttonByText('取消')).toBeUndefined()
    w.unmount()
  })

  it('「确定」发 ok；「取消」发 cancel 并请求关闭', async () => {
    const w = mountModal({ title: 'X', description: 'd' })
    await flushPromises()
    buttonByText('确定')!.click()
    await flushPromises()
    expect(w.emitted('ok')).toHaveLength(1)
    expect(w.emitted('cancel')).toBeUndefined()

    buttonByText('取消')!.click()
    await flushPromises()
    expect(w.emitted('cancel')).toHaveLength(1)
    expect(w.emitted('update:open')?.at(-1)).toEqual([false])
    w.unmount()
  })

  it('右上角关闭按钮只请求关闭，不发 cancel', async () => {
    const w = mountModal({ title: 'Y' })
    await flushPromises()
    slot('dialog-close')!.click()
    await flushPromises()
    expect(w.emitted('update:open')?.at(-1)).toEqual([false])
    expect(w.emitted('cancel')).toBeUndefined()
    w.unmount()
  })

  it('confirmLoading 时「确定」前面显示加载指示', async () => {
    const w = mountModal({ title: 'X', confirmLoading: true })
    await flushPromises()
    expect(buttonByText('确定')!.querySelector('svg, [data-slot="spinner"]')).not.toBeNull()
    w.unmount()
  })

  it('视口内限高，只让 body 滚动：标题与底栏留在视口里', async () => {
    const w = mountModal(
      { title: '表数据', description: '只读', maxWidth: 900, minWidth: 480 },
      { default: '<p class="tall-row">row</p>' }
    )
    await flushPromises()
    const cls = dialog()!.getAttribute('class') || ''
    expect(cls).toContain('max-h-[calc(100vh-2rem)]')
    expect(cls).toContain('flex')
    expect(cls).toContain('flex-col')

    const body = slot('sb-modal-body')!
    expect(Array.from(body.classList)).toEqual(expect.arrayContaining(['min-h-0', 'min-w-0', 'overflow-y-auto', 'overflow-x-hidden']))
    // 给 overflow 裁切下的焦点环让出空间（padding 与负 margin 抵消，视觉位置不变）
    expect(Array.from(body.classList)).toEqual(expect.arrayContaining(['px-1', '-mx-1']))
    expect(body.querySelector('.tall-row')).not.toBeNull()
    // 标题与描述在 header，不在滚动 body 里
    expect(body.textContent).not.toContain('表数据')
    expect(body.textContent).not.toContain('只读')
    expect(slot('dialog-header')!.textContent).toContain('表数据')
    expect(Array.from(slot('dialog-footer')!.classList)).toContain('shrink-0')
    expect(slot('dialog-footer')!.textContent).toContain('确定')
    w.unmount()
  })

  it('footer 插槽可整体替换默认按钮', async () => {
    const w = mountModal({ title: 'X' }, { footer: '<button class="custom">自定义</button>' })
    await flushPromises()
    expect(document.body.querySelector('.custom')).not.toBeNull()
    expect(buttonByText('确定')).toBeUndefined()
    w.unmount()
  })
})
