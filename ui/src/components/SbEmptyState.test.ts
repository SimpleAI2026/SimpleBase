import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { uiStubs } from '../test/helpers'
import SbEmptyState from './SbEmptyState.vue'

describe('SbEmptyState', () => {
  it('emits action when the button is clicked', async () => {
    const w = mount(SbEmptyState, {
      props: { title: '空', description: '无', actionText: '新建' },
      global: { stubs: uiStubs }
    })
    expect(w.text()).toContain('空')
    await w.get('button').trigger('click')
    expect(w.emitted('action')).toBeTruthy()
  })
})
