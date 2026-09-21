import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { useAuthStore } from '../../stores/auth'
import { useProjectStore } from '../../stores/project'
import { uiStubs } from '../../test/helpers'
import ConnectionPanel from './ConnectionPanel.vue'

vi.mock('../../services/http', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../services/http')>()
  return {
    ...actual,
    getApiKey: () => actual.getApiKey(),
    setApiKey: actual.setApiKey
  }
})

describe('ConnectionPanel', () => {
  it('covers save/reset/label and settings-open key reload', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const auth = useAuthStore()
    const project = useProjectStore()
    project.setProject('00000000-0000-0000-0000-000000000002', '商城')
    auth.openSettings('connection')
    const w = mount(ConnectionPanel, { global: { plugins: [pinia], stubs: uiStubs } })
    await flushPromises()
    const vm = w.vm as any
    expect(vm.projectLabel).toContain('商城')
    vm.key = '  sb_new  '
    vm.saveKey()
    expect(auth.apiKey).toBe('sb_new')
    vm.saveAll()
    expect(toast.success).toHaveBeenCalledWith('设置已保存')
    vm.resetKey()
    expect(auth.apiKey).toBe('sb_live_dev_key_12345')
    auth.closeSettings()
    auth.openSettings()
    await flushPromises()
    project.projectName = ''
    expect(String(vm.projectLabel)).toBeTruthy()
    project.projectId = ''
    expect(vm.projectLabel).toBe('未选择')
    auth.markUnauthorized()
    await flushPromises()
    expect(vm.unauthorized).toBe(true)
    w.unmount()
  })
})
