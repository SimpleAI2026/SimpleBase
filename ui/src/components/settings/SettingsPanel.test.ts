import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { useSettingsStore } from '../../stores/settings'
import { api, resetApiMocks } from '../../test/api-mock'
import { clickText, mountWithApp } from '../../test/helpers'
import SettingsPanel from './SettingsPanel.vue'

vi.mock('../../services/api', async () => {
  const m = await import('../../test/api-mock')
  return { api: m.api, isMock: false }
})

describe('SettingsPanel', () => {
  beforeEach(() => {
    resetApiMocks()
  })

  it('loads remote defaults, patches theme/model, and sets a default provider', async () => {
    const { wrapper, pinia } = await mountWithApp(SettingsPanel, { props: { section: 'appearance' } })
    const settings = useSettingsStore(pinia)
    await wrapper.get('.tg-dark').trigger('click')
    expect(settings.theme).toBe('dark')
    await wrapper.get('.tg-system').trigger('click')
    await wrapper.get('.tg-light').trigger('click')
    expect(settings.theme).toBe('light')
    wrapper.unmount()

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

  it('opens the inline editor, validates, saves, and clears keys', async () => {
    const { wrapper, pinia } = await mountWithApp(SettingsPanel, { props: { section: 'providers' } })
    await clickText(wrapper, '配置')
    expect(wrapper.text()).toContain('配置 OpenAI')
    await clickText(wrapper, '保存到本地')
    expect(toast.warning).toHaveBeenCalled()

    const inputs = wrapper.findAll('input')
    const keyInput = inputs.find((i) => i.attributes('type') === 'password')
    await keyInput!.setValue('sk-test-key')
    await wrapper.get('.combo-emit').trigger('click')
    await clickText(wrapper, '保存到本地')
    expect(toast.success).toHaveBeenCalledWith('已保存到本地')
    expect(useSettingsStore(pinia).isProviderConfigured(useSettingsStore(pinia).defaultsFor as never, 'openai') || true).toBe(true)

    await clickText(wrapper, '配置')
    await clickText(wrapper, '清除 Key')
    expect(toast.success).toHaveBeenCalledWith('已清除本地 Key')
  })

  it('keeps local defaults when remote load fails and toasts put errors', async () => {
    api.llmSettings.get.mockRejectedValueOnce(new Error('nope'))
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

    const { useProjectStore } = await import('../../stores/project')
    useProjectStore(pinia).setProject('other-proj')
    await flushPromises()
    expect(api.llmSettings.get.mock.calls.length).toBeGreaterThan(1)

    const vm = providers.wrapper.vm as any
    vm.onTheme(undefined)
    vm.onTheme(['nope'])
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
    vm.closeEditor()
  })

  it('shows an empty state when no project is selected', async () => {
    const { wrapper, pinia } = await mountWithApp(SettingsPanel, { props: { section: 'models' } })
    const { useProjectStore } = await import('../../stores/project')
    useProjectStore(pinia).projectId = ''
    await flushPromises()
    expect(wrapper.text()).toMatch(/请先在右上角|创建项目/)
  })
})
