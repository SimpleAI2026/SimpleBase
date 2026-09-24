import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks } from '../../test/api-mock'
import { mountWithApp } from '../../test/helpers'

vi.mock('../../services/api', async () => {
  const m = await import('../../test/api-mock')
  return { api: m.api, isMock: false }
})

import GoFuncTestModal from './GoFuncTestModal.vue'
import GoFuncVersionsModal from './GoFuncVersionsModal.vue'

const record = {
  id: 'gf-1',
  name: 'hello',
  file: 'hello.go',
  description: '',
  activeVersion: 1,
  latestVersion: 2,
  published: true,
  exports: ['Hello', 'Ping'],
  createdAt: 't',
  updatedAt: 't'
}

describe('GoFuncTestModal / GoFuncVersionsModal', () => {
  beforeEach(() => {
    resetApiMocks()
  })

  it('test modal loads versions and sends request', async () => {
    api.gofunctions.listVersions.mockResolvedValue({
      activeVersion: 1,
      versions: [
        { version: 2, exports: ['Ping'], note: 'x', createdAt: 't', active: false },
        { version: 1, exports: ['Hello', 'Ping'], note: '', createdAt: 't', active: true }
      ]
    })
    api.gofunctions.test.mockResolvedValue({
      ok: true,
      statusCode: 200,
      durationMs: 8,
      version: 1,
      activeVersion: 1,
      functionName: 'Hello',
      data: { message: 'hi' },
      error: ''
    })
    const { wrapper } = await mountWithApp(GoFuncTestModal, {
      props: { open: true, record }
    })
    await flushPromises()
    expect(api.gofunctions.listVersions).toHaveBeenCalled()
    expect(wrapper.text()).toContain('版本')
    expect(wrapper.text()).toContain('导出函数')
    const send = wrapper.findAll('button').find((b) => b.text().includes('发送请求'))
    await send!.trigger('click')
    await flushPromises()
    expect(api.gofunctions.test).toHaveBeenCalled()
    expect(wrapper.text()).toContain('200')
    expect(wrapper.text()).toContain('hi')
  })

  it('test modal rejects invalid JSON and validates body', async () => {
    const { wrapper } = await mountWithApp(GoFuncTestModal, {
      props: { open: true, record }
    })
    await flushPromises()
    const ta = wrapper.find('textarea')
    await ta.setValue('{bad')
    const send = wrapper.findAll('button').find((b) => b.text().includes('发送请求'))
    await send!.trigger('click')
    await flushPromises()
    expect(api.gofunctions.test).not.toHaveBeenCalled()
    const validate = wrapper.findAll('button').find((b) => b.text().includes('校验'))
    await validate!.trigger('click')
    await flushPromises()
  })

  it('test modal copies invoke URL', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    const { wrapper } = await mountWithApp(GoFuncTestModal, {
      props: { open: true, record }
    })
    await flushPromises()
    const copy = wrapper.findAll('button').find((b) => b.text().includes('复制'))
    await copy!.trigger('click')
    await flushPromises()
    expect(writeText).toHaveBeenCalled()
  })

  it('versions modal lists and activates', async () => {
    api.gofunctions.listVersions.mockResolvedValue({
      activeVersion: 1,
      versions: [
        { version: 2, exports: ['Ping'], note: '', createdAt: 't', active: false },
        { version: 1, exports: ['Hello'], note: '', createdAt: 't', active: true }
      ]
    })
    api.gofunctions.activate.mockResolvedValue({ activeVersion: 2 })
    const { wrapper } = await mountWithApp(GoFuncVersionsModal, {
      props: { open: true, record }
    })
    await flushPromises()
    expect(wrapper.text()).toContain('v1')
    expect(wrapper.text()).toContain('生效')
    const activateBtn = wrapper.findAll('button').find((b) => b.text().includes('设为生效'))
    await activateBtn!.trigger('click')
    await flushPromises()
    expect(api.gofunctions.activate).toHaveBeenCalled()
  })

  it('test modal switches version and function', async () => {
    api.gofunctions.listVersions.mockResolvedValue({
      activeVersion: 1,
      versions: [
        { version: 2, exports: ['Ping'], note: 'x', createdAt: 't', active: false },
        { version: 1, exports: ['Hello'], note: '', createdAt: 't', active: true }
      ]
    })
    const { wrapper } = await mountWithApp(GoFuncTestModal, {
      props: { open: true, record }
    })
    await flushPromises()
    // 点 v2
    const v2 = wrapper.findAll('button').find((b) => b.text().includes('v2'))
    await v2!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('导出函数（v2）')
    // 点导出 Ping
    const ping = wrapper.findAll('button').find((b) => b.text().trim() === 'Ping')
    await ping!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('hello/Ping')
  })

  it('test modal shows error payload when ok=false', async () => {
    api.gofunctions.test.mockResolvedValue({
      ok: false,
      statusCode: 500,
      durationMs: 3,
      version: 1,
      activeVersion: 1,
      functionName: 'Hello',
      error: 'boom'
    })
    const { wrapper } = await mountWithApp(GoFuncTestModal, {
      props: { open: true, record }
    })
    await flushPromises()
    const send = wrapper.findAll('button').find((b) => b.text().includes('发送请求'))
    await send!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('执行失败')
    expect(wrapper.text()).toContain('boom')
  })

  it('test modal falls back to record versions and draft persistence', async () => {
    api.gofunctions.listVersions.mockRejectedValueOnce(new Error('x'))
    const { wrapper } = await mountWithApp(GoFuncTestModal, {
      props: {
        open: true,
        record: {
          ...record,
          versions: [{ version: 3, exports: ['Zed'], note: '', createdAt: 't', active: true }]
        }
      }
    })
    await flushPromises()
    expect(wrapper.text()).toContain('Zed')
    const ta = wrapper.find('textarea')
    await ta.setValue('{"a":1}')
    const validate = wrapper.findAll('button').find((b) => b.text().includes('校验'))
    await validate!.trigger('click')
    await flushPromises()
  })

  it('test modal maps 4xx status and empty data', async () => {
    api.gofunctions.test.mockResolvedValue({
      ok: false,
      statusCode: 400,
      durationMs: 1,
      version: 1,
      activeVersion: 1,
      functionName: 'Hello',
      error: 'bind fail'
    })
    const { wrapper } = await mountWithApp(GoFuncTestModal, {
      props: { open: true, record }
    })
    await flushPromises()
    const send = wrapper.findAll('button').find((b) => b.text().includes('发送请求'))
    await send!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('400')
    // 成功态 prettyData 分支
    api.gofunctions.test.mockResolvedValue({
      ok: true,
      statusCode: 200,
      durationMs: 1,
      version: 1,
      activeVersion: 1,
      functionName: 'Hello',
      data: null
    })
    await send!.trigger('click')
    await flushPromises()
  })

  it('versions modal handles load/activate/copy errors', async () => {
    api.gofunctions.listVersions.mockRejectedValueOnce(new Error('load fail'))
    const { wrapper: w1 } = await mountWithApp(GoFuncVersionsModal, {
      props: { open: true, record }
    })
    await flushPromises()
    expect(w1.text()).toContain('暂无版本')

    api.gofunctions.listVersions.mockResolvedValue({
      activeVersion: 1,
      versions: [{ version: 1, exports: ['Hello'], note: '', createdAt: 't', active: true }]
    })
    api.gofunctions.activate.mockRejectedValueOnce(new Error('act fail'))
    const { wrapper } = await mountWithApp(GoFuncVersionsModal, {
      props: { open: true, record }
    })
    await flushPromises()
    // 无设为生效（已是 active）→ 点复制源码走 get 失败
    api.gofunctions.get.mockRejectedValueOnce(new Error('src fail'))
    const copy = wrapper.findAll('button').find((b) => b.text().includes('复制源码'))
    await copy!.trigger('click')
    await flushPromises()

    // 未发布版本的设为生效失败
    api.gofunctions.listVersions.mockResolvedValue({
      activeVersion: 1,
      versions: [{ version: 2, exports: ['X'], note: '', createdAt: 't', active: false }]
    })
    const { wrapper: w3 } = await mountWithApp(GoFuncVersionsModal, {
      props: { open: true, record }
    })
    await flushPromises()
    const act = w3.findAll('button').find((b) => b.text().includes('设为生效'))
    await act!.trigger('click')
    await flushPromises()
  })

  it('test modal send network error', async () => {
    api.gofunctions.test.mockRejectedValueOnce(new Error('net'))
    const { wrapper } = await mountWithApp(GoFuncTestModal, {
      props: { open: true, record }
    })
    await flushPromises()
    const send = wrapper.findAll('button').find((b) => b.text().includes('发送请求'))
    await send!.trigger('click')
    await flushPromises()
  })

  it('versions modal copies source', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    api.gofunctions.listVersions.mockResolvedValue({
      activeVersion: 1,
      versions: [{ version: 1, exports: ['Hello'], note: '', createdAt: 't', active: true }]
    })
    api.gofunctions.get.mockResolvedValue({
      ...record,
      source: 'package main',
      versions: [{ version: 1, exports: ['Hello'], note: '', createdAt: 't', active: true, source: 'package main' }]
    })
    const { wrapper } = await mountWithApp(GoFuncVersionsModal, {
      props: { open: true, record }
    })
    await flushPromises()
    const copy = wrapper.findAll('button').find((b) => b.text().includes('复制源码'))
    await copy!.trigger('click')
    await flushPromises()
    expect(writeText).toHaveBeenCalledWith('package main')
  })
})
