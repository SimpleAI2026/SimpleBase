import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs, cancelModal, inDialog, installJsdomPolyfills, modalOkDisabled, mountDialog, okModal, realDialogStubs } from '@/test/helpers'
import { Select } from '@/components/ui/select'
import CreateProjectModal from '@/components/modal/CreateProjectModal.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

installJsdomPolyfills()

describe('CreateProjectModal', () => {
  beforeEach(() => resetApiMocks())

  it('validates name/id and creates a project', async () => {
    const w = await mountDialog(CreateProjectModal, {
      props: { open: true },
      global: { stubs: { ...uiStubs, ...realDialogStubs } }
    })
    // 名称为空时「确定」是禁用的（真实用户点不了）；校验提示只能由回车触发。
    expect(modalOkDisabled()).toBe(true)
    await inDialog().get('#project-name').trigger('keydown.enter')
    expect(toast.warning).toHaveBeenCalledWith('请输入项目名称')

    await inDialog().get('#project-name').setValue('bad/name')
    expect(inDialog().text()).toContain('路径分隔符')
    await inDialog().get('#project-name').setValue('ok')
    await inDialog().get('#project-id').setValue('bad-id!')
    expect(inDialog().text()).toContain('8 位')
    // 非法名称 / ID 同样禁用「确定」
    expect(modalOkDisabled()).toBe(true)
    await inDialog().get('#project-id').setValue('proj-001')
    expect(modalOkDisabled()).toBe(false)
    await okModal()
    await flushPromises()
    expect(api.projects.create).toHaveBeenCalled()
    expect(w.emitted('created')).toBeTruthy()

    api.projects.create.mockRejectedValueOnce(new Error('dup'))
    await w.setProps({ open: false })
    await w.setProps({ open: true })
    await inDialog().get('#project-name').setValue('again')
    await okModal()
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('dup')
  })
  it('引擎下拉默认 DuckDB，选择 SQLite 后随创建请求提交；重新打开恢复默认', async () => {
    const w = await mountDialog(CreateProjectModal, { props: { open: true }, global: { stubs: { ...uiStubs, ...realDialogStubs, Select: false, SelectTrigger: false, SelectValue: false, SelectContent: false, SelectItem: false } } })
    const engine = w.getComponent(Select)
    expect(engine.props('modelValue')).toBe('duckdb')
    expect(inDialog().find('[data-project-engine]').exists()).toBe(true)
    engine.vm.$emit('update:modelValue', 'sqlite')
    await inDialog().get('#project-name').setValue('p')
    await okModal()
    await flushPromises()
    expect(api.projects.create).toHaveBeenCalledWith({ name: 'p', id: undefined, engine: 'sqlite' })

    await w.setProps({ open: false })
    await w.setProps({ open: true })
    expect(w.getComponent(Select).props('modelValue')).toBe('duckdb')
    w.unmount()
  })

  it('closes on cancel', async () => {
    const w = await mountDialog(CreateProjectModal, { props: { open: true }, global: { stubs: { ...uiStubs, ...realDialogStubs } } })
    await cancelModal()
    expect(w.emitted('update:open')).toBeTruthy()
    w.unmount()
  })

})
