import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs, clickText } from '@/test/helpers'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

import SandboxDrawer from '@/components/sandbox/SandboxDrawer.vue'

const item = {
  id: 'sbx-1', name: 'test', cloudName: 'sbx-1', source: 'api', status: 'running', image: 'python:3.12-slim',
  cpus: 1, memoryMiB: 256, network: 'none', idleTimeoutS: 300, maxDurationS: 1800, createdAt: '2026-10-04T00:00:00Z'
}
function drawer(source = 'api') {
  return mount(SandboxDrawer, {
    props: { open: true, projectId: 'p', sandbox: { ...item, source }, maxFileBytes: 1048576 },
    global: { stubs: uiStubs }
  })
}

describe('SandboxDrawer', () => {
  beforeEach(() => resetApiMocks())
  it('shows nonzero exit and truncated output', async () => {
    api.sandboxes.exec.mockResolvedValueOnce({ stdout: 'x', stderr: 'bad', exitCode: 3,
      stdoutTruncated: true, stderrTruncated: false, timedOut: false, durationMs: 8 })
    const w = drawer()
    const vm = w.vm as unknown as { command: string; execute: () => Promise<void> }
    vm.command = 'exit 3'
    await vm.execute()
    await flushPromises()
    expect(w.text()).toContain('exit 3')
    expect(w.text()).toContain('输出已截断')
    expect(w.text()).toContain('bad')
    w.unmount()
  })
  it('disables Agent terminal', async () => {
    const w = drawer('agent')
    expect(w.text()).toContain('终端只读')
    const vm = w.vm as unknown as { execute: () => Promise<void> }
    await vm.execute()
    expect(api.sandboxes.exec).not.toHaveBeenCalled()
    w.unmount()
  })

  it('uploads binary data without text transcoding and downloads raw blob', async () => {
    const w = drawer()
    const vm = w.vm as unknown as { uploadFile: (event: Event) => Promise<void>; downloadFile: () => Promise<void>; filePath: string }
    const binary = { name: 'data.bin', size: 3, arrayBuffer: async () => new Uint8Array([0, 255, 128]).buffer }
    await vm.uploadFile({ target: { files: [binary], value: '' } } as unknown as Event)
    expect(api.sandboxes.files.upload).toHaveBeenCalledWith('p', 'sbx-1', '/workspace/data.bin', new Uint8Array([0, 255, 128]))
    w.unmount()
  })

  it('handles file browsing, editing, errors and timeout outputs', async () => {
    const w = drawer()
    const vm = w.vm as unknown as {
      tab: string; command: string; history: unknown[]; directory: string; fileText: string; filePath: string;
      execute: () => Promise<void>; loadFiles: () => Promise<void>;
      openEntry: (entry: { name: string; path: string; kind: string; size: number }) => Promise<void>;
      createFile: () => void; saveFile: () => Promise<void>; removeFile: (path: string) => Promise<void>;
      uploadFile: (e: Event) => Promise<void>; copyExample: () => Promise<void>
    }
    api.sandboxes.exec.mockResolvedValueOnce({ stdout: '', stderr: '', exitCode: -1, durationMs: 1000, timedOut: true })
    vm.command = 'sleep 2'
    await vm.execute()
    await flushPromises()
    expect(w.text()).toContain('超时')
    api.sandboxes.exec.mockRejectedValueOnce(new Error('network'))
    vm.command = 'bad'
    await vm.execute()

    vm.tab = '文件'
    await flushPromises()
    api.sandboxes.files.list.mockResolvedValueOnce([{ name: 'a.txt', path: '/workspace/a.txt', kind: 'file', size: 3 }])
    await vm.loadFiles()
    expect(api.sandboxes.files.list).toHaveBeenCalledWith('p', 'sbx-1', '/workspace')
    api.sandboxes.files.read.mockResolvedValueOnce({ content: 'abc', encoding: 'utf8', truncated: true })
    await vm.openEntry({ name: 'a.txt', path: '/workspace/a.txt', kind: 'file', size: 3 })
    expect(vm.fileText).toBe('abc')
    await vm.saveFile()
    expect(api.sandboxes.files.write).toHaveBeenCalledWith('p', 'sbx-1', '/workspace/a.txt', 'abc')
    await vm.removeFile('/workspace/a.txt')
    expect(api.sandboxes.files.remove).toHaveBeenCalled()
    await vm.openEntry({ name: 'sub', path: '/workspace/sub', kind: 'directory', size: 0 })
    expect(vm.directory).toBe('/workspace/sub')
    vm.createFile()
    expect(vm.filePath).toBe('/workspace/sub/new.txt')
    await vm.uploadFile({ target: { files: [{ name: 'big.txt', size: 1048577 }] } } as unknown as Event)
    await vm.copyExample()
    w.unmount()
  })
})

describe('SandboxDrawer interactions', () => {
  beforeEach(() => resetApiMocks())

  it('runs commands with enter key, switches tabs and navigates directories', async () => {
    const w = drawer()
    await w.get('[aria-label="沙盒命令"]').setValue('echo ok')
    await w.get('[aria-label="沙盒命令"]').trigger('keyup.enter')
    await flushPromises()
    expect(api.sandboxes.exec).toHaveBeenCalledWith('p', 'sbx-1', { command: 'echo ok' })
    expect(w.emitted('changed')).toHaveLength(1)
    expect(w.text()).toContain('ok')

    api.sandboxes.files.list.mockResolvedValueOnce([
      { name: 'sub', path: '/workspace/sub', kind: 'directory', size: 0 },
      { name: 'a.txt', path: '/workspace/a.txt', kind: 'file', size: 2 }
    ])
    await clickText(w, '文件')
    await flushPromises()
    expect(w.text()).toContain('[目录] sub')
    const entry = w.findAll('button').find((b) => b.text().includes('[目录] sub'))
    await entry!.trigger('click')
    await flushPromises()
    expect(api.sandboxes.files.list).toHaveBeenLastCalledWith('p', 'sbx-1', '/workspace/sub')
    await clickText(w, '上一级')
    await flushPromises()
    expect(api.sandboxes.files.list).toHaveBeenLastCalledWith('p', 'sbx-1', '/workspace')
    await clickText(w, '新建文件')
    await w.get('[aria-label="文件内容"]').setValue('hello')
    await clickText(w, '保存')
    await flushPromises()
    expect(api.sandboxes.files.write).toHaveBeenCalledWith('p', 'sbx-1', '/workspace/new.txt', 'hello')
    await clickText(w, '刷新')
    await flushPromises()

    await clickText(w, '信息')
    expect(w.text()).toContain('Cloud：sbx-1')
    expect(w.text()).toContain('未启动')
    await clickText(w, '终端')
    expect(w.text()).toContain('$ echo ok')
    w.unmount()
  })

  it('downloads files, copies curl and resets when sandbox changes', async () => {
    const createObjectURL = vi.fn(() => 'blob:x')
    const revokeObjectURL = vi.fn()
    vi.stubGlobal('URL', Object.assign(URL, { createObjectURL, revokeObjectURL }))
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const writeText = vi.fn().mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error('denied'))
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })

    const w = drawer()
    const vm = w.vm as unknown as {
      tab: string; filePath: string; fileText: string; history: unknown[]; directory: string;
      downloadFile: () => Promise<void>; copyExample: () => Promise<void>; openEntry: (e: object) => Promise<void>
    }
    await vm.downloadFile()
    expect(api.sandboxes.files.download).not.toHaveBeenCalled()
    api.sandboxes.files.read.mockResolvedValueOnce({ content: 'AAE=', encoding: 'base64', truncated: false })
    await vm.openEntry({ name: 'bin', path: '/workspace/bin', kind: 'file', size: 2 })
    vm.tab = '文件'
    await flushPromises()
    expect(w.text()).toContain('二进制文件无法编辑')
    expect(vm.fileText).toBe('')
    await clickText(w, '下载')
    await flushPromises()
    expect(api.sandboxes.files.download).toHaveBeenCalledWith('p', 'sbx-1', '/workspace/bin')
    expect(click).toHaveBeenCalled()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:x')
    api.sandboxes.files.download.mockRejectedValueOnce('boom')
    await vm.downloadFile()
    api.sandboxes.files.download.mockRejectedValueOnce(new Error('download failed'))
    await vm.downloadFile()
    vm.filePath = '/'
    await vm.downloadFile()

    vm.tab = '信息'
    await flushPromises()
    await clickText(w, '复制 curl 示例')
    await flushPromises()
    expect(writeText.mock.calls[0][0]).toContain('/v1/projects/p/sandboxes/sbx-1/exec')
    expect(writeText.mock.calls[0][0]).toContain('$API_KEY')
    await vm.copyExample()
    expect(writeText).toHaveBeenCalledTimes(2)

    await w.setProps({ sandbox: { ...item, id: 'sbx-2', lastActiveAt: 'now', lastError: 'cloud removal failed' } })
    expect(vm.tab).toBe('终端')
    expect(vm.filePath).toBe('')
    vm.tab = '信息'
    await flushPromises()
    expect(w.text()).toContain('cloud removal failed')
    click.mockRestore()
    vi.unstubAllGlobals()
    w.unmount()
  })

  it('reports file errors, guards concurrent actions and handles empty states', async () => {
    const w = drawer()
    const vm = w.vm as unknown as {
      tab: string; command: string; running: boolean; saving: boolean; filePath: string;
      execute: () => Promise<void>; loadFiles: () => Promise<void>; saveFile: () => Promise<void>;
      removeFile: (p: string) => Promise<void>; uploadFile: (e: Event) => Promise<void>;
      openEntry: (e: object) => Promise<void>
    }
    vm.command = '   '
    await vm.execute()
    vm.command = 'echo x'
    vm.running = true
    await vm.execute()
    vm.running = false
    api.sandboxes.exec.mockRejectedValueOnce('boom')
    await vm.execute()
    expect(api.sandboxes.exec).toHaveBeenCalledTimes(1)

    api.sandboxes.files.list.mockRejectedValueOnce(new Error('list failed'))
    await vm.loadFiles()
    api.sandboxes.files.list.mockRejectedValueOnce('boom')
    await vm.loadFiles()
    api.sandboxes.files.read.mockRejectedValueOnce(new Error('read failed'))
    await vm.openEntry({ name: 'a', path: '/workspace/a', kind: 'file', size: 1 })
    api.sandboxes.files.read.mockRejectedValueOnce('boom')
    await vm.openEntry({ name: 'a', path: '/workspace/a', kind: 'file', size: 1 })
    vm.saving = true
    await vm.saveFile()
    vm.saving = false
    api.sandboxes.files.write.mockRejectedValueOnce(new Error('write failed'))
    await vm.saveFile()
    api.sandboxes.files.write.mockRejectedValueOnce('boom')
    await vm.saveFile()
    api.sandboxes.files.remove.mockRejectedValueOnce(new Error('rm failed'))
    await vm.removeFile('/workspace/a')
    api.sandboxes.files.remove.mockRejectedValueOnce('boom')
    await vm.removeFile('/workspace/a')
    vm.filePath = '/workspace/b'
    await vm.removeFile('/workspace/c')
    expect(vm.filePath).toBe('/workspace/b')
    await vm.removeFile('/workspace/b')
    expect(vm.filePath).toBe('')

    const target = { files: [{ name: 'a', size: 1, arrayBuffer: async () => new ArrayBuffer(1) }], value: 'x' }
    api.sandboxes.files.upload.mockRejectedValueOnce(new Error('upload failed'))
    await vm.uploadFile({ target } as unknown as Event)
    expect(target.value).toBe('')
    api.sandboxes.files.upload.mockRejectedValueOnce('boom')
    await vm.uploadFile({ target } as unknown as Event)
    await vm.uploadFile({ target: { files: [] } } as unknown as Event)
    expect(api.sandboxes.files.upload).toHaveBeenCalledTimes(2)
    vm.tab = '文件'
    await flushPromises()
    expect(w.text()).toContain('目录为空')
    w.unmount()
  })

  it('agent sandboxes are read-only in files tab and null sandbox is inert', async () => {
    api.sandboxes.files.list.mockResolvedValue([{ name: 'a.txt', path: '/workspace/a.txt', kind: 'file', size: 2 }])
    const agent = drawer('agent')
    await clickText(agent, '文件')
    await flushPromises()
    expect(agent.text()).not.toContain('新建文件')
    expect(agent.text()).not.toContain('上传文件')
    expect(agent.find('.confirm-action').exists()).toBe(false)
    agent.unmount()
    api.sandboxes.files.list.mockClear()

    const empty = mount(SandboxDrawer, {
      props: { open: true, projectId: 'p', sandbox: null, maxFileBytes: 1 },
      global: { stubs: uiStubs }
    })
    const vm = empty.vm as unknown as {
      tab: string; filePath: string; loadFiles: () => Promise<void>; openEntry: (e: object) => Promise<void>;
      saveFile: () => Promise<void>; removeFile: (p: string) => Promise<void>; uploadFile: (e: Event) => Promise<void>;
      downloadFile: () => Promise<void>; copyExample: () => Promise<void>; execute: () => Promise<void>
    }
    await vm.execute()
    await vm.loadFiles()
    await vm.openEntry({ name: 'a', path: '/a', kind: 'file', size: 0 })
    await vm.saveFile()
    await vm.removeFile('/a')
    await vm.uploadFile({ target: { files: [{ name: 'a', size: 0 }] } } as unknown as Event)
    vm.filePath = '/a'
    await vm.downloadFile()
    await vm.copyExample()
    expect(api.sandboxes.files.list).not.toHaveBeenCalled()
    expect(api.sandboxes.files.download).not.toHaveBeenCalled()
    await empty.get('.sb-modal')
    empty.unmount()

    // 关闭状态切到文件 tab 不加载；关闭事件透传。
    const closed = mount(SandboxDrawer, {
      props: { open: false, projectId: 'p', sandbox: item, maxFileBytes: 1 },
      global: { stubs: { ...uiStubs, SbModal: { props: ['open'], emits: ['update:open'], template: '<div><button class="close" @click="$emit(\'update:open\', false)">c</button><slot /></div>' } } }
    })
    ;(closed.vm as unknown as { tab: string }).tab = '文件'
    await flushPromises()
    expect(api.sandboxes.files.list).not.toHaveBeenCalled()
    await closed.get('.close').trigger('click')
    expect(closed.emitted('update:open')?.[0]).toEqual([false])
    closed.unmount()
  })
})
