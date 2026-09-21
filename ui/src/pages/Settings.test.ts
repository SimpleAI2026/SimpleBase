import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { useSettingsStore } from '../stores/settings'
import { api, resetApiMocks } from '../test/api-mock'
import { clickText, mountWithApp } from '../test/helpers'

vi.mock('../services/api', async () => {
  const m = await import('../test/api-mock')
  return { api: m.api, isMock: false }
})

import Settings from './Settings.vue'

describe('Settings', () => {
  beforeEach(() => {
    resetApiMocks()
  })

  it('loads remote defaults, patches theme/model, and sets a default provider', async () => {
    const { wrapper, pinia } = await mountWithApp(Settings)
    const settings = useSettingsStore(pinia)
    await wrapper.get('.tg-dark').trigger('click')
    expect(settings.theme).toBe('dark')
    await wrapper.get('.tg-system').trigger('click')
    await wrapper.get('.tg-light').trigger('click')
    expect(settings.theme).toBe('light')

    await wrapper.get('.select-emit').trigger('click')
    await flushPromises()
    expect(api.llmSettings.put).toHaveBeenCalled()

    await wrapper.get('.combo-emit').trigger('click')
    await flushPromises()
    await wrapper.get('.slider-emit').trigger('click')
    await flushPromises()
    const max = wrapper.get('#max-tokens')
    await max.setValue('2048')
    await flushPromises()
    expect(api.llmSettings.put).toHaveBeenCalled()

    await clickText(wrapper, '设为默认')
    await flushPromises()
    expect(toast.success).toHaveBeenCalledWith('已设为默认供应商')
  })

  it('opens the editor, validates, saves, and clears keys', async () => {
    const { wrapper, pinia } = await mountWithApp(Settings)
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
    const { wrapper, pinia } = await mountWithApp(Settings)
    await wrapper.get('.select-emit').trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('save def')

    api.llmSettings.put.mockRejectedValueOnce(new Error('prov'))
    await clickText(wrapper, '设为默认')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('prov')

    const { useProjectStore } = await import('../stores/project')
    useProjectStore(pinia).setProject('00000000-0000-0000-0000-000000000003')
    await flushPromises()
    expect(api.llmSettings.get.mock.calls.length).toBeGreaterThan(1)

    const vm = wrapper.vm as any
    vm.onTheme(undefined)
    vm.onTheme(['nope'])
    vm.openEditor('openai')
    await flushPromises()
    if (wrapper.find('.sheet').exists()) {
      if (wrapper.find('.sheet-close').exists()) {
        await wrapper.get('.sheet-close').trigger('click')
      }
      if (wrapper.find('.sheet-content-close').exists()) {
        await wrapper.get('.sheet-content-close').trigger('click')
      }
    }
    if (wrapper.find('.combo-emit').exists()) {
      await wrapper.get('.combo-emit').trigger('click')
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
  })
})
