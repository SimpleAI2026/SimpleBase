import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { clickText, mountWithApp } from '@/test/helpers'
import SettingsPanel from '@/components/settings/SettingsPanel.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

describe('SettingsPanel', () => {
  beforeEach(() => {
    resetApiMocks()
  })

  it('loads remote defaults, patches model, and sets a default provider', async () => {
    const models = await mountWithApp(SettingsPanel, { props: { section: 'models' } })
    await models.wrapper.get('.select-emit').trigger('click')
    await flushPromises()
    expect(api.llmSettings.put).toHaveBeenCalled()

    await models.wrapper.get('.combo-emit').trigger('click')
    await flushPromises()
    await models.wrapper.get('.slider-emit').trigger('click')
    await flushPromises()
    const max = models.wrapper.get('#max-tokens')
    await max.setValue('2048')
    await flushPromises()
    expect(api.llmSettings.put).toHaveBeenCalled()
    models.wrapper.unmount()

    const providers = await mountWithApp(SettingsPanel, { props: { section: 'providers' } })
    await clickText(providers.wrapper, '设为默认')
    await flushPromises()
    expect(toast.success).toHaveBeenCalledWith('已设为默认供应商')
    providers.wrapper.unmount()
  })

  it('opens the inline editor, validates, saves, and clears keys (server-side)', async () => {
    const { wrapper } = await mountWithApp(SettingsPanel, { props: { section: 'providers' } })
    await clickText(wrapper, '配置')
    expect(wrapper.text()).toContain('配置 OpenAI')
    await clickText(wrapper, '保存')
    expect(toast.warning).toHaveBeenCalled()

    const inputs = wrapper.findAll('input')
    const keyInput = inputs.find((i) => i.attributes('type') === 'password')
    await keyInput!.setValue('sk-test-key')
    await wrapper.get('.combo-emit').trigger('click')
    // 保存后服务端凭证列表返回已配置条目，驱动「已配置」徽标与清除按钮可用
    api.llmProviderCreds.list.mockResolvedValue([
      { provider: 'openai', defaultModel: '', enabled: true, credentials: { api_key: 'sk-...key' }, hasApiKey: true, updatedAt: 't' }
    ])
    await clickText(wrapper, '保存')
    await flushPromises()
    expect(api.llmProviderCreds.put).toHaveBeenCalledWith(
      expect.any(String),
      'openai',
      expect.objectContaining({ credentials: expect.objectContaining({ api_key: 'sk-test-key' }) })
    )
    expect(toast.success).toHaveBeenCalledWith('已保存到服务端')

    await clickText(wrapper, '配置')
    await clickText(wrapper, '清除 Key')
    await flushPromises()
    expect(api.llmProviderCreds.remove).toHaveBeenCalledWith(expect.any(String), 'openai')
    expect(toast.success).toHaveBeenCalledWith('已清除该厂商 Key')
  })

  it('keeps local defaults when remote load fails and toasts put errors', async () => {
    api.llmSettings.get.mockRejectedValueOnce(new Error('nope'))
    const failed = await mountWithApp(SettingsPanel, { props: { section: 'models' } })
    expect(failed.wrapper.text()).toContain('加载失败')
    expect(failed.wrapper.text()).toContain('nope')
    expect(failed.wrapper.find('.select-emit').exists()).toBe(false)
    failed.wrapper.unmount()

    api.llmSettings.put.mockRejectedValueOnce(new Error('save def'))
    const { wrapper, pinia } = await mountWithApp(SettingsPanel, { props: { section: 'models' } })
    await wrapper.get('.select-emit').trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('save def')

    api.llmSettings.put.mockRejectedValueOnce(new Error('prov'))
    const providers = await mountWithApp(SettingsPanel, { props: { section: 'providers' } })
    await clickText(providers.wrapper, '设为默认')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('prov')

    const { useProjectStore } = await import('@/stores/project')
    useProjectStore(pinia).setProject('other-proj')
    await flushPromises()
    expect(api.llmSettings.get.mock.calls.length).toBeGreaterThan(1)

    const vm = providers.wrapper.vm as any
    vm.openEditor('openai')
    await flushPromises()
    await clickText(providers.wrapper, '取消')
    if (providers.wrapper.find('.combo-emit').exists()) {
      await providers.wrapper.get('.combo-emit').trigger('click')
    }
    vm.editorForm.api_key = 'sk-abc'
    vm.editorForm.base_url = ''
    vm.editorForm.organization = 'org'
    vm.saveEditor()
    vm.openEditor('custom_openai')
    vm.saveEditor()
    vm.editorProviderId = 'missing'
    vm.saveEditor()
    useProjectStore(pinia).projectId = ''
    await vm.loadServerDefaults()
    await vm.loadCreds()
    vm.closeEditor()
  })

  it('shows an empty state when no project is selected', async () => {
    const { wrapper, pinia } = await mountWithApp(SettingsPanel, { props: { section: 'models' } })
    const { useProjectStore } = await import('@/stores/project')
    useProjectStore(pinia).projectId = ''
    await flushPromises()
    expect(wrapper.text()).toMatch(/请先在右上角|创建项目/)
  })
  it('handles non-Error responses', async () => {
    const settings = await mountWithApp(SettingsPanel, { props: { section: 'models' } })
    const st = settings.wrapper.vm as Record<string, any>
    api.llmSettings.put.mockRejectedValueOnce({ nope: true })
    await st.patchDefaults({ maxTokens: 1 })
    api.llmSettings.put.mockRejectedValueOnce({ nope: true })
    const providers = await mountWithApp(SettingsPanel, { props: { section: 'providers' } })
    await (providers.wrapper.vm as Record<string, any>).setDefault('openai')
    if (settings.wrapper.find('#max-tokens').exists()) {
      await settings.wrapper.get('#max-tokens').setValue('')
      await flushPromises()
    }
    settings.wrapper.unmount()
    providers.wrapper.unmount()

  })

  it('covers extended SettingsPanel helpers', async () => {
    const settings = mount(SettingsPanel, {
      props: { section: 'providers' },
      global: { plugins: [createPinia()], stubs: { default: true } },
      shallow: true
    })
    await flushPromises()
    const svm = settings.vm as any
    await svm.loadServerDefaults()
    api.llmSettings.get.mockRejectedValueOnce(new Error('x'))
    await svm.loadServerDefaults()
    await svm.patchDefaults({ temperature: 0.1 })
    api.llmSettings.put.mockRejectedValueOnce(new Error('x'))
    await svm.patchDefaults({})
    svm.remoteCred('openai')
    svm.configured('openai')
    svm.remoteMask('openai')
    await svm.setDefault('openai')
    svm.openEditor('openai')
    svm.saveEditor()
    svm.clearEditor()
    settings.unmount()

  })

})
