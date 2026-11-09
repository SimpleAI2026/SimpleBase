import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { uiStubs } from '@/test/helpers'
import ConfirmAction from '@/components/ConfirmAction.vue'

const controlledDialog = {
  props: ['open'],
  emits: ['update:open'],
  template: '<div><slot /><button class="ad-open" @click="$emit(\'update:open\', true)">开</button><button class="ad-close" @click="$emit(\'update:open\', false)">关</button></div>'
}

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

  it('controlled mode confirms without also canceling, and cancel closes', async () => {
    const w = mount(ConfirmAction, {
      props: { title: '确认破坏性操作', description: 'database delete --id shop', controlled: true, open: true },
      global: { stubs: { ...uiStubs, AlertDialog: controlledDialog } }
    })
    expect(w.text()).toContain('database delete --id shop')
    await w.get('.ad-ok').trigger('click')
    expect(w.emitted('confirm')).toBeTruthy()
    await w.get('.ad-close').trigger('click')
    expect(w.emitted('cancel')).toBeFalsy()

    const denied = mount(ConfirmAction, {
      props: { title: '确认破坏性操作', description: 'cron trigger', controlled: true, open: true },
      global: { stubs: { ...uiStubs, AlertDialog: controlledDialog } }
    })
    await denied.get('.ad-open').trigger('click')
    await denied.get('.ad-close').trigger('click')
    expect(denied.emitted('cancel')).toBeTruthy()
    expect(denied.emitted('update:open')?.map((args) => args[0])).toEqual([true, false])
  })
})
