import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '../../test/api-mock'
import { uiStubs } from '../../test/helpers'
import CreateProjectModal from './CreateProjectModal.vue'

vi.mock('../../services/api', async () => {
  const m = await import('../../test/api-mock')
  return { api: m.api, isMock: false }
})

describe('CreateProjectModal', () => {
  beforeEach(() => resetApiMocks())

  it('validates name/id and creates a project', async () => {
    const w = mount(CreateProjectModal, {
      props: { open: true },
      global: { stubs: uiStubs }
    })
    await w.get('.sb-ok').trigger('click')
    expect(toast.warning).toHaveBeenCalledWith('请输入项目名称')

    await w.get('#project-name').setValue('bad/name')
    expect(w.text()).toContain('路径分隔符')
    await w.get('#project-name').setValue('ok')
    await w.get('#project-id').setValue('not-uuid')
    expect(w.text()).toContain('须为合法 UUID')
    await w.get('#project-id').setValue('00000000-0000-0000-0000-000000000123')
    await w.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(api.projects.create).toHaveBeenCalled()
    expect(w.emitted('created')).toBeTruthy()

    api.projects.create.mockRejectedValueOnce(new Error('dup'))
    await w.setProps({ open: false })
    await w.setProps({ open: true })
    await w.get('#project-name').setValue('again')
    await w.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('dup')
  })
})
