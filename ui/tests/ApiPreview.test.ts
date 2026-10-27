import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ApiPreview from '@/pages/ApiPreview.vue'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'

vi.mock('@/services/api', async () => {
  const module = await import('@/test/api-mock')
  return { api: module.api }
})

const document = {
  openapi: '3.0.3',
  paths: {
    '/health/live': {
      get: { operationId: 'live', 'x-tryable': true }
    },
    '/v1/projects/{projectID}/documents': {
      post: {
        operationId: 'create',
        'x-tryable': true,
        parameters: [{ name: 'projectID', in: 'path', required: true }],
        requestBody: { required: true, content: { 'application/json': {} } }
      }
    }
  }
}

describe('ApiPreview', () => {
  beforeEach(() => {
    resetApiMocks()
    api.preview.document.mockResolvedValue(document)
    api.preview.request.mockResolvedValue({ status: 200, body: '{"ok":true}' })
  })

  it('loads routes, validates parameters and runs GET without confirmation', async () => {
    const wrapper = mount(ApiPreview, { global: { stubs: { ...uiStubs, ConfirmAction: false } } })
    await flushPromises()
    expect(wrapper.text()).toContain('/health/live')
    expect(wrapper.text()).toContain('$SIMPLEBASE_TOKEN')
    const post = wrapper.findAll('button').find((button) => button.text().includes('POST'))!
    await post.trigger('click')
    const run = wrapper.findAll('button').find((button) => button.text() === '发送请求')!
    await run.trigger('click')
    expect(api.preview.request).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('请填写路径参数 projectID')
    await wrapper.get('input[placeholder="projectID"]').setValue('p1')
    await wrapper.get('textarea').setValue('{"name":"Ada"}')
    await run.trigger('click')
    expect(api.preview.request).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('确认执行写请求')
    await wrapper.get('.ad-ok').trigger('click')
    await flushPromises()
    expect(api.preview.request).toHaveBeenCalledWith('POST', '/v1/projects/p1/documents', { name: 'Ada' })

    const get = wrapper.findAll('button').find((button) => button.text().includes('GET'))!
    await get.trigger('click')
    await wrapper.findAll('button').find((button) => button.text() === '发送请求')!.trigger('click')
    await flushPromises()
    expect(api.preview.request).toHaveBeenCalledWith('GET', '/health/live', undefined)
    expect(wrapper.text()).toContain('200')
  })

  it('handles load errors, invalid JSON and request failures', async () => {
    api.preview.document.mockRejectedValueOnce(new Error('offline'))
    const failed = mount(ApiPreview, { global: { stubs: uiStubs } })
    await flushPromises()
    expect(failed.text()).toContain('offline')
    api.preview.document.mockResolvedValue(document)
    await failed.findAll('button').find((button) => button.text() === '重试')!.trigger('click')
    await flushPromises()

    const wrapper = mount(ApiPreview, { global: { stubs: { ...uiStubs, ConfirmAction: false } } })
    await flushPromises()
    await wrapper.findAll('button').find((button) => button.text().includes('POST'))!.trigger('click')
    await wrapper.get('input[placeholder="搜索端点"]').setValue('documents')
    await wrapper.get('input[placeholder="limit=20&offset=0"]').setValue('limit=1')
    await wrapper.get('input[placeholder="projectID"]').setValue('p1')
    await wrapper.get('textarea').setValue('{bad')
    await wrapper.findAll('button').find((button) => button.text() === '发送请求')!.trigger('click')
    expect(wrapper.text()).toContain('请求体必须是合法 JSON')
    await wrapper.get('textarea').setValue('{}')
    api.preview.request.mockRejectedValueOnce(new Error('denied'))
    await wrapper.findAll('button').find((button) => button.text() === '发送请求')!.trigger('click')
    await wrapper.get('.ad-ok').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('denied')
  })

  it('handles empty documents and non-tryable endpoints', async () => {
    api.preview.document.mockResolvedValueOnce({ openapi: '3.0.3', paths: {} })
    const empty = mount(ApiPreview, { global: { stubs: uiStubs } })
    await flushPromises()
    expect(empty.text()).not.toContain('发送请求')
    await (empty.vm as unknown as { runRequest: () => Promise<void> }).runRequest()
    api.preview.document.mockResolvedValue({ openapi: '3.0.3', paths: { '/v1/projects/{projectID}/realtime': { get: { operationId: 'rt', 'x-tryable': false, parameters: [{ name: 'projectID', in: 'path', required: true }] } } } })
    const wrapper = mount(ApiPreview, { global: { stubs: uiStubs } })
    await flushPromises()
    const vm = wrapper.vm as unknown as { requestRun: () => void; runRequest: () => Promise<void> }
    vm.requestRun()
    expect(api.preview.request).not.toHaveBeenCalled()
  })
})
