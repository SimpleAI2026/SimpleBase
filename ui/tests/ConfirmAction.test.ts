import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { alertDialogEl, clickAlertDialog, installJsdomPolyfills, realAlertDialogStubs, uiStubs } from '@/test/helpers'
import ConfirmAction from '@/components/ConfirmAction.vue'

installJsdomPolyfills()

/** reka 的 Presence 在关闭时等离场动画结束（jsdom 没有动画，但要多过几个宏任务）才卸载内容。 */
async function waitForClosed() {
  for (let i = 0; i < 20 && alertDialogEl(); i++) {
    await new Promise((resolve) => setTimeout(resolve, 10))
    await flushPromises()
  }
  expect(alertDialogEl()).toBeNull()
}

/** 真实 reka AlertDialog：内容 teleport 到 body，确认 / 取消是真实按钮。 */
const stubs = { ...uiStubs, ...realAlertDialogStubs }

describe('ConfirmAction（触发器模式）', () => {
  it('点触发元素才弹出确认；确认只发 confirm，不发 cancel；确认后弹窗关闭', async () => {
    const w = mount(ConfirmAction, {
      attachTo: document.body,
      props: { title: '删?', description: '不可撤销' },
      slots: { default: '<button class="trigger">del</button>' },
      global: { stubs }
    })
    expect(alertDialogEl()).toBeNull()
    await w.get('.trigger').trigger('click')
    await flushPromises()
    expect(alertDialogEl()!.textContent).toContain('删?')
    expect(alertDialogEl()!.textContent).toContain('不可撤销')

    await clickAlertDialog('确定')
    expect(w.emitted('confirm')).toHaveLength(1)
    expect(w.emitted('cancel')).toBeUndefined()
    expect(w.emitted('update:open')?.map((a) => a[0])).toEqual([true, false])
  })

  it('取消只发 cancel，不发 confirm', async () => {
    const w = mount(ConfirmAction, {
      attachTo: document.body,
      props: { title: '删?' },
      slots: { default: '<button class="trigger">del</button>' },
      global: { stubs }
    })
    await w.get('.trigger').trigger('click')
    await flushPromises()
    await clickAlertDialog('取消')
    expect(w.emitted('cancel')).toBeTruthy()
    expect(w.emitted('confirm')).toBeUndefined()
    expect(w.emitted('update:open')?.map((a) => a[0])).toEqual([true, false])
  })

  it('disabled：只渲染插槽，点击不会弹出确认', async () => {
    const w = mount(ConfirmAction, {
      attachTo: document.body,
      props: { title: '删?', disabled: true },
      slots: { default: '<button class="trigger">del</button>' },
      global: { stubs }
    })
    await w.get('.trigger').trigger('click')
    await flushPromises()
    expect(alertDialogEl()).toBeNull()
  })

  it('取消后再次打开并确认：不会把上一次的取消状态带进来', async () => {
    const w = mount(ConfirmAction, {
      attachTo: document.body,
      props: { title: '删?' },
      slots: { default: '<button class="trigger">del</button>' },
      global: { stubs }
    })
    await w.get('.trigger').trigger('click')
    await flushPromises()
    await clickAlertDialog('取消')
    await w.get('.trigger').trigger('click')
    await flushPromises()
    const cancelsBefore = w.emitted('cancel')!.length
    await clickAlertDialog('确定')
    expect(w.emitted('cancel')).toHaveLength(cancelsBefore)
    expect(w.emitted('confirm')).toHaveLength(1)
  })

  // 回归：「取消」曾经发两次 cancel（AlertDialogCancel 的 @click 一次 + onOpenChange(false) 一次）。
  it('点「取消」只发一次 cancel', async () => {
    const w = mount(ConfirmAction, {
      attachTo: document.body,
      props: { title: '删?' },
      slots: { default: '<button class="trigger">del</button>' },
      global: { stubs }
    })
    await w.get('.trigger').trigger('click')
    await flushPromises()
    await clickAlertDialog('取消')
    expect(w.emitted('cancel')).toHaveLength(1)
  })
})

describe('ConfirmAction（受控模式）', () => {
  // 受控模式下父级负责关闭：用 onUpdate:open 回写 open，才能还原页面里的真实用法。
  function mountControlled(description = 'database delete --id shop') {
    const w = mount(ConfirmAction, {
      attachTo: document.body,
      props: {
        title: '确认破坏性操作',
        description,
        controlled: true,
        open: true,
        'onUpdate:open': (open: boolean) => w.setProps({ open })
      },
      global: { stubs }
    })
    return w
  }

  it('open 控制显示；确认发 confirm 并请求关闭，弹窗随之消失', async () => {
    const w = mountControlled()
    await flushPromises()
    expect(alertDialogEl()!.textContent).toContain('database delete --id shop')
    await clickAlertDialog('确定')
    expect(w.emitted('confirm')).toHaveLength(1)
    expect(w.emitted('update:open')?.at(-1)).toEqual([false])
    await waitForClosed()
  })

  // 回归：受控模式下「确定」同时是 DialogClose，曾先触发 update:open(false) 再发 confirm——
  // 父组件在 update:open 里清空了要确认的目标，confirm 到达时已经无事可做（点「确定」删不掉）。
  // 现在 confirm 必须先于关闭，且不会多发 cancel。
  it('受控模式确认：confirm 先于 update:open(false) 发出，且不发 cancel', async () => {
    const order: string[] = []
    const w = mount(ConfirmAction, {
      attachTo: document.body,
      props: {
        title: 't',
        controlled: true,
        open: true,
        onConfirm: () => order.push('confirm'),
        onCancel: () => order.push('cancel'),
        'onUpdate:open': (open: boolean) => { order.push(`update:open=${open}`); void w.setProps({ open }) }
      },
      global: { stubs }
    })
    await flushPromises()
    await clickAlertDialog('确定')
    await flushPromises()
    expect(order[0]).toBe('confirm')
    expect(order).not.toContain('cancel')
    expect(order).toContain('update:open=false')
  })

  it('取消发 cancel 并请求关闭，弹窗消失；父级重新 open 后可再次出现', async () => {
    const w = mountControlled('cron trigger')
    await flushPromises()
    await clickAlertDialog('取消')
    expect(w.emitted('cancel')).toBeTruthy()
    expect(w.emitted('confirm')).toBeUndefined()
    expect(w.emitted('update:open')?.at(-1)).toEqual([false])
    await waitForClosed()

    await w.setProps({ open: true })
    await flushPromises()
    expect(alertDialogEl()!.textContent).toContain('cron trigger')
  })
})
