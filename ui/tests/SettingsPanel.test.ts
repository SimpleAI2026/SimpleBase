import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { clickText, mountWithApp, uiStubs } from '@/test/helpers'
import { useProjectStore } from '@/stores/project'
import ModelDefaultsPanel from '@/components/settings/ModelDefaultsPanel.vue'

/**
 * 设置 → 模型：默认供应商 / API Key / Base URL / 采样参数同属 sys_llm_settings 一行。
 * api_key 只单向写，服务端只回掩码（留空保存即沿用既有值）。
 */
vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

async function mountPanel() {
  const pinia = createPinia()
  setActivePinia(pinia)
  // 面板依赖项目上下文：无默认项目，需显式选择。
  useProjectStore().setProject('testprj1', '测试项目')
  const wrapper = mount(ModelDefaultsPanel, {
    global: { plugins: [pinia], stubs: uiStubs }
  })
  await flushPromises()
  return wrapper
}

describe('ModelDefaultsPanel', () => {
  beforeEach(() => {
    resetApiMocks()
  })

  it('loads remote defaults and patches model parameters', async () => {
    const models = await mountPanel()
    await models.get('.select-emit').trigger('click')
    await flushPromises()
    expect(api.llmSettings.put).toHaveBeenCalled()

    await models.get('.combo-emit').trigger('click')
    await flushPromises()
    await models.get('.slider-emit').trigger('click')
    await flushPromises()
    const max = models.get('#max-tokens')
    await max.setValue('2048')
    await flushPromises()
    expect(api.llmSettings.put).toHaveBeenCalled()
    models.unmount()
  })

  it('saves the API key and clears the input, leaving only the mask', async () => {
    api.llmSettings.put.mockResolvedValueOnce({
      defaultProvider: 'openai',
      defaultModel: 'gpt-4o-mini',
      baseUrl: '',
      apiKeyMasked: 'sk-...7890',
      hasApiKey: true,
      temperature: 0.7,
      maxTokens: 1024
    })
    const wrapper = await mountPanel()
    const key = wrapper.get('#model-api-key')
    await key.setValue('sk-live-1234567890')
    await clickText(wrapper, '保存')
    await flushPromises()
    expect(api.llmSettings.put).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({ defaultProvider: 'openai' }),
      'sk-live-1234567890'
    )
    expect(toast.success).toHaveBeenCalledWith('已保存到服务端')
    // 保存后输入框清空，占位符展示服务端掩码
    expect((key.element as HTMLInputElement).value).toBe('')
    expect(key.attributes('placeholder')).toContain('sk-...7890')
    wrapper.unmount()
  })

  it('surfaces a save failure for the key', async () => {
    const wrapper = await mountPanel()
    api.llmSettings.put.mockRejectedValueOnce(new Error('nope'))
    await wrapper.get('#model-api-key').setValue('sk-x')
    await clickText(wrapper, '保存')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('nope')
    wrapper.unmount()
  })

  it('keeps local defaults when remote load fails and toasts put errors', async () => {
    api.llmSettings.get.mockRejectedValueOnce(new Error('nope'))
    const failed = await mountPanel()
    expect(failed.text()).toContain('加载失败')
    expect(failed.text()).toContain('nope')
    expect(failed.find('.select-emit').exists()).toBe(false)
    failed.unmount()

    api.llmSettings.put.mockRejectedValueOnce(new Error('save def'))
    const models = await mountPanel()
    await models.get('.select-emit').trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('save def')
    models.unmount()
  })

  it('reloads panel data when the project changes', async () => {
    const { wrapper, pinia } = await mountWithApp(ModelDefaultsPanel)
    const before = api.llmSettings.get.mock.calls.length
    const { useProjectStore } = await import('@/stores/project')
    useProjectStore(pinia).setProject('other-proj')
    await flushPromises()
    expect(api.llmSettings.get.mock.calls.length).toBeGreaterThan(before)
    wrapper.unmount()
  })

  it('shows an empty state when no project is selected', async () => {
    const { wrapper, pinia } = await mountWithApp(ModelDefaultsPanel)
    const { useProjectStore } = await import('@/stores/project')
    useProjectStore(pinia).projectId = ''
    await flushPromises()
    expect(wrapper.text()).toMatch(/请先在右上角|创建项目/)
    wrapper.unmount()
  })

  it('handles non-Error responses', async () => {
    const settings = await mountPanel()
    const st = settings.vm as Record<string, any>
    api.llmSettings.put.mockRejectedValueOnce({ nope: true })
    await st.patchDefaults({ maxTokens: 1 })
    if (settings.find('#max-tokens').exists()) {
      await settings.get('#max-tokens').setValue('')
      await flushPromises()
    }
    settings.unmount()
  })

  it('covers extended panel helpers', async () => {
    const models = await mountPanel()
    const mvm = models.vm as any
    await mvm.loadServerDefaults()
    api.llmSettings.get.mockRejectedValueOnce(new Error('x'))
    await mvm.loadServerDefaults()
    await mvm.patchDefaults({ temperature: 0.1 })
    api.llmSettings.put.mockRejectedValueOnce(new Error('x'))
    await mvm.patchDefaults({})
    api.llmSettings.put.mockRejectedValueOnce({ nope: true })
    await mvm.saveConnection()
    models.unmount()
  })
})
