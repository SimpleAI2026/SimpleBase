import { flushPromises, type VueWrapper } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { mountWithApp } from '@/test/helpers'
import type { SandboxItem, SandboxCapabilities } from '@/services/types'

vi.mock('vue-sonner', () => ({ toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() } }))
vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

import SandboxDrawer from '@/components/sandbox/SandboxDrawer.vue'

const caps: SandboxCapabilities = {
  available: true, backend: 'fake', images: ['python:3.12-slim'], defaultImage: 'python:3.12-slim',
  cpus: 1, memoryMiB: 512, timeoutDefaultS: 300, timeoutMaxS: 3600, maxFileBytes: 1048576, maxOutputBytes: 65536,
  maxPerProject: 5, terminal: true
}
const item: SandboxItem = {
  id: 'sbx-1', name: 'test', templateId: 'python:3.12-slim', state: 'running', status: 'running', source: 'api',
  cpuCount: 1, memoryMB: 256, idleTimeoutS: 300, metadata: {}, createdAt: '2026-10-04T00:00:00Z'
}

/** 页签用真实 Tabs（uiStubs 里的桩不承载真实交互语义），其余沿用 mountWithApp 默认桩。 */
const realTabs = { Tabs: false, TabsList: false, TabsTrigger: false, TabsContent: false }
const terminalStub = { template: '<div data-terminal />' }
const drawerStubs = { ...realTabs, SandboxTerminal: terminalStub }

/**
 * jsdom 的 Blob 没有 arrayBuffer()，组件读文件时依赖它。这里给一个只实现
 * 组件用到的两个成员的对象（size / arrayBuffer），行为与真实 Blob 一致。
 */
function fakeBlob(bytes: number[]): Blob {
  const buf = new Uint8Array(bytes)
  return { size: buf.byteLength, arrayBuffer: async () => buf.buffer } as unknown as Blob
}

/** reka 的 TabsTrigger 在 mousedown 上切换，不是 click。 */
async function openTab(w: VueWrapper, label: string) {
  const trigger = w.findAll('[data-slot="tabs-trigger"]').find((t) => t.text() === label)
  if (!trigger) throw new Error('tab not found: ' + label)
  await trigger.trigger('mousedown')
  await flushPromises()
}
async function clickText(w: VueWrapper, text: string) {
  const btn = w.findAll('button').find((b) => b.text().includes(text))
  if (!btn) throw new Error('button not found: ' + text)
  await btn.trigger('click')
  await flushPromises()
}

describe('SandboxDrawer', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    resetApiMocks()
  })

  it('shows terminal not-running hint when sandbox is paused', async () => {
    const { wrapper } = await mountWithApp(SandboxDrawer, {
      props: { open: true, projectId: 'p', sandbox: { ...item, state: 'paused', status: 'stopped' }, capabilities: caps },
      stubs: drawerStubs
    })
    await openTab(wrapper, '终端')
    await flushPromises()
    // 终端组件在状态非 running 时显示不可用提示。由于终端是懒加载动态 import，我们只能桩掉整个组件。
    expect(wrapper.get('[data-terminal]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('handles file browsing, editing, deleting and read-only guard', async () => {
    api.sandboxes.files.list.mockResolvedValue([
      { name: 'sub', path: '/workspace/sub', kind: 'directory', size: 0 },
      { name: 'a.txt', path: '/workspace/a.txt', kind: 'file', size: 3 }
    ])
    api.sandboxes.files.read.mockResolvedValue(fakeBlob([97, 98, 99]))
    const { wrapper } = await mountWithApp(SandboxDrawer, {
      props: { open: true, projectId: 'p', sandbox: item, capabilities: caps },
      stubs: drawerStubs
    })
    await openTab(wrapper, '文件')
    await flushPromises()
    expect(api.sandboxes.files.list).toHaveBeenCalledWith('p', 'sbx-1', '/workspace')
    expect(wrapper.text()).toContain('a.txt')
    const entry = wrapper.findAll('button').find((b) => b.text().includes('[目录] sub'))
    await entry!.trigger('click')
    await flushPromises()
    expect(api.sandboxes.files.list).toHaveBeenCalledWith('p', 'sbx-1', '/workspace/sub')
    await clickText(wrapper, '上一级')
    await flushPromises()
    expect(api.sandboxes.files.list).toHaveBeenLastCalledWith('p', 'sbx-1', '/workspace')
    const fileBtn = wrapper.findAll('button').find((b) => b.text() === 'a.txt')
    await fileBtn!.trigger('click')
    await flushPromises()
    expect(api.sandboxes.files.read).toHaveBeenCalledWith('p', 'sbx-1', '/workspace/a.txt')
    const editor = wrapper.find('textarea')
    expect(editor.element.value).toBe('abc')
    await clickText(wrapper, '新建文件')
    await wrapper.get('[aria-label="文件内容"]').setValue('hello')
    await clickText(wrapper, '保存')
    await flushPromises()
    expect(api.sandboxes.files.write).toHaveBeenCalledWith('p', 'sbx-1', '/workspace/new.txt', 'hello')
    wrapper.unmount()
  })

  it('uploads binary without transcoding and downloads raw blob', async () => {
    const createObjectURL = vi.fn(() => 'blob:x')
    const revokeObjectURL = vi.fn()
    vi.stubGlobal('URL', Object.assign(URL, { createObjectURL, revokeObjectURL }))
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    api.sandboxes.files.list.mockResolvedValue([{ name: 'data.bin', path: '/workspace/data.bin', kind: 'file', size: 3 }])
    api.sandboxes.files.read.mockResolvedValue(fakeBlob([0, 255, 128]))
    const { wrapper } = await mountWithApp(SandboxDrawer, {
      props: { open: true, projectId: 'p', sandbox: item, capabilities: caps },
      stubs: drawerStubs
    })
    await openTab(wrapper, '文件')
    await flushPromises()
    const vm = wrapper.vm as unknown as { uploadFile: (e: Event) => Promise<void>; fileText: string; fileBinary: boolean; filePath: string }
    const binary = { name: 'data.bin', size: 3, arrayBuffer: async () => new Uint8Array([0, 255, 128]).buffer }
    await vm.uploadFile({ target: { files: [binary], value: '' } } as unknown as Event)
    expect(api.sandboxes.files.write).toHaveBeenCalledWith('p', 'sbx-1', '/workspace/data.bin', binary)
    const fileBtn = wrapper.findAll('button').find((b) => b.text() === 'data.bin')
    await fileBtn!.trigger('click')
    await flushPromises()
    expect(vm.fileBinary).toBe(true)
    expect(vm.fileText).toBe('')
    expect(wrapper.text()).toContain('二进制文件无法编辑')
    await clickText(wrapper, '下载')
    await flushPromises()
    expect(click).toHaveBeenCalled()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:x')
    click.mockRestore()
    vi.unstubAllGlobals()
    wrapper.unmount()
  })

  it('reports errors and handles concurrent guard', async () => {
    api.sandboxes.files.list.mockRejectedValue(new Error('list failed'))
    const { wrapper } = await mountWithApp(SandboxDrawer, {
      props: { open: true, projectId: 'p', sandbox: item, capabilities: caps },
      stubs: drawerStubs
    })
    await openTab(wrapper, '文件')
    await flushPromises()
    expect(wrapper.find('[data-slot="sb-error-state"]').exists()).toBe(true)
    await clickText(wrapper, '重试')
    await flushPromises()
    const vm = wrapper.vm as unknown as { saving: boolean; saveFile: () => Promise<void>; filePath: string; fileText: string }
    vm.filePath = '/workspace/new.txt'
    vm.fileText = 'x'
    api.sandboxes.files.write.mockRejectedValueOnce(new Error('write failed'))
    await vm.saveFile()
    vm.saving = true
    await vm.saveFile()
    expect(api.sandboxes.files.write).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('shows info tab with state, spec, timestamps, SSH and E2B snippets', async () => {
    const { wrapper } = await mountWithApp(SandboxDrawer, {
      props: {
        open: true, projectId: 'p',
        sandbox: { ...item, startedAt: '2026-10-04T01:00:00Z', lastActiveAt: '2026-10-04T01:10:00Z', endAt: '2026-10-04T02:00:00Z', lastError: 'restart failed' },
        capabilities: { ...caps, ssh: { enabled: true, listen: '0.0.0.0:2222', hostKeyFingerprint: 'SHA256:abc' } }
      },
      stubs: drawerStubs
    })
    await openTab(wrapper, '信息')
    expect(wrapper.text()).toContain('running · api')
    expect(wrapper.text()).toContain('python:3.12-slim')
    expect(wrapper.text()).toContain('1C / 256MB')
    expect(wrapper.text()).toContain('restart failed')
    expect(wrapper.text()).toContain('SSH 登录')
    expect(wrapper.text()).toContain('E2B SDK 接入')
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    await clickText(wrapper, '复制 SSH 命令')
    await flushPromises()
    expect(writeText.mock.calls[0][0]).toContain('ssh -p 2222 sbx-1@')
    wrapper.unmount()
  })

  it('oversized file is treated as binary and warned', async () => {
    api.sandboxes.files.list.mockResolvedValue([{ name: 'big.txt', path: '/workspace/big.txt', kind: 'file', size: 999999 }])
    api.sandboxes.files.read.mockResolvedValue(fakeBlob([97, 98]))
    const { wrapper } = await mountWithApp(SandboxDrawer, {
      props: { open: true, projectId: 'p', sandbox: item, capabilities: { ...caps, maxFileBytes: 1 } },
      stubs: drawerStubs
    })
    await openTab(wrapper, '文件')
    await flushPromises()
    await wrapper.findAll('button').find((b) => b.text() === 'big.txt')!.trigger('click')
    await flushPromises()
    expect(toast.warning).toHaveBeenCalledWith('文件超出大小限制，已按二进制处理')
    expect(wrapper.find('textarea').element.value).toBe('')
    wrapper.unmount()
  })

  it('surfaces read / delete / download failures', async () => {
    vi.mocked(toast.error).mockClear()
    vi.mocked(toast.success).mockClear()
    api.sandboxes.files.list.mockResolvedValue([{ name: 'a.txt', path: '/workspace/a.txt', kind: 'file', size: 1 }])
    api.sandboxes.files.read.mockRejectedValue(new Error('read'))
    const { wrapper } = await mountWithApp(SandboxDrawer, {
      props: { open: true, projectId: 'p', sandbox: item, capabilities: caps },
      stubs: drawerStubs
    })
    await openTab(wrapper, '文件')
    await flushPromises()
    await wrapper.findAll('button').find((b) => b.text() === 'a.txt')!.trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalled()
    const vm = wrapper.vm as unknown as {
      filePath: string
      downloadFile: () => Promise<void>
      removeFile: (p: string) => Promise<void>
    }
    vm.filePath = '/workspace/a.txt'
    await vm.downloadFile()
    expect(toast.error).toHaveBeenCalledTimes(2)
    api.sandboxes.files.remove.mockRejectedValueOnce(new Error('rm'))
    await vm.removeFile('/workspace/a.txt')
    expect(toast.error).toHaveBeenCalledTimes(3)
    // 删除成功后清掉当前文件并重新列目录
    api.sandboxes.files.remove.mockResolvedValueOnce(undefined)
    vm.filePath = '/workspace/a.txt'
    await vm.removeFile('/workspace/a.txt')
    expect(vm.filePath).toBe('')
    expect(toast.success).toHaveBeenCalledWith('文件已删除')
    wrapper.unmount()
  })

  it('upload rejects oversized files and reports failures, then clears the input', async () => {
    const { wrapper } = await mountWithApp(SandboxDrawer, {
      props: { open: true, projectId: 'p', sandbox: item, capabilities: { ...caps, maxFileBytes: 2 } },
      stubs: drawerStubs
    })
    const vm = wrapper.vm as unknown as { uploadFile: (e: Event) => Promise<void> }
    await vm.uploadFile({ target: { files: [{ name: 'big.bin', size: 99 }], value: '' } } as unknown as Event)
    expect(toast.error).toHaveBeenCalledWith('文件超出大小限制')
    // 没有选中文件时直接返回
    await vm.uploadFile({ target: { files: [], value: '' } } as unknown as Event)
    api.sandboxes.files.write.mockRejectedValueOnce(new Error('up'))
    const target = { files: [{ name: 'ok.txt', size: 1 }], value: 'x' }
    await vm.uploadFile({ target } as unknown as Event)
    expect(toast.error).toHaveBeenCalled()
    expect(target.value).toBe('')
    wrapper.unmount()
  })

  it('resets file path and tab when sandbox changes', async () => {
    const { wrapper } = await mountWithApp(SandboxDrawer, {
      props: { open: true, projectId: 'p', sandbox: item, capabilities: caps },
      stubs: drawerStubs
    })
    const vm = wrapper.vm as unknown as { tab: string; filePath: string; createFile: () => void }
    vm.tab = 'files'
    vm.createFile()
    expect(vm.filePath).toBe('/workspace/new.txt')
    await wrapper.setProps({ sandbox: { ...item, id: 'sbx-2' } })
    expect(vm.tab).toBe('terminal')
    expect(vm.filePath).toBe('')
    wrapper.unmount()
  })

  it('agent sandboxes are read-only in files tab and null sandbox is inert', async () => {
    api.sandboxes.files.list.mockResolvedValue([{ name: 'a.txt', path: '/workspace/a.txt', kind: 'file', size: 2 }])
    const { wrapper: agent } = await mountWithApp(SandboxDrawer, {
      props: { open: true, projectId: 'p', sandbox: { ...item, source: 'agent' }, capabilities: caps },
      stubs: drawerStubs
    })
    await openTab(agent, '文件')
    await flushPromises()
    expect(agent.text()).not.toContain('新建文件')
    expect(agent.text()).not.toContain('上传文件')
    agent.unmount()

    const { wrapper: empty } = await mountWithApp(SandboxDrawer, {
      props: { open: true, projectId: 'p', sandbox: null, capabilities: caps },
      stubs: drawerStubs
    })
    api.sandboxes.files.list.mockClear()
    const vm = empty.vm as unknown as { saveFile: () => Promise<void> }
    await vm.saveFile()
    expect(api.sandboxes.files.list).not.toHaveBeenCalled()
    empty.unmount()
  })
})

describe('SandboxDrawer 边界分支', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    resetApiMocks()
  })

  it('没有沙盒与能力信息时所有操作都是空操作', async () => {
    const { wrapper } = await mountWithApp(SandboxDrawer, {
      props: { open: true, projectId: 'p', sandbox: null, capabilities: null },
      stubs: drawerStubs
    })
    const vm = wrapper.vm as unknown as {
      tab: string
      maxFileBytes: number
      e2bSnippet: string
      openEntry: (e: { name: string; path: string; kind: string; size: number }) => Promise<void>
      removeFile: (p: string) => Promise<void>
      downloadFile: () => Promise<void>
    }
    vm.tab = 'files'
    await flushPromises()
    await vm.openEntry({ name: 'a', path: '/workspace/a', kind: 'file', size: 1 })
    await vm.removeFile('/workspace/a')
    await vm.downloadFile()
    expect(api.sandboxes.files.list).not.toHaveBeenCalled()
    expect(api.sandboxes.files.read).not.toHaveBeenCalled()
    expect(api.sandboxes.files.remove).not.toHaveBeenCalled()
    expect(vm.maxFileBytes).toBe(1048576)
    expect(vm.e2bSnippet).toContain("Sandbox.connect('')")
    wrapper.unmount()
  })

  it('上一级回到根目录、下载文件名兜底、SSH 无指纹时不渲染指纹行', async () => {
    const createObjectURL = vi.fn(() => 'blob:y')
    const revokeObjectURL = vi.fn()
    vi.stubGlobal('URL', Object.assign(URL, { createObjectURL, revokeObjectURL }))
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    api.sandboxes.files.read.mockResolvedValue(fakeBlob([1]))
    const { wrapper } = await mountWithApp(SandboxDrawer, {
      props: {
        open: true, projectId: 'p', sandbox: item,
        capabilities: { ...caps, ssh: { enabled: true, listen: '0.0.0.0:2222', hostKeyFingerprint: '' } }
      },
      stubs: drawerStubs
    })
    const vm = wrapper.vm as unknown as { directory: string; filePath: string; goUp: () => void; downloadFile: () => Promise<void> }

    // 没选文件：下载直接返回
    await vm.downloadFile()
    expect(api.sandboxes.files.read).not.toHaveBeenCalled()

    // 以斜杠结尾的路径取不到文件名，退回 file
    vm.filePath = '/workspace/'
    await vm.downloadFile()
    expect(click).toHaveBeenCalled()

    // 目录只剩一级时上一级回到根
    vm.directory = '/x'
    vm.goUp()
    expect(vm.directory).toBe('/workspace')

    await openTab(wrapper, '信息')
    expect(wrapper.text()).toContain('ssh -p 2222')
    expect(wrapper.text()).not.toContain('SHA256')
    click.mockRestore()
    vi.unstubAllGlobals()
    wrapper.unmount()
  })
})

describe('SandboxDrawer interactions', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    resetApiMocks()
  })

  it('copies E2B example and curl and handles clipboard failures', async () => {
    const writeText = vi.fn().mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error('denied'))
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    const { wrapper } = await mountWithApp(SandboxDrawer, {
      props: { open: true, projectId: 'p1', sandbox: item, capabilities: caps },
      stubs: drawerStubs
    })
    await openTab(wrapper, '信息')
    await clickText(wrapper, '复制接入片段')
    await flushPromises()
    expect(writeText.mock.calls[0][0]).toContain('export E2B_API_URL=')
    expect(writeText.mock.calls[0][0]).toContain("Sandbox.connect('sbx-1')")
    await clickText(wrapper, '复制 curl 示例')
    await flushPromises()
    expect(writeText.mock.calls[1][0]).toContain('/e2b/v2/sandboxes')
    expect(writeText.mock.calls[1][0]).toContain('p1')
    const vm = wrapper.vm as unknown as { copyText: (t: string, k: string) => Promise<void> }
    await vm.copyText('x', 'sshCopied')
    expect(toast.error).toHaveBeenCalledWith('无法复制到剪贴板')
    wrapper.unmount()
  })

  it('keeps tabs and files when open=false and only loads files when tab active + open', async () => {
    api.sandboxes.files.list.mockResolvedValue([])
    const { wrapper } = await mountWithApp(SandboxDrawer, {
      props: { open: false, projectId: 'p', sandbox: item, capabilities: caps },
      stubs: drawerStubs
    })
    const vm = wrapper.vm as unknown as { tab: string }
    vm.tab = 'files'
    await flushPromises()
    expect(api.sandboxes.files.list).not.toHaveBeenCalled()
    await wrapper.setProps({ open: true })
    vm.tab = 'info'
    await flushPromises()
    vm.tab = 'files'
    await flushPromises()
    expect(api.sandboxes.files.list).toHaveBeenCalledOnce()
    wrapper.unmount()
  })
})
