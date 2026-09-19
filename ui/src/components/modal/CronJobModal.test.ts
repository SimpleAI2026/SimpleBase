import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { useProjectStore } from '../../stores/project'
import { api, resetApiMocks } from '../../test/api-mock'
import { sampleCron, sampleGoFn, uiStubs } from '../../test/helpers'
import CronJobModal from './CronJobModal.vue'

vi.mock('../../services/api', async () => {
  const m = await import('../../test/api-mock')
  return { api: m.api, isMock: false }
})

function mountModal(props: Record<string, unknown> = {}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useProjectStore().setProject('00000000-0000-0000-0000-000000000002')
  return mount(CronJobModal, {
    props: { open: true, ...props },
    global: { plugins: [pinia], stubs: uiStubs }
  })
}

describe('CronJobModal', () => {
  beforeEach(() => {
    resetApiMocks()
    api.gofunctions.list.mockResolvedValue([sampleGoFn])
  })

  it('covers interval math, validation, create and update', async () => {
    const w = mountModal()
    await flushPromises()
    const vm = w.vm as any
    vm.intervalUnit = 'day'
    vm.intervalValue = 2
    expect(vm.intervalSeconds).toBe(172800)
    vm.intervalUnit = 'hour'
    vm.intervalValue = 3
    expect(vm.intervalSeconds).toBe(10800)
    vm.intervalUnit = 'minute'
    vm.intervalValue = 5
    expect(vm.intervalSeconds).toBe(300)
    vm.intervalValue = 0
    expect(vm.intervalSeconds).toBe(0)
    vm.form.inputJson = '{bad'
    expect(vm.inputJsonError).toBeTruthy()
    vm.form.inputJson = '{"a":1}'
    expect(vm.inputJsonError).toBe('')
    vm.form.name = ''
    expect(vm.canSave).toBe(false)
    vm.form.name = 'n'
    vm.form.scheduleKind = 'cron'
    vm.form.cronExpr = '0 2 *'
    expect(vm.canSave).toBe(false)
    vm.form.cronExpr = '0 2 * * *'
    vm.form.funcFile = 'hello'
    vm.form.funcExport = 'Hello'
    expect(vm.canSave).toBe(true)
    await vm.save()
    expect(api.cronjobs.create).toHaveBeenCalled()

    api.gofunctions.list.mockRejectedValueOnce(new Error('x'))
    await vm.loadFunctions()
    expect(vm.gofunctions).toEqual([])
    w.unmount()

    for (const seconds of [86400, 3600, 90]) {
      const pinia = createPinia()
      setActivePinia(pinia)
      useProjectStore().setProject('00000000-0000-0000-0000-000000000002')
      const e = mount(CronJobModal, {
        props: {
          open: false,
          target: { ...sampleCron, scheduleKind: 'interval', intervalSeconds: seconds, funcFile: 'hello', funcExport: 'Hello' }
        },
        global: { plugins: [pinia], stubs: uiStubs }
      })
      await e.setProps({ open: true })
      await flushPromises()
      expect((e.vm as any).form.scheduleKind).toBe('interval')
      e.unmount()
    }

    const edit = mountModal({ target: { ...sampleCron, funcFile: 'hello', funcExport: 'Hello' } })
    await flushPromises()
    const evm = edit.vm as any
    evm.form.inputJson = '{}'
    evm.form.funcFile = 'hello'
    evm.form.funcExport = 'Hello'
    evm.form.cronExpr = '0 2 * * *'
    evm.form.name = 'nightly'
    await evm.save()
    expect(api.cronjobs.update).toHaveBeenCalled()
    api.cronjobs.update.mockRejectedValueOnce(new Error('upd'))
    await evm.save()
    expect(toast.error).toHaveBeenCalledWith('upd')
    evm.saving = true
    await evm.save()
    edit.unmount()
  })
})
