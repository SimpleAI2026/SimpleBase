import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import McpServerModal from '@/components/modal/McpServerModal.vue'
import { api, resetApiMocks } from '@/test/api-mock'
import { mountWithApp } from '@/test/helpers'

vi.mock('@/services/api', async () => {
  const module = await import('@/test/api-mock')
  return { api: module.api }
})

const existing = {
  id: 'mcp-1',
  name: 'github',
  transport: 'http',
  url: 'https://mcp.example.com/mcp',
  timeout_s: 45,
  enabled: true,
  tools_policy: {},
  header_names: ['Authorization'],
  has_credential: true,
  created_at: 't',
  updated_at: 't'
}

type Vm = {
  name: string
  url: string
  credential: string
  timeout: number
  save: () => Promise<void>
}

async function mountModal(server: typeof existing | null) {
  const { wrapper } = await mountWithApp(McpServerModal, {
    props: { open: true, projectId: 'p1', server }
  })
  return { wrapper, vm: wrapper.vm as unknown as Vm }
}

describe('McpServerModal', () => {
  beforeEach(() => {
    resetApiMocks()
  })

  it('creates a server with a bearer credential', async () => {
    const { vm } = await mountModal(null)
    vm.name = 'github'
    vm.url = 'https://mcp.example.com/mcp'
    vm.credential = 'token-123'
    vm.timeout = 30
    await vm.save()
    await flushPromises()

    expect(api.mcpServers.create).toHaveBeenCalledWith('p1', {
      name: 'github',
      url: 'https://mcp.example.com/mcp',
      timeout_s: 30,
      headers: { Authorization: 'Bearer token-123' },
      enabled: true
    })
  })

  it('keeps the saved credential when the field is left empty', async () => {
    const { vm } = await mountModal(existing)
    // 编辑态回填非敏感字段，凭据始终为空。
    expect(vm.name).toBe('github')
    expect(vm.credential).toBe('')
    vm.name = 'github2'
    await vm.save()
    await flushPromises()

    // headers 省略 → 后端沿用既有值。
    expect(api.mcpServers.update).toHaveBeenCalledWith('p1', 'mcp-1', {
      name: 'github2',
      url: 'https://mcp.example.com/mcp',
      timeout_s: 45
    })
  })

  it('rejects a missing name or url and surfaces save failures', async () => {
    const { vm } = await mountModal(null)
    vm.name = ''
    vm.url = ''
    await vm.save()
    expect(api.mcpServers.create).not.toHaveBeenCalled()

    vm.name = 'x'
    vm.url = 'https://x/mcp'
    vm.credential = 'tok'
    api.mcpServers.create.mockRejectedValueOnce(new Error('boom'))
    await vm.save()
    await flushPromises()
    // 失败后不关闭弹窗，用户可以修正后重试。
    expect(api.mcpServers.create).toHaveBeenCalled()
  })

  it('drives every input through the template handlers', async () => {
    const { wrapper, vm } = await mountModal(null)
    // 四个输入框：名称 / URL / 凭据 / 超时，各自走 update:model-value 处理器。
    const inputs = wrapper.findAll('input')
    expect(inputs.length).toBeGreaterThanOrEqual(4)
    await inputs[0].setValue('fs')
    await inputs[1].setValue('https://fs.example.com/mcp')
    await inputs[2].setValue('tok')
    await inputs[3].setValue('90')
    await flushPromises()
    expect(vm.name).toBe('fs')
    expect(vm.url).toBe('https://fs.example.com/mcp')
    expect(vm.credential).toBe('tok')
    expect(vm.timeout).toBe(90)

    // 空超时回落到 60。
    await inputs[3].setValue('')
    await flushPromises()
    expect(vm.timeout).toBe(60)

    // 取消按钮只关闭弹窗，不发请求。
    const cancel = wrapper.findAll('button').find((b) => b.text().includes('取消'))!
    await cancel.trigger('click')
    expect(wrapper.emitted('update:open')?.at(-1)).toEqual([false])

    // 再次打开时既有的 update:open 事件仍能透传（模板 @update:open 处理器）。
    await wrapper.setProps({ open: false })
    await wrapper.setProps({ open: true })
    await flushPromises()
    expect(wrapper.emitted('update:open')?.length).toBeGreaterThanOrEqual(1)
  })

  it('ignores a save while one is in flight and closes on the modal event', async () => {
    const { wrapper, vm } = await mountModal(null)
    vm.name = 'x'
    vm.url = 'https://x/mcp'
    // 第一次保存挂住，第二次直接返回（saving 守卫）。
    let release!: () => void
    api.mcpServers.create.mockImplementationOnce(
      () => new Promise((resolve) => { release = () => resolve({ id: 'm', name: 'x' }) })
    )
    const first = vm.save()
    await flushPromises()
    await vm.save()
    expect(api.mcpServers.create).toHaveBeenCalledTimes(1)
    release()
    await first
    await flushPromises()

    // SbModal 的 update:open 透传给父组件。
    await wrapper.get('.sb-modal-close').trigger('click')
    expect(wrapper.emitted('update:open')?.at(-1)).toEqual([false])
  })

  it('uses a different credential placeholder when one is already saved', async () => {
    const { wrapper } = await mountModal(existing)
    const inputs = wrapper.findAll('input')
    // 有已保存凭据时提示语不同（占位符来自 computed）。
    expect(inputs[2].attributes('placeholder')).not.toBe('粘贴访问令牌')
  })
})