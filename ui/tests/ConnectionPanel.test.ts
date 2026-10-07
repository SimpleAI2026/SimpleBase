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

function setup() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  const project = useProjectStore()
  project.setProject('dev-shop', '商城')
  auth.openSettings()
  const w = mount(ConnectionPanel, { global: { plugins: [pinia], stubs: uiStubs } })
  return { auth, project, w }
}

describe('ConnectionPanel', () => {
  beforeEach(() => {
    resetApiMocks()
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
  })

  it('Key 明文可见，保存与恢复默认', async () => {
    const { auth, project, w } = setup()
    await flushPromises()
    const vm = w.vm as any
    expect(w.get('input#api-key').attributes('type')).toBe('text')
    expect(w.get('input#current-project').element.value).toBe('dev-shop')

    vm.key = '  sb_new  '
    vm.saveKey()
    expect(auth.apiKey).toBe('sb_new')
    await w.get('input#api-key').setValue('sb_clicked')
    const save = w.findAll('button').find((b) => b.text() === '保存')!
    await save.trigger('click')
    expect(toast.success).toHaveBeenCalledWith('设置已保存')
    const restore = w.findAll('button').find((b) => b.text().startsWith('恢复默认'))!
    await restore.trigger('click')
    expect(auth.apiKey).toBe('sb_live_dev_key_12345')

    auth.markUnauthorized()
    await flushPromises()
    expect(vm.unauthorized).toBe(true)
    project.projectId = ''
    await flushPromises()
    expect(w.get('input#current-project').element.value).toBe('未选择')
    auth.closeSettings()
    auth.openSettings()
    await flushPromises()
    w.unmount()
  })

  it('复制项目 ID 与 API Key', async () => {
    const { w } = setup()
    await flushPromises()
    const vm = w.vm as any
    vm.key = 'sb_copy_me'
    await flushPromises()
    const copies = w.findAll('button').filter((b) => b.text() === '复制')
    expect(copies).toHaveLength(2)
    await copies[0].trigger('click')
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('dev-shop')
    await copies[1].trigger('click')
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('sb_copy_me')
    await flushPromises()
    expect(toast.success).toHaveBeenCalledWith('已复制API Key')

    ;(navigator.clipboard.writeText as any).mockRejectedValueOnce(new Error('x'))
    await vm.copyText('t', 'T')
    expect(toast.error).toHaveBeenCalledWith('复制失败')
    w.unmount()
  })

  it('重置：签发新 Key、吊销旧 Key、切换为当前使用', async () => {
    const { auth, w } = setup()
    await flushPromises()
    const vm = w.vm as any
    api.apiKeys.list.mockResolvedValueOnce([
      { id: 'old1', permissions: [] },
      { id: 'old2', permissions: [] }
    ])
    await vm.resetKey()
    expect(api.apiKeys.create).toHaveBeenCalledWith('dev-shop')
    expect(api.apiKeys.revoke).toHaveBeenCalledWith('dev-shop', 'old1')
    expect(api.apiKeys.revoke).toHaveBeenCalledWith('dev-shop', 'old2')
    expect(vm.key).toBe('sb_live_mock_secret')
    expect(auth.apiKey).toBe('sb_live_mock_secret')
    expect(toast.success).toHaveBeenCalledWith('已重置：新 Key 已设为当前使用')
    w.unmount()
  })

  it('重置：列表失败仍签发；吊销失败给出警告；签发失败报错', async () => {
    const { w } = setup()
    await flushPromises()
    const vm = w.vm as any

    api.apiKeys.list.mockRejectedValueOnce(new Error('list'))
    await vm.resetKey()
    expect(api.apiKeys.create).toHaveBeenCalledTimes(1)
    expect(api.apiKeys.revoke).not.toHaveBeenCalled()

    api.apiKeys.list.mockResolvedValueOnce([{ id: 'o', permissions: [] }])
    api.apiKeys.revoke.mockRejectedValueOnce(new Error('rvk'))
    await vm.resetKey()
    expect(toast.warning).toHaveBeenCalled()

    api.apiKeys.create.mockRejectedValueOnce(new Error('issue'))
    await vm.resetKey()
    expect(toast.error).toHaveBeenCalledWith('issue')

    api.apiKeys.create.mockRejectedValueOnce('boom')
    await vm.resetKey()
    expect(toast.error).toHaveBeenCalledWith('重置失败')
    w.unmount()
  })

  it('无项目或只读角色时不可重置', async () => {
    const { auth, project, w } = setup()
    await flushPromises()
    const vm = w.vm as any
    project.projectId = ''
    await vm.resetKey()
    expect(api.apiKeys.create).not.toHaveBeenCalled()
    expect(vm.canReset).toBe(false)

    project.setProject('dev-shop', '商城')
    ;(auth as any).user = { id: 'u', username: 'a', role: 'admin' }
    await flushPromises()
    expect(vm.canReset).toBe(false)
    await vm.resetKey()
    expect(api.apiKeys.create).not.toHaveBeenCalled()
    w.unmount()
  })
  it('ConnectionPanel mounts without a sheet', async () => {
    const { w } = setup()
    expect(w.find('.sheet').exists()).toBe(false)
    expect(w.text()).toContain('API Key')
    w.unmount()
  })

})
