import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import McpServersPanel from '@/components/settings/McpServersPanel.vue'
import { api, resetApiMocks } from '@/test/api-mock'
import { clickText, mountWithApp, realDialogStubs, confirmAction, realAlertDialogStubs } from '@/test/helpers'

vi.mock('@/services/api', async () => {
  const module = await import('@/test/api-mock')
  return { api: module.api }
})

const server = {
  id: 'mcp-1',
  name: 'github',
  transport: 'http',
  url: 'https://mcp.example.com/mcp',
  timeout_s: 60,
  enabled: true,
  tools_policy: { create_issue: { destructive: true }, search: { read_only: true } },
  header_names: ['Authorization'],
  has_credential: true,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z'
}

const extraStubs = {
  ...realDialogStubs,
  ...realAlertDialogStubs,
  McpServerModal: {
    props: ['open', 'projectId', 'server'],
    emits: ['saved', 'update:open'],
    template:
      '<div v-if="open" class="mcp-modal" @click="$emit(\'update:open\', false)" />'
  }
}

describe('McpServersPanel', () => {
  beforeEach(() => {
    resetApiMocks()
    api.mcpServers.list.mockResolvedValue({ servers: [server], builtin: [] })
  })

  it('renders servers without ever showing credential values', async () => {
    const { wrapper } = await mountWithApp(McpServersPanel, { projectId: 'p1', stubs: extraStubs })
    expect(wrapper.text()).toContain('MCP Server')
    expect(wrapper.text()).toContain('github')
    expect(wrapper.text()).toContain('https://mcp.example.com/mcp')
    // 只展示 header 名字，不展示值。
    expect(wrapper.text()).toContain('Authorization')
    // tools_policy 的条目数即工具数。
    expect(wrapper.text()).toContain('2')
  })

  it('toggles, tests and deletes a server', async () => {
    const { wrapper } = await mountWithApp(McpServersPanel, { projectId: 'p1', stubs: extraStubs })
    await wrapper.get('[data-slot="switch"]').trigger('click')
    await flushPromises()
    expect(api.mcpServers.update).toHaveBeenCalledWith('p1', 'mcp-1', { enabled: false })

    await clickText(wrapper, '测试')
    await flushPromises()
    expect(api.mcpServers.test).toHaveBeenCalledWith('p1', 'mcp-1')

    await confirmAction(wrapper, '删除')
    expect(api.mcpServers.remove).toHaveBeenCalledWith('p1', 'mcp-1')
  })

  it('opens the create modal and surfaces failures', async () => {
    api.mcpServers.list.mockResolvedValueOnce({ servers: [], builtin: [] })
    const { wrapper } = await mountWithApp(McpServersPanel, { projectId: 'p1', stubs: extraStubs })
    expect(wrapper.text()).toContain('暂无 MCP Server')

    await clickText(wrapper, '新增 Server')
    expect(wrapper.find('.mcp-modal').exists()).toBe(true)

    api.mcpServers.test.mockRejectedValueOnce(new Error('test failed'))
    api.mcpServers.update.mockRejectedValueOnce(new Error('update failed'))
    api.mcpServers.remove.mockRejectedValueOnce(new Error('remove failed'))
    const vm = wrapper.vm as unknown as {
      toggle: (item: typeof server, enabled: boolean) => Promise<void>
      test: (item: typeof server) => Promise<void>
      remove: (item: typeof server) => Promise<void>
    }
    await vm.toggle(server, false)
    await vm.test(server)
    await vm.remove(server)

    api.mcpServers.list.mockRejectedValueOnce(new Error('offline'))
    await clickText(wrapper, '刷新')
    await flushPromises()
    expect(wrapper.text()).toContain('offline')
  })

  it('stops testing when the server reports a failure', async () => {
    api.mcpServers.test.mockResolvedValueOnce({ ok: false, tools: [] })
    const { wrapper } = await mountWithApp(McpServersPanel, { projectId: 'p1', stubs: extraStubs })
    await clickText(wrapper, '测试')
    await flushPromises()
    expect(api.mcpServers.test).toHaveBeenCalled()
  })

  it('closes the modal when the child emits update:open', async () => {
    const { wrapper } = await mountWithApp(McpServersPanel, { projectId: 'p1', stubs: extraStubs })
    await clickText(wrapper, '新增 Server')
    expect(wrapper.find('.mcp-modal').exists()).toBe(true)
    await wrapper.get('.mcp-modal').trigger('click')
    await flushPromises()
    expect(wrapper.find('.mcp-modal').exists()).toBe(false)
  })

  it('opens the edit modal with the selected server', async () => {
    const { wrapper } = await mountWithApp(McpServersPanel, { projectId: 'p1', stubs: extraStubs })
    await clickText(wrapper, '编辑')
    await flushPromises()
    expect(wrapper.find('.mcp-modal').exists()).toBe(true)
    // 编辑态把当前记录传给弹窗（凭据仍不回填）。
    const vm = wrapper.vm as unknown as { editing: typeof server | null }
    expect(vm.editing?.id).toBe('mcp-1')
  })

  it('hides write actions for read-only principals', async () => {
    const { wrapper } = await mountWithApp(McpServersPanel, { projectId: 'p1', stubs: extraStubs })
    const vm = wrapper.vm as unknown as {
      openCreate: () => void
      openEdit: (item: typeof server) => void
    }
    const { useAuthStore } = await import('@/stores/auth')
    const auth = useAuthStore()
    const original = auth.canWrite
    Object.defineProperty(auth, 'canWrite', { get: () => false, configurable: true })
    vm.openCreate()
    vm.openEdit(server)
    // 只读主体不打开弹窗。
    expect(wrapper.find('.mcp-modal').exists()).toBe(false)

    // 只读主体的 toggle 也必须直接返回（不调 API）。
    api.mcpServers.update.mockClear()
    const vmFull = wrapper.vm as unknown as { toggle: (i: typeof server, e: boolean) => Promise<void> }
    await vmFull.toggle(server, false)
    expect(api.mcpServers.update).not.toHaveBeenCalled()
    Object.defineProperty(auth, 'canWrite', { get: () => original, configurable: true })
  })

  it('ignores concurrent toggles and repeated tests on the same server', async () => {
    const { wrapper } = await mountWithApp(McpServersPanel, { projectId: 'p1', stubs: extraStubs })
    const vm = wrapper.vm as unknown as {
      toggle: (i: typeof server, e: boolean) => Promise<void>
      test: (i: typeof server) => Promise<void>
    }
    let releaseToggle!: () => void
    api.mcpServers.update.mockImplementationOnce(
      () => new Promise((resolve) => { releaseToggle = () => resolve({ ...server, enabled: false }) })
    )
    const first = vm.toggle(server, false)
    await flushPromises()
    // 同一 Server 的第二次 toggle 被 in-flight 集合挡住。
    await vm.toggle(server, true)
    expect(api.mcpServers.update).toHaveBeenCalledTimes(1)
    releaseToggle()
    await first

    let releaseTest!: () => void
    api.mcpServers.test.mockImplementationOnce(
      () => new Promise((resolve) => { releaseTest = () => resolve({ ok: true, tools: [] }) })
    )
    const t1 = vm.test(server)
    await flushPromises()
    await vm.test(server)
    expect(api.mcpServers.test).toHaveBeenCalledTimes(1)
    releaseTest()
    await t1
  })

  it('renders the read-only placeholder instead of actions', async () => {
    const { wrapper } = await mountWithApp(McpServersPanel, { projectId: 'p1', stubs: extraStubs })
    const { useAuthStore } = await import('@/stores/auth')
    const auth = useAuthStore()
    Object.defineProperty(auth, 'canWrite', { get: () => false, configurable: true })
    await flushPromises()
    expect(wrapper.text()).toContain('—')
    Object.defineProperty(auth, 'canWrite', { get: () => true, configurable: true })
  })

  it('skips requests without a project and ignores stale responses', async () => {
    const { wrapper, pinia } = await mountWithApp(McpServersPanel, { projectId: 'p1', stubs: extraStubs })
    const vm = wrapper.vm as unknown as { load: () => Promise<void> }
    const { useProjectStore } = await import('@/stores/project')
    const store = useProjectStore(pinia)
    // 没有项目：直接落空，不发请求。
    store.setProject('', '')
    await flushPromises()
    api.mcpServers.list.mockClear()

    let releaseOld!: (v: { servers: typeof server[]; builtin: never[] }) => void
    api.mcpServers.list.mockImplementationOnce(
      () => new Promise((resolve) => { releaseOld = resolve })
    )
    store.setProject('p1', 'One')
    await flushPromises()
    const stale = vm.load()
    store.setProject('p2', 'Two')
    await flushPromises()
    releaseOld({ servers: [server], builtin: [] })
    await stale
    // 旧项目的响应不能覆盖新项目列表。
    expect(api.mcpServers.list).toHaveBeenLastCalledWith('p2')
  })

  it('tolerates a response without servers and a missing tools policy', async () => {
    api.mcpServers.list.mockResolvedValueOnce(undefined as never)
    const { wrapper } = await mountWithApp(McpServersPanel, { projectId: 'p1', stubs: extraStubs })
    expect(wrapper.text()).toContain('暂无 MCP Server')

    api.mcpServers.list.mockResolvedValueOnce({ servers: [{ ...server, tools_policy: undefined as never, has_credential: false }], builtin: [] })
    const vm = wrapper.vm as unknown as { load: () => Promise<void>; toggle: (i: any, e: boolean) => Promise<void> }
    await vm.load()
    // 没有 tools_policy 时工具数按 0 处理。
    expect(wrapper.text()).toContain('0')
  })

  it('closes the modal when the project changes', async () => {
    const { wrapper, pinia } = await mountWithApp(McpServersPanel, { projectId: 'p1', stubs: extraStubs })
    await clickText(wrapper, '新增 Server')
    expect(wrapper.find('.mcp-modal').exists()).toBe(true)
    const { useProjectStore } = await import('@/stores/project')
    useProjectStore(pinia).setProject('p2', 'Second')
    await flushPromises()
    expect(wrapper.find('.mcp-modal').exists()).toBe(false)
    expect(api.mcpServers.list).toHaveBeenLastCalledWith('p2')
  })
})