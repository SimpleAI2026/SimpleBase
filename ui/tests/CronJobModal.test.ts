import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { useProjectStore } from '@/stores/project'
import { api, resetApiMocks } from '@/test/api-mock'
import { sampleCron, sampleGoFn, uiStubs } from '@/test/helpers'
import CronJobModal from '@/components/modal/CronJobModal.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api, isMock: false }
})

function mountModal(props: Record<string, unknown> = {}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useProjectStore().setProject('dev-shop')
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

  it('associates schedule controls with their labels and help text', async () => {
    const w = mountModal()
    await flushPromises()
    expect(w.get('.sb-modal').attributes('data-max-width')).toBe('720')
    expect(w.get('.overflow-y-auto').classes()).toContain('px-2')
    expect(w.get('label[for="cron-name"]').text()).toBe('任务名')
    expect(w.get('#cron-name').attributes('aria-describedby')).toContain('cron-name-help')
    expect(w.get('label[for="cron-expression"]').text()).toBe('cron 表达式')
    expect(w.get('#cron-expression').attributes('aria-describedby')).toBe('cron-expression-help')
    expect(w.get('[aria-label="常用 cron 预设"]').exists()).toBe(true)
    expect(w.get('[aria-labelledby="cron-file-label"]').exists()).toBe(true)
    const vm = w.vm as any
    vm.form.name = '1invalid'
    vm.form.inputJson = '{bad'
    await flushPromises()
    expect(w.get('#cron-name').attributes('aria-describedby')).toContain('cron-name-error')
    expect(w.get('#cron-input-json').attributes('aria-describedby')).toContain('cron-json-error')
    vm.form.scheduleKind = 'interval'
    await flushPromises()
    expect(w.get('label[for="cron-interval-value"]').exists()).toBe(true)
    expect(w.get('[aria-label="间隔单位"]').exists()).toBe(true)
    vm.form.scheduleKind = 'once'
    await flushPromises()
    expect(w.get('label[for="cron-run-at"]').exists()).toBe(true)
    w.unmount()
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

    const createReset = mountModal({ open: false })
    await createReset.setProps({ open: true })
    await flushPromises()
    expect((createReset.vm as any).form.name).toBe('')
    ;(createReset.vm as any).preset = '0 * * * *'
    await flushPromises()
    expect((createReset.vm as any).form.cronExpr).toBe('0 * * * *')
    createReset.unmount()

    for (const seconds of [86400, 3600, 90]) {
      const pinia = createPinia()
      setActivePinia(pinia)
      useProjectStore().setProject('dev-shop')
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

  it('filters unpublished go functions and validates name format', async () => {
    const unpublished = {
      ...sampleGoFn,
      id: 'gf-2',
      name: 'draft',
      file: 'draft.go',
      activeVersion: 0,
      latestVersion: 1,
      published: false,
      exports: ['Draft']
    }
    api.gofunctions.list.mockResolvedValue([sampleGoFn, unpublished])
    const w = mountModal()
    await flushPromises()
    const vm = w.vm as any
    expect(vm.unpublishedCount).toBe(1)
    expect(vm.availableFunctions.map((g: { name: string }) => g.name)).toEqual(['hello'])
    // 未发布函数不可作目标
    vm.form.name = 'ok-job'
    vm.form.scheduleKind = 'cron'
    vm.form.cronExpr = '0 2 * * *'
    vm.form.funcFile = 'draft'
    vm.form.funcExport = 'Draft'
    expect(vm.canSave).toBe(false)
    // 任务名格式：数字开头 / 中文 → 拦截
    vm.form.funcFile = 'hello'
    vm.form.funcExport = 'Hello'
    expect(vm.canSave).toBe(true)
    vm.form.name = '1bad'
    expect(vm.nameError).toBeTruthy()
    expect(vm.canSave).toBe(false)
    vm.form.name = '每日任务'
    expect(vm.nameError).toBeTruthy()
    expect(vm.canSave).toBe(false)
    vm.form.name = 'nightly-ok'
    expect(vm.nameError).toBe('')
    expect(vm.canSave).toBe(true)
    // 编辑态保留当前目标（即使未发布）
    const edit = mountModal({
      target: { ...sampleCron, funcFile: 'draft', funcExport: 'Draft' }
    })
    await flushPromises()
    const evm = edit.vm as any
    expect(evm.availableFunctions.some((g: { name: string }) => g.name === 'draft')).toBe(true)
    expect(evm.nameError).toBe('')
    edit.unmount()
    w.unmount()
  })
})
