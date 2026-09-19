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
