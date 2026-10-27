import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { clickText, mountWithApp, uiStubs } from '@/test/helpers'
import { useAuthStore } from '@/stores/auth'
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

  describe('免费模型卡片', () => {
    it('展示调用 API、脱敏 Key、默认模型与无限 token，并指向使用文档', async () => {
      const { wrapper, pinia } = await mountWithApp(ModelDefaultsPanel)
      useAuthStore(pinia).apiKey = 'sb_live_abcdefghijklmnop'
      await flushPromises()

      const card = wrapper.get('[data-testid="free-model-card"]')
      expect(card.classes()).toContain('bg-success/10')
      expect(card.get('[data-testid="free-model-base-url"]').text()).toBe(
        `${window.location.origin}/v1/projects/testprj1/v1`
      )
      const keyText = card.get('[data-testid="free-model-key"]').text()
      expect(keyText).toContain('sb_live')
      expect(keyText).toContain('mnop')
      expect(keyText).not.toContain('abcdefghijklmnop')
      expect(card.text()).toContain('无限 token')
      expect(card.get('[data-testid="free-model-docs"]').attributes('to')).toBe(
        '/docs/getting-started/openai-compatible'
      )
      wrapper.unmount()
    })

    it('未设置默认模型与 Key 时给出提示，复制 Key 按钮禁用', async () => {
      api.llmSettings.get.mockResolvedValueOnce({ defaultProvider: '', defaultModel: '', temperature: 0.7, maxTokens: 1024 })
      const { wrapper } = await mountWithApp(ModelDefaultsPanel)
      const card = wrapper.get('[data-testid="free-model-card"]')
      expect(card.get('[data-testid="free-model-name"]').text()).toContain('未设置')
      expect(card.get('[data-testid="free-model-key"]').text()).toContain('暂无配置')
      const keyCopy = card.findAll('button').find((b) => b.attributes('aria-label') === '复制 API Key')
      expect(keyCopy?.attributes('disabled')).toBeDefined()
      wrapper.unmount()
    })

    it('复制地址与 Key 走剪贴板并提示，失败时给错误提示', async () => {
      const writeText = vi.fn().mockResolvedValueOnce(undefined).mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error('denied'))
      Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
      const { wrapper, pinia } = await mountWithApp(ModelDefaultsPanel)
      useAuthStore(pinia).apiKey = 'sb_live_abcdefghijklmnop'
      await flushPromises()

      const copyApi = wrapper.findAll('button').find((b) => b.attributes('aria-label') === '复制调用 API 地址')!
      await copyApi.trigger('click')
      await flushPromises()
      expect(writeText).toHaveBeenLastCalledWith(`${window.location.origin}/v1/projects/testprj1/v1`)
      expect(toast.success).toHaveBeenCalledWith('调用 API已复制')

      const copyKey = wrapper.findAll('button').find((b) => b.attributes('aria-label') === '复制 API Key')!
      await copyKey.trigger('click')
      await flushPromises()
      expect(writeText).toHaveBeenLastCalledWith('sb_live_abcdefghijklmnop')

      await copyKey.trigger('click')
      await flushPromises()
      expect(toast.error).toHaveBeenCalledWith('无法复制到剪贴板')
      wrapper.unmount()
    })

    it('点击文档链接会关闭设置弹窗，避免遮住文档页', async () => {
      const { wrapper, pinia } = await mountWithApp(ModelDefaultsPanel)
      const auth = useAuthStore(pinia)
      auth.openSettings({ tab: 'models' })
      await wrapper.get('[data-testid="free-model-docs"]').trigger('click')
      expect(auth.settingsOpen).toBe(false)
      wrapper.unmount()
    })

    it('无项目时不渲染卡片', async () => {
      const { wrapper, pinia } = await mountWithApp(ModelDefaultsPanel)
      useProjectStore(pinia).projectId = ''
      await flushPromises()
      expect(wrapper.find('[data-testid="free-model-card"]').exists()).toBe(false)
      wrapper.unmount()
    })
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
