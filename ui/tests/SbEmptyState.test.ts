import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { InboxIcon } from '@lucide/vue'
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

    const custom = mount(SbEmptyState, {
      props: { icon: InboxIcon, description: '自定义图标' },
      global: { stubs: uiStubs }
    })
    expect(custom.text()).toContain('自定义图标')
    custom.unmount()
  })
})
