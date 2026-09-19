import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { useAuthStore } from '../stores/auth'
import { useProjectStore } from '../stores/project'
import { uiStubs } from '../test/helpers'
import ApiKeyDrawer from './ApiKeyDrawer.vue'

vi.mock('../services/http', () => ({
  getApiKey: () => 'sb_from_http',
  baseURL: ''
}))

describe('ApiKeyDrawer', () => {
  it('covers save/reset/label and drawer open wiring', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const auth = useAuthStore()
    const project = useProjectStore()
    project.setProject('00000000-0000-0000-0000-000000000002', '商城')
    auth.openDrawer()
    const w = mount(ApiKeyDrawer, { global: { plugins: [pinia], stubs: uiStubs } })
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
    vm.onOpen(true)
    vm.onOpen(false)
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
