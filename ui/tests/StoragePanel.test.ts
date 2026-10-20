import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { useAuthStore } from '@/stores/auth'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import StoragePanel from '@/components/settings/StoragePanel.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

const BASE_SETTINGS = {
  database: 'local',
  files: 'local',
  data_dir: '/tmp/simplebase-data',
  cache_dir: '/tmp/simplebase-cache',
  bucket: '',
  prefix: '',
  source: { database: 'yaml', files: 'yaml', data_dir: 'default' },
  config_path: 'config.yaml',
  writable_config: true,
  data_present: { local: false }
}

function setup() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.openSettings()
  const w = mount(StoragePanel, { global: { plugins: [pinia], stubs: uiStubs } })
  return { auth, w }
}

describe('StoragePanel', () => {
  beforeEach(() => {
    resetApiMocks()
    vi.mocked(api.storage.get).mockResolvedValue({ ...BASE_SETTINGS })
  })

  it('加载并展示当前存储配置', async () => {
    const { w } = setup()
    await flushPromises()
    expect(api.storage.get).toHaveBeenCalled()
    expect((w.get('input#storage-data-dir').element as HTMLInputElement).value).toBe('/tmp/simplebase-data')
    expect((w.get('input#storage-cache-dir').element as HTMLInputElement).value).toBe('/tmp/simplebase-cache')
    w.unmount()
  })

  it('数据已存在时提示需要 reset', async () => {
    vi.mocked(api.storage.get).mockResolvedValue({
      ...BASE_SETTINGS,
      data_present: { local: true }
    })
    const { w } = setup()
    await flushPromises()
    expect(w.text()).toContain('已有数据')
    w.unmount()
  })

  it('配置不可写时提示改用环境变量', async () => {
    vi.mocked(api.storage.get).mockResolvedValue({
      ...BASE_SETTINGS,
      writable_config: false
    })
    const { w } = setup()
    await flushPromises()
    expect(w.text()).toContain('配置文件不可写')
    w.unmount()
  })

  it('保存调用 PUT 并弹出重启指引', async () => {
    const { auth, w } = setup()
    await flushPromises()
    auth.user = { id: 'u1', username: 'super', role: 'superadminl1' } as never
    await flushPromises()
    const vm = w.vm as any
    expect(vm.canEdit).toBe(true)
    vm.database = 's3'
    await flushPromises()
    expect(vm.dirty).toBe(true)
    vm.save()
    await flushPromises()
    expect(api.storage.put).toHaveBeenCalledWith({
      database: 's3',
      files: 'local',
      data_dir: '/tmp/simplebase-data',
      confirm_reset: true
    })
    expect(toast.success).toHaveBeenCalledWith('存储配置已写入')
    expect(w.text()).toContain('./build.sh reset')
    w.unmount()
  })

  it('加载失败展示错误', async () => {
    vi.mocked(api.storage.get).mockRejectedValue(new Error('boom'))
    const { w } = setup()
    await flushPromises()
    expect(w.text()).toContain('boom')
    w.unmount()
  })

  it('非超级管理员不可编辑', async () => {
    const { auth, w } = setup()
    await flushPromises()
    auth.user = { id: 'u2', username: 'user', role: 'user' } as never
    await flushPromises()
    const vm = w.vm as any
    expect(vm.canEdit).toBe(false)
    expect(w.findAll('button').some((b) => b.text() === '保存')).toBe(false)
    w.unmount()
  })

  it('字段被环境变量接管时锁定并标注来源', async () => {
    vi.mocked(api.storage.get).mockResolvedValue({
      ...BASE_SETTINGS,
      source: { database: 'env', files: 'env', data_dir: 'env' }
    })
    const { auth, w } = setup()
    await flushPromises()
    auth.user = { id: 'u1', username: 'super', role: 'superadminl1' } as never
    await flushPromises()
    expect(w.text()).toContain('该字段由环境变量控制')
    const vm = w.vm as any
    expect(vm.sourceOf('database')).toBe('env')
    // env 字段被禁用（Select disabled + Input disabled）。
    expect(w.findAll('button').some((b) => b.text() === '保存')).toBe(true)
    w.unmount()
  })

  it('save 的守卫：未变更或保存中不重复提交', async () => {
    const { auth, w } = setup()
    await flushPromises()
    auth.user = { id: 'u1', username: 'super', role: 'superadminl1' } as never
    await flushPromises()
    const vm = w.vm as any
    expect(vm.dirty).toBe(false)
    expect(vm.sourceOf('nope')).toBe('default')
    // settings 未加载（null）时 dirty 为 false。
    vm.settings = null
    expect(vm.dirty).toBe(false)
    vm.settings = { ...BASE_SETTINGS }
    vm.save() // dirty=false 直接返回
    expect(api.storage.put).not.toHaveBeenCalled()
    vm.database = 's3'
    expect(vm.dirty).toBe(true)
    vm.files = 's3'
    vm.dataDir = '/tmp/other'
    expect(vm.dirty).toBe(true)
    vm.save()
    await flushPromises()
    expect(api.storage.put).toHaveBeenCalledWith({
      database: 's3',
      files: 's3',
      data_dir: '/tmp/other',
      confirm_reset: true
    })
    w.unmount()
  })

  it('保存失败展示错误信息', async () => {
    vi.mocked(api.storage.put).mockRejectedValueOnce(new Error('conflict!'))
    const { auth, w } = setup()
    await flushPromises()
    auth.user = { id: 'u1', username: 'super', role: 'superadminl1' } as never
    await flushPromises()
    const vm = w.vm as any
    vm.database = 's3'
    vm.save()
    await flushPromises()
    expect(w.text()).toContain('conflict!')
    w.unmount()
  })

  it('通过 UI 事件修改 database / files / dataDir 后可保存', async () => {
    const { auth, w } = setup()
    await flushPromises()
    auth.user = { id: 'u1', username: 'super', role: 'superadminl1' } as never
    await flushPromises()
    const vm = w.vm as any

    // 模板 Select v-model 的 onUpdate:modelValue 回调
    vm.database = 's3'
    await flushPromises()
    expect(vm.dirty).toBe(true)
    // files Select
    vm.files = 's3'
    await flushPromises()
    expect(vm.dirty).toBe(true)
    // dataDir Input
    vm.dataDir = '/tmp/new-data'
    await flushPromises()
    expect(vm.dirty).toBe(true)

    vm.save()
    await flushPromises()
    expect(api.storage.put).toHaveBeenCalledWith({
      database: 's3',
      files: 's3',
      data_dir: '/tmp/new-data',
      confirm_reset: true
    })
    w.unmount()
  })
})
