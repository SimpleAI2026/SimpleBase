import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { api, resetApiMocks } from '@/test/api-mock'
import type { SandboxCapabilities, SandboxItem } from '@/services/types'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

/** 记录所有创建的 socket，供测试驱动 onopen / onmessage / onclose。 */
class FakeSocket {
  static all: FakeSocket[] = []
  static readonly OPEN = 1
  static readonly CLOSED = 3
  readyState = 0
  binaryType = ''
  sent: unknown[] = []
  onopen: (() => void) | null = null
  onmessage: ((e: { data: unknown }) => void) | null = null
  onerror: (() => void) | null = null
  onclose: (() => void) | null = null
  constructor(public url: string) {
    FakeSocket.all.push(this)
  }
  send(data: unknown) { this.sent.push(data) }
  close() { this.readyState = 3; this.onclose?.() }
}

const termWrites: string[] = []
/** 最近一次创建的终端实例，供测试驱动 onData / onResize。 */
const terms: Array<{
  dataCb?: (t: string) => void
  resizeCb?: (s: { cols: number; rows: number }) => void
  fitCall?: () => void
  cols: number
  rows: number
}> = []
vi.mock('@xterm/xterm', () => ({
  Terminal: class {
    cols = 80
    rows = 24
    focus = vi.fn()
    write = (data: Uint8Array) => { termWrites.push(new TextDecoder().decode(data)) }
    reset = vi.fn()
    dispose = vi.fn()
    loadAddon = vi.fn()
    open = vi.fn()
    dataCb: ((t: string) => void) | undefined
    resizeCb: ((s: { cols: number; rows: number }) => void) | undefined
    constructor() { terms.push(this) }
    onData(cb: (t: string) => void) { this.dataCb = cb }
    onResize(cb: (s: { cols: number; rows: number }) => void) { this.resizeCb = cb }
  }
}))
vi.mock('@xterm/addon-fit', () => ({
  FitAddon: class {
    // 转发最近一个终端的 fit 调用，测试用它验证零宽容器时被跳过
    fit = () => { const last = terms[terms.length - 1]; last.fitCall?.() }
    constructor() {
      // 终端与 addon 的创建顺序由组件保证：先 Terminal 再 FitAddon
      const last = terms[terms.length - 1]
      if (last) last.fitCall = vi.fn()
    }
  }
}))
vi.mock('@xterm/xterm/css/xterm.css', () => ({}))

/** 取当前挂载中的终端实例。 */
function mountedTerm(w: { find: (s: string) => unknown }) {
  void w
  return terms[terms.length - 1]
}

import SandboxTerminal from '@/components/sandbox/SandboxTerminal.vue'

const caps: SandboxCapabilities = {
  available: true, backend: 'local', images: ['x'], defaultImage: 'x', cpus: 1, memoryMiB: 512,
  timeoutDefaultS: 300, timeoutMaxS: 3600, maxFileBytes: 1024, maxOutputBytes: 1024, maxPerProject: 5, terminal: true
}
const item: SandboxItem = {
  id: 'sbx-1', name: 'test', templateId: 'x', state: 'running', status: 'running', source: 'api',
  cpuCount: 1, memoryMB: 256, idleTimeoutS: 300, metadata: {}
}

function mountTerminal(over: Partial<{ active: boolean; sandbox: SandboxItem; capabilities: SandboxCapabilities }> = {}) {
  return mount(SandboxTerminal, {
    props: { projectId: 'p', sandbox: item, capabilities: caps, active: true, ...over },
    global: { plugins: [createPinia()] }
  })
}

describe('SandboxTerminal', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    resetApiMocks()
    FakeSocket.all = []
    termWrites.length = 0
    terms.length = 0
    vi.stubGlobal('WebSocket', FakeSocket as unknown as typeof WebSocket)
  })

  it('不建连时不请求票据', async () => {
    mountTerminal({ active: false })
    await flushPromises()
    expect(api.sandboxes.terminalTicket).not.toHaveBeenCalled()
  })

  it('按四种原因给出空态且不请求票据', async () => {
    const cases: Array<[Partial<{ sandbox: SandboxItem; capabilities: SandboxCapabilities }>, string]> = [
      [{ sandbox: { ...item, source: 'agent' } }, '此沙盒由 Agent 会话使用'],
      [{ capabilities: { ...caps, terminal: false } }, '当前后端不支持交互终端'],
      [{ sandbox: { ...item, state: 'paused' } }, '沙盒已暂停'],
      [{ capabilities: null as unknown as SandboxCapabilities }, '当前后端不支持交互终端']
    ]
    for (const [over, text] of cases) {
      const w = mountTerminal(over)
      await flushPromises()
      expect(w.text()).toContain(text)
      expect(w.find('[data-testid="terminal-host"]').exists()).toBe(false)
      w.unmount()
    }
    expect(api.sandboxes.terminalTicket).not.toHaveBeenCalled()
  })

  it('连接后转发键盘输入、显示输出、回传退出码', async () => {
    const w = mountTerminal()
    await flushPromises()
    expect(api.sandboxes.terminalTicket).toHaveBeenCalledWith('p', 'sbx-1')
    const ws = FakeSocket.all[0]
    expect(ws.url).toContain('ticket=tk')
    ws.readyState = 1
    ws.onopen?.()
    await flushPromises()
    expect(w.text()).toContain('已连接')

    // 进程输出（二进制帧）→ 终端
    ws.onmessage?.({ data: new Uint8Array([104, 105]).buffer })
    expect(termWrites.join('')).toBe('hi')

    // 控制帧：resize 不发回（服务端不需要确认），exit 更新状态
    ws.onmessage?.({ data: JSON.stringify({ type: 'exit', code: 3 }) })
    await flushPromises()
    expect(w.text()).toContain('exit 3')
    expect(w.text()).toContain('连接已关闭')
    w.unmount()
  })

  it('resize 与 error 控制帧', async () => {
    const w = mountTerminal()
    await flushPromises()
    const ws = FakeSocket.all[0]
    ws.readyState = 1
    ws.onopen?.()
    ws.onmessage?.({ data: JSON.stringify({ type: 'resize', cols: 10, rows: 5 }) })
    ws.onmessage?.({ data: 'not json' })
    ws.onmessage?.({ data: JSON.stringify({ type: 'error', msg: 'boom' }) })
    await flushPromises()
    expect(w.text()).toContain('boom')
    ws.onerror?.()
    await flushPromises()
    expect(w.text()).toContain('连接失败')
    w.unmount()
  })

  it('票据失败与断线显示可重连状态', async () => {
    api.sandboxes.terminalTicket.mockRejectedValueOnce(new Error('403'))
    const w = mountTerminal()
    await flushPromises()
    expect(w.text()).toContain('无法获取终端票据')
    expect(w.text()).toContain('连接失败')
    // 重新连接：这次成功
    const btn = w.findAll('button').find((b) => b.text().includes('重新连接'))
    await btn!.trigger('click')
    await flushPromises()
    expect(FakeSocket.all.length).toBe(1)
    // 服务端关闭
    FakeSocket.all[0].readyState = 1
    FakeSocket.all[0].onopen?.()
    FakeSocket.all[0].readyState = 3
    FakeSocket.all[0].onclose?.()
    await flushPromises()
    expect(w.text()).toContain('连接已关闭')
    w.unmount()
  })

  it('切换到同一沙盒时复用终端、切沙盒时重建', async () => {
    const w = mountTerminal()
    await flushPromises()
    const first = w.find('[data-testid="terminal-host"]')
    expect(first.exists()).toBe(true)
    await w.setProps({ active: false })
    await w.setProps({ active: true })
    await flushPromises()
    // 同沙盒不重建 DOM 宿主
    expect(w.find('[data-testid="terminal-host"]').exists()).toBe(true)
    await w.setProps({ sandbox: { ...item, id: 'sbx-2' } })
    await flushPromises()
    expect(api.sandboxes.terminalTicket).toHaveBeenLastCalledWith('p', 'sbx-2')
    w.unmount()
  })

  it('卸载时关闭 socket', async () => {
    const w = mountTerminal()
    await flushPromises()
    const ws = FakeSocket.all[0]
    ws.readyState = 1
    w.unmount()
    expect(ws.readyState).toBe(3)
  })

  it('键盘输入与 resize 经 socket 发出，未连接时静默丢弃', async () => {
    const w = mountTerminal()
    await flushPromises()
    const ws = FakeSocket.all[0]
    const term = mountedTerm(w)
    // 未连接：不发送
    term.dataCb?.('x')
    expect(ws.sent).toEqual([])
    ws.readyState = 1
    ws.onopen?.()
    term.dataCb?.('ls\n')
    expect(ws.sent).toContainEqual(new TextEncoder().encode('ls\n'))
    term.resizeCb?.({ cols: 100, rows: 30 })
    expect(ws.sent).toContainEqual(JSON.stringify({ type: 'resize', cols: 100, rows: 30 }))
    w.unmount()
  })

  it('容器未布局时不 fit，close 抛错也被吞掉', async () => {
    const w = mountTerminal()
    await flushPromises()
    const host = w.get('[data-testid="terminal-host"]').element as HTMLElement
    const term = mountedTerm(w)
    Object.defineProperty(host, 'clientWidth', { value: 0, configurable: true })
    expect(() => term.fitCall?.()).not.toThrow()
    const ws = FakeSocket.all[0]
    ws.close = () => {
      throw new Error('already closed')
    }
    expect(() => w.unmount()).not.toThrow()
  })

  it('容器尺寸变化时 fit：页签隐藏则跳过，fit 抛错被吞掉', async () => {
    const callbacks: Array<() => void> = []
    vi.stubGlobal('ResizeObserver', class {
      constructor(cb: () => void) { callbacks.push(cb) }
      observe() {}
      unobserve() {}
      disconnect() {}
    })
    const w = mountTerminal()
    await flushPromises()
    const host = w.get('[data-testid="terminal-host"]').element as HTMLElement
    Object.defineProperty(host, 'clientWidth', { value: 300, configurable: true })
    const term = mountedTerm(w)
    expect(callbacks.length).toBe(1)

    // 可见且有宽度：真正调用 fit
    callbacks[0]()
    expect(term.fitCall).toHaveBeenCalledTimes(1)

    // fit 抛错（容器还没布局）：不向外传播
    term.fitCall = vi.fn(() => { throw new Error('not laid out') })
    expect(() => callbacks[0]()).not.toThrow()
    expect(term.fitCall).toHaveBeenCalledTimes(1)

    // 页签隐藏：不再 fit
    await w.setProps({ active: false })
    callbacks[0]()
    expect(term.fitCall).toHaveBeenCalledTimes(1)
    w.unmount()
  })

  it('只读账号显示只读空态且不建连', async () => {
    const { useAuthStore } = await import('@/stores/auth')
    // 组件用的是挂载时传入的 pinia，登录态必须写进同一个实例。
    const pinia = createPinia()
    setActivePinia(pinia)
    useAuthStore().user = { id: 'u', username: 'a', role: 'admin' } as never
    const ro = mount(SandboxTerminal, {
      props: { projectId: 'p', sandbox: item, capabilities: caps, active: true },
      global: { plugins: [pinia] }
    })
    await flushPromises()
    expect(ro.find('[data-testid="terminal-host"]').exists()).toBe(false)
    expect(ro.text()).toContain('只读')
    expect(api.sandboxes.terminalTicket).not.toHaveBeenCalled()
    ro.unmount()
  })

  it('exit 缺少退出码、error 缺少消息时有兜底，其他控制帧被忽略', async () => {
    const w = mountTerminal()
    await flushPromises()
    const ws = FakeSocket.all[0]
    ws.readyState = 1
    ws.onopen?.()
    ws.onmessage?.({ data: JSON.stringify({ type: 'noop' }) })
    ws.onmessage?.({ data: JSON.stringify({ type: 'error' }) })
    await flushPromises()
    expect(w.get('[data-testid="terminal-status"]').text()).toBe('连接失败')
    ws.onmessage?.({ data: JSON.stringify({ type: 'exit' }) })
    await flushPromises()
    expect(w.text()).toContain('exit 0')
    w.unmount()
  })

  it('过期的连接尝试（票据返回时已被取代）不会建立 socket', async () => {
    let release!: (v: { ticket: string }) => void
    api.sandboxes.terminalTicket.mockImplementationOnce(() => new Promise((r) => { release = r }))
    const w = mountTerminal()
    await flushPromises()
    // 票据还没回来就隐藏页签，generation 递增
    await w.setProps({ active: false })
    release({ ticket: 'late' })
    await flushPromises()
    expect(FakeSocket.all.length).toBe(0)
    w.unmount()
  })

  it('票据失败返回时若连接尝试已过期，不覆盖当前状态', async () => {
    let fail!: (e: Error) => void
    api.sandboxes.terminalTicket.mockImplementationOnce(() => new Promise((_, rej) => { fail = rej }))
    const w = mountTerminal()
    await flushPromises()
    await w.setProps({ active: false })
    fail(new Error('late'))
    await flushPromises()
    expect(w.text()).not.toContain('无法获取终端票据')
    expect(w.get('[data-testid="terminal-status"]').text()).toBe('连接中…')
    w.unmount()
  })

  it('socket 打开时若已被取代则忽略所有回调', async () => {
    const w = mountTerminal()
    await flushPromises()
    const ws = FakeSocket.all[0]
    const { onopen, onmessage, onerror, onclose } = ws
    await w.setProps({ active: false })
    onopen?.()
    onmessage?.({ data: new Uint8Array([65]).buffer })
    onerror?.()
    onclose?.()
    await flushPromises()
    expect(termWrites).toEqual([])
    w.unmount()
  })

  it('终端宿主在加载 xterm 期间消失时不创建终端', async () => {
    // 在 xterm 还在动态加载时卸载，ensureTerminal 的二次检查应直接返回
    const w = mountTerminal()
    w.unmount()
    await flushPromises()
    expect(terms.length).toBe(0)
  })

  it('连接建立后 socket 关闭事件不会覆盖 error 状态', async () => {
    const w = mountTerminal()
    await flushPromises()
    const ws = FakeSocket.all[0]
    ws.readyState = 1
    ws.onopen?.()
    ws.onerror?.()
    ws.onclose?.()
    await flushPromises()
    expect(w.text()).toContain('连接失败')
    // 重新连接后，旧 socket 的回调一律被忽略（generation 已变）。
    await w.findAll('button').find((b) => b.text().includes('重新连接'))!.trigger('click')
    await flushPromises()
    ws.onmessage?.({ data: JSON.stringify({ type: 'exit', code: 9 }) })
    ws.onclose?.()
    await flushPromises()
    expect(w.text()).not.toContain('exit 9')
    w.unmount()
  })
})
