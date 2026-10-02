import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { useAuthStore } from '@/stores/auth'
import { useProjectStore } from '@/stores/project'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import ConnectionPanel from '@/components/settings/ConnectionPanel.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api, isMock: false }
})

describe('ConnectionPanel', () => {
  beforeEach(() => {
    resetApiMocks()
  })

  it('covers save/reset/label and settings open wiring', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const auth = useAuthStore()
    const project = useProjectStore()
    project.setProject('dev-shop', '商城')
    auth.openSettings()
    const w = mount(ConnectionPanel, { global: { plugins: [pinia], stubs: uiStubs } })
    await flushPromises()
    const vm = w.vm as any
    expect(vm.projectLabel).toContain('商城')
    vm.key = '  sb_new  '
    vm.saveKey()
    expect(auth.apiKey).toBe('sb_new')
    await w.get('input#api-key').setValue('sb_clicked')
    const buttons = w.findAll('button')
    await buttons[0].trigger('click')
    expect(toast.success).toHaveBeenCalledWith('设置已保存')
    await buttons[1].trigger('click')
    expect(auth.apiKey).toBe('sb_live_dev_key_12345')
    project.projectName = ''
    expect(String(vm.projectLabel)).toBeTruthy()
    project.projectId = ''
    expect(vm.projectLabel).toBe('未选择')
    auth.markUnauthorized()
    await flushPromises()
    expect(vm.unauthorized).toBe(true)
    auth.closeSettings()
    auth.openSettings()
    await flushPromises()
    w.unmount()
  })

  it('签发后一键设为当前使用', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const auth = useAuthStore()
    const project = useProjectStore()
    project.setProject('dev-shop', '商城')
    auth.openSettings()
    const w = mount(ConnectionPanel, { global: { plugins: [pinia], stubs: uiStubs } })
    await flushPromises()
    const vm = w.vm as any
    vm.issuedSecret = 'sb_live_issued_x'
    await flushPromises()
    vm.useIssued()
    expect(vm.key).toBe('sb_live_issued_x')
    expect(auth.apiKey).toBe('sb_live_issued_x')
    expect(toast.success).toHaveBeenCalledWith('已设为当前使用的 Key')
    w.unmount()
  })

  it('签发、吊销、复制与列表失败兜底', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const auth = useAuthStore()
    const project = useProjectStore()
    project.setProject('dev-shop', '商城')
    auth.openSettings()
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
    const w = mount(ConnectionPanel, { global: { plugins: [pinia], stubs: uiStubs } })
    await flushPromises()
    const vm = w.vm as any
    expect(api.apiKeys.list).toHaveBeenCalled()

    await vm.issueKey()
    expect(api.apiKeys.create).toHaveBeenCalledWith('dev-shop')
    expect(vm.issuedSecret).toBe('sb_live_mock_secret')

    await vm.copySecret()
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('sb_live_mock_secret')

    await vm.revokeKey({ id: 'k1', permissions: ['read'] })
    expect(api.apiKeys.revoke).toHaveBeenCalledWith('dev-shop', 'k1')

    api.apiKeys.create.mockRejectedValueOnce(new Error('issue'))
    await vm.issueKey()
    expect(toast.error).toHaveBeenCalledWith('issue')

    api.apiKeys.revoke.mockRejectedValueOnce(new Error('rvk'))
    await vm.revokeKey({ id: 'k2', permissions: [] })
    expect(toast.error).toHaveBeenCalledWith('rvk')

    api.apiKeys.list.mockRejectedValueOnce(new Error('list'))
    await vm.loadKeys()
    expect(vm.keys.length).toBe(0)

    // 无项目时签发/吊销直接返回
    project.projectId = ''
    await vm.issueKey()
    await vm.revokeKey({ id: 'k3', permissions: [] })
    w.unmount()
  })
})
