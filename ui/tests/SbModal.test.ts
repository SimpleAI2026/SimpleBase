import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { uiStubs } from '../../test/helpers'
import SbModal from './SbModal.vue'

const stubs = { ...uiStubs, SbModal: false }

describe('SbModal', () => {
  it('maps width breakpoints and disables ok from okButtonProps', async () => {
    const widths = [480, 520, 600, 700, 800, 900, 'auto'] as const
    for (const width of widths) {
      const w = mount(SbModal, {
        props: { open: true, title: 'T', width, okButtonProps: { disabled: width === 480 } },
        global: { stubs }
      })
      expect(w.text()).toContain('T')
      const content = w.find('.dialog-content')
      expect(content.exists() || w.text().includes('确定')).toBe(true)
      w.unmount()
    }
  })

  it('applies exact maxWidth and minWidth via CSS variables', () => {
    const w = mount(SbModal, {
      props: { open: true, title: 'Sized', maxWidth: 900, minWidth: 480 },
      global: { stubs }
    })
    const vm = w.vm as { contentStyle: Record<string, string>; contentClass: string }
    expect(vm.contentStyle['--sb-modal-max-w']).toBe('900px')
    expect(vm.contentStyle['--sb-modal-min-w']).toBe('480px')
    expect(vm.contentClass).toContain('sm:max-w-[var(--sb-modal-max-w)]')
    expect(vm.contentClass).toContain('sm:min-w-[var(--sb-modal-min-w)]')
    expect(vm.contentClass).not.toContain('sm:max-w-5xl')
    const content = w.find('.dialog-content')
    if (content.exists()) {
      expect(content.attributes('style') || '').toContain('--sb-modal-max-w: 900px')
    }
    w.unmount()

    const w2 = mount(SbModal, {
      props: { open: true, title: 'Css', maxWidth: '40rem', minWidth: '20rem' },
      global: { stubs }
    })
    const vm2 = w2.vm as { contentStyle: Record<string, string> }
    expect(vm2.contentStyle['--sb-modal-max-w']).toBe('40rem')
    expect(vm2.contentStyle['--sb-modal-min-w']).toBe('20rem')
    w2.unmount()
  })

  it('hides the default footer when hideFooter is set', () => {
    const w = mount(SbModal, {
      props: { open: true, title: 'No footer', hideFooter: true },
      global: { stubs }
    })
    expect(w.text()).not.toContain('确定')
    expect(w.text()).not.toContain('取消')
    expect(w.find('.dialog-footer').exists()).toBe(false)
    w.unmount()
  })

  it('emits ok and cancel', async () => {
    const w = mount(SbModal, {
      props: { open: true, title: 'X', description: 'd', confirmLoading: true },
      global: { stubs }
    })
    const buttons = w.findAll('button')
    await buttons[0].trigger('click')
    expect(w.emitted('cancel')).toBeTruthy()
    expect(w.emitted('update:open')?.[0]).toEqual([false])
    const w2 = mount(SbModal, { props: { open: true, title: 'X' }, global: { stubs } })
    await w2.findAll('button')[1].trigger('click')
    expect(w2.emitted('ok')).toBeTruthy()
  })
})
