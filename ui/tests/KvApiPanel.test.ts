import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import KvApiPanel from '@/components/databases/kv/KvApiPanel.vue'
import { KV_COMMANDS, KV_COMMAND_GROUPS, kvBasePath, kvCurlSnippet } from '@/components/databases/kv/kv-endpoints'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

async function mountPanel(props: Record<string, unknown> = {}) {
  const w = mount(KvApiPanel, {
    props: { projectId: 'p', ...props },
    global: { stubs: uiStubs }
  })
  await flushPromises()
  return w
}

/** 从 KV_COMMANDS 里取命令定义（模拟真实目录数据驱动 openTry） */
function cmd(name: string) {
  const c = KV_COMMANDS.find((x) => x.name === name)
  if (!c) throw new Error('unknown command ' + name)
  return c
}

describe('KvApiPanel（项目级单端点 API 子页签）', () => {
  beforeEach(() => resetApiMocks())

  it('展示 Base 路径与命令表（key 分组默认）', async () => {
    const w = await mountPanel()
    expect(w.text()).toContain('/v1/projects/p/kv')
    expect(w.text()).toContain('单端点')
    // key 分组命令可见
    expect(w.text()).toContain('SCAN')
    expect(w.text()).toContain('EXPIRE')
    w.unmount()
  })

  it('切换分组渲染对应命令', async () => {
    const w = await mountPanel()
    const vm = w.vm as any
    vm.group = 'string'
    await flushPromises()
    expect(w.text()).toContain('MSET')
    expect(w.text()).not.toContain('EXPIRE')
    w.unmount()
  })

  it('复制 Base 路径 / curl 成功 toast；clipboard 不可用时降级 execCommand', async () => {
    const w = await mountPanel()
    const vm = w.vm as any
    await vm.copy('plain-text', '已复制')
    await flushPromises()
    expect(toast.success).toHaveBeenCalledWith('已复制')
    const writeText = vi.fn().mockRejectedValue(new Error('denied'))
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    await vm.copy('fallback', '已复制降级')
    await flushPromises()
    expect(writeText).toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalledWith('已复制降级')
    w.unmount()
  })

  it('试调用弹窗：打开时预填默认值并生成请求体预览', async () => {
    const w = await mountPanel()
    const vm = w.vm as any
    vm.openTry(cmd('SET'))
    expect(vm.tryOpen).toBe(true)
    expect(vm.tryKey).toBe('session:1001')
    expect(vm.tryValue).toBe('hello')
    // 请求体预览替换占位符
    expect(vm.tryBodyPreview).toContain('"argvs": [')
    expect(vm.tryBodyPreview).toContain('"SET"')
    expect(vm.tryBodyPreview).toContain('"session:1001"')
    w.unmount()
  })

  it('runTry：占位符替换后走 api.kv.exec 并展示响应', async () => {
    api.kv.exec.mockResolvedValueOnce('OK')
    const w = await mountPanel()
    const vm = w.vm as any
    vm.openTry(cmd('SET'))
    vm.tryKey = 'session:9'
    vm.tryValue = 'v9'
    await vm.runTry()
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', {
      type: 'cmd',
      argvs: ['SET', 'session:9', 'v9']
    })
    expect(vm.tryResult).toContain('OK')
    // SET 是写命令 → emit changed
    expect(w.emitted('changed')).toBeTruthy()
    w.unmount()
  })

  it('runTry：读命令不 emit changed；失败展示错误；非 Error 走 String(e)', async () => {
    const w = await mountPanel()
    const vm = w.vm as any
    api.kv.exec.mockResolvedValueOnce(null)
    vm.openTry(cmd('GET'))
    vm.tryKey = 'missing'
    await vm.runTry()
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['GET', 'missing'] })
    // 读命令不 emit
    expect(w.emitted('changed')).toBeFalsy()
    // 响应 null → 显示 "null"
    expect(vm.tryResult).toBe('null')

    // Error 实例
    api.kv.exec.mockRejectedValueOnce(new Error('not found'))
    await vm.runTry()
    expect(vm.tryResult).toContain('not found')
    expect(vm.trying).toBe(false)

    // 非 Error 拒绝
    api.kv.exec.mockRejectedValueOnce('raw-fail')
    await vm.runTry()
    expect(vm.tryResult).toContain('raw-fail')
    w.unmount()
  })

  it('readonly 时写命令按钮禁用，读命令可用', async () => {
    const w = await mountPanel({ readonly: true })
    const vm = w.vm as any
    vm.group = 'key'
    await flushPromises()
    // 写命令（DEL）按钮禁用
    const delRow = w.findAll('tr').find((r) => r.text().includes('DEL'))!
    const delBtns = delRow.findAll('button')
    expect((delBtns.find((b) => b.text() === '试调用')!.element as HTMLButtonElement).disabled).toBe(true)
    // 读命令（TYPE）可用
    const typeRow = w.findAll('tr').find((r) => r.text().includes('TYPE'))!
    const typeBtns = typeRow.findAll('button')
    expect((typeBtns.find((b) => b.text() === '试调用')!.element as HTMLButtonElement).disabled).toBe(false)
    w.unmount()
  })

  it('模板交互：复制 Base / 复制 curl / 试调用打开弹窗 / 执行 / 取消', async () => {
    const w = await mountPanel()
    // 复制 Base 路径按钮（顶部）
    await w.findAll('button').find((b) => b.text() === '复制')!.trigger('click')
    await flushPromises()
    expect(toast.success).toHaveBeenCalledWith('已复制 Base 路径')
    // 复制 curl（模板 onClick → copy）
    await w.findAll('button').find((b) => b.text() === '复制 curl')!.trigger('click')
    await flushPromises()
    expect(toast.success).toHaveBeenCalledWith('已复制 curl')
    // 试调用按钮打开弹窗：TTL 行（key 分组，含 {key} 占位）
    const ttlRow = w.findAll('tr').find((r) => r.text().includes('剩余秒数'))!
    await ttlRow.findAll('button').find((b) => b.text() === '试调用')!.trigger('click')
    await flushPromises()
    const vm = w.vm as any
    expect(vm.tryOpen).toBe(true)
    expect(vm.tryTarget?.name).toBe('TTL')
    // key 输入框双向绑定
    const keyInput = w.findAll('#kv-try-key')[0]
    await keyInput.setValue('abc')
    expect(vm.tryKey).toBe('abc')
    // 执行（SbModal ok → runTry）
    api.kv.exec.mockResolvedValueOnce(120)
    await w.find('.sb-ok').trigger('click')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', { type: 'cmd', argvs: ['TTL', 'abc'] })
    // 取消关闭
    await w.find('.sb-cancel').trigger('click')
    expect(vm.tryOpen).toBe(false)
    w.unmount()
  })

  it('弹窗内 Value 输入双向绑定；不含占位符的命令不渲染输入', async () => {
    const w = await mountPanel()
    const vm = w.vm as any
    // SET 含 {key} 和 {value}
    vm.openTry(cmd('SET'))
    await flushPromises()
    expect(w.findAll('#kv-try-key').length).toBeGreaterThan(0)
    expect(w.findAll('#kv-try-value').length).toBeGreaterThan(0)
    const valTa = w.findAll('#kv-try-value')[0]
    await valTa.setValue('vvv')
    expect(vm.tryValue).toBe('vvv')
    // DBSIZE 无占位符 → 不渲染 Key/Value 输入
    vm.openTry(cmd('DBSIZE'))
    await flushPromises()
    expect(w.findAll('#kv-try-key').length).toBe(0)
    expect(w.findAll('#kv-try-value').length).toBe(0)
    w.unmount()
  })

  it('分组切换：Tabs v-model 内联回调与分组按钮', async () => {
    const w = await mountPanel()
    const vm = w.vm as any
    // Tabs v-model 内联回调（update:modelValue → group）
    const tabs = w.findComponent({ name: 'Tabs' })
    if (tabs.exists()) {
      tabs.vm.$emit('update:modelValue', 'zset')
      await flushPromises()
      expect(vm.group).toBe('zset')
      expect(w.text()).toContain('ZADD')
    }
    w.unmount()
  })

  it('目录纯函数：kvBasePath / kvCurlSnippet / 分组完整性', () => {
    expect(kvBasePath('p')).toBe('/v1/projects/p/kv')
    expect(kvBasePath('a/b')).toBe('/v1/projects/a%2Fb/kv')
    const curl = kvCurlSnippet(cmd('SET'), 'http://x/api')
    expect(curl).toContain("curl -X POST 'http://x/api'")
    expect(curl).toContain('Authorization: Bearer <API_KEY>')
    expect(curl).toContain('"type":"cmd"')
    // 每个命令都归属合法分组
    const groups = KV_COMMAND_GROUPS.map((g) => g.value)
    for (const c of KV_COMMANDS) expect(groups).toContain(c.group)
  })
})
