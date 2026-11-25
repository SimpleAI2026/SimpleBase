import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { useProjectStore } from '@/stores/project'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs, cancelModal, closeDialog, inDialog, installJsdomPolyfills, mountDialog, realDialogStubs } from '@/test/helpers'
import GoFunctionModal from '@/components/modal/GoFunctionModal.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

vi.mock('@/components/editor/GoMonacoEditor.vue', () => ({
  default: { props: ['modelValue'], template: '<textarea class="monaco" />' }
}))

installJsdomPolyfills()

describe('GoFunctionModal', () => {
  beforeEach(() => {
    resetApiMocks()
    setActivePinia(createPinia())
    useProjectStore().setProject('dev-shop')
  })

  it('previews exports and saves create/edit/view paths', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    useProjectStore().setProject('dev-shop')
    const w = await mountDialog(GoFunctionModal, {
      props: { open: true, mode: 'create' },
      global: { plugins: [pinia], stubs: { ...uiStubs, ...realDialogStubs, GoMonacoEditor: true } }
    })
    const sourceCode = inDialog().get('code')
    expect(sourceCode.classes()).toEqual(expect.arrayContaining(['sb-code-view', 'sb-mono']))
    expect(inDialog().find('.max-w-full.overflow-hidden').exists()).toBe(true)
    const vm = w.vm as any
    vm.nameValue = 'hello'
    vm.sourceValue = 'package main\nfunc Hello() {}\nfunc hello() {}\nfunc Ping() {}'
    expect(vm.previewExports).toEqual(['Hello', 'Ping'])
    expect(vm.saveDisabled).toBe(false)
    await vm.save()
    expect(api.gofunctions.create).toHaveBeenCalled()

    api.gofunctions.create.mockRejectedValueOnce(new Error('compile'))
    await vm.save()
    expect(toast.error).toHaveBeenCalledWith('compile')

    await w.setProps({ open: false })
    await w.setProps({ open: true, mode: 'create' })
    expect(vm.nameValue).toBe('')

    await w.setProps({ open: false })
    await w.setProps({ open: true, mode: 'edit', target: { name: 'hello' } })
    await flushPromises()
    expect(api.gofunctions.get).toHaveBeenCalled()
    await vm.save()
    expect(api.gofunctions.saveVersion).toHaveBeenCalled()

    api.gofunctions.get.mockRejectedValueOnce(new Error('src'))
    await w.setProps({ open: false })
    await w.setProps({ open: true, mode: 'edit', target: { name: 'hello' } })
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('加载云函数源码失败')

    await w.setProps({ open: false })
    await w.setProps({ open: true, mode: 'view', target: { name: 'hello', source: 'package main' } })
    expect(vm.title).toContain('查看')
    vm.close()
    expect(w.emitted('update:open')).toBeTruthy()
    w.unmount()
  })
  it('GoFunctionModal name input and modal close', async () => {
    const w = await mountDialog(GoFunctionModal, {
      props: { open: true, mode: 'create' },
      global: { plugins: [createPinia()], stubs: { ...uiStubs, ...realDialogStubs, GoMonacoEditor: true } }
    })
    const name = inDialog().find('input')
    if (name.exists()) await name.setValue('Echo')
    const cancel = inDialog().find('.sb-cancel')
    if (cancel.exists()) await cancel.trigger('click')
    else await inDialog().findAll('button').find((b) => b.text().includes('取消'))?.trigger('click')
    w.unmount()
  })


  it('GoFunctionModal monaco, close emit, and save catch', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    useProjectStore().setProject('dev-shop')
    const w = await mountDialog(GoFunctionModal, {
      props: { open: true, mode: 'create' },
      global: { plugins: [pinia], stubs: { ...uiStubs, ...realDialogStubs } }
    })
    if (inDialog().find('.monaco').exists()) {
      await inDialog().get('.monaco').setValue('func Ping() int { return 1 }')
    }
    const vm = w.vm as Record<string, any>
    api.gofunctions.create.mockRejectedValueOnce('save')
    vm.nameValue = 'Ping'
    await vm.save()
    if (inDialog().find('.sb-cancel').exists()) await cancelModal()
    else await inDialog().findAll('button').find((b) => b.text().includes('取消') || b.text().includes('关闭'))?.trigger('click')
    w.unmount()
  })

  it('modal close 转发 update:open，monaco 输入更新 sourceValue', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    useProjectStore().setProject('dev-shop')
    const w = await mountDialog(GoFunctionModal, {
      props: { open: true, mode: 'create' },
      global: {
        plugins: [pinia],
        stubs: { ...uiStubs, ...realDialogStubs,
          // 带 emit 的编辑器桩，触发模板 v-model 的 onUpdate:modelValue 回调
          GoMonacoEditor: {
            props: ['modelValue', 'readOnly', 'minHeight'],
            emits: ['update:modelValue'],
            template:
              '<textarea class="monaco" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />'
          }
        }
      }
    })
    // SbModal 桩的关闭按钮 → @update:open 转发
    await closeDialog()
    expect(w.emitted('update:open')?.at(-1)).toEqual([false])
    // GoMonacoEditor 桩 v-model → sourceValue
    await inDialog().get('.monaco').setValue('package main\nfunc Echo() {}')
    const vm = w.vm as Record<string, any>
    expect(vm.sourceValue).toContain('func Echo() {}')
    w.unmount()
  })
})
