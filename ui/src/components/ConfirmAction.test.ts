import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { uiStubs } from '../test/helpers'
import ConfirmAction from './ConfirmAction.vue'

describe('ConfirmAction enabled', () => {
  it('renders the dialog and emits confirm', async () => {
    const w = mount(ConfirmAction, {
      props: { title: '删?', description: '不可撤销' },
      slots: { default: '<button>del</button>' },
      global: { stubs: uiStubs }
    })
    expect(w.text()).toContain('删?')
    await w.get('.ad-ok').trigger('click')
    expect(w.emitted('confirm')).toBeTruthy()
  })
})
