import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ApiPreview from '@/pages/ApiPreview.vue'
import { useProjectStore } from '@/stores/project'
import { api, resetApiMocks } from '@/test/api-mock'
import { mountWithApp } from '@/test/helpers'

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

const sourceDocument = {
  openapi: '3.0.3',
  paths: {
    '/v1/projects/{projectID}/databases/{databaseID}/data/collections/{collection}': {
      get: {
        operationId: 'getCollection',
        'x-tryable': true,
        parameters: [
          { name: 'projectID', in: 'path', required: true, 'x-sb-source': { kind: 'project' } },
          { name: 'databaseID', in: 'path', required: true, 'x-sb-source': { kind: 'databases' } },
          { name: 'collection', in: 'path', required: true, 'x-sb-source': { kind: 'collections', depends_on: ['databaseID'] } }
        ]
      }
    }
  }
}

const stubs = { ConfirmAction: false }
// 不桩 Combobox：用真实组件，断言打在真实 DOM 上
const realCombobox = {
  Combobox: false,
  ComboboxAnchor: false,
  ComboboxInput: false,
  ComboboxList: false,
  ComboboxViewport: false,
  ComboboxEmpty: false,
  ComboboxGroup: false,
  ComboboxItem: false
}

function db(id: string, name = id) {
  return { id, name, engine: 'duckdb', status: 'ready' }
}

describe('ApiPreview', () => {
  beforeEach(() => {
    sessionStorage.clear()
    resetApiMocks()
    api.preview.document.mockResolvedValue(document)
    api.preview.request.mockResolvedValue({ status: 200, body: '{"ok":true}' })
  })

  it('loads routes, validates parameters and runs GET without confirmation', async () => {
    const { wrapper } = await mountWithApp(ApiPreview, { stubs })
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
    const { wrapper: failed } = await mountWithApp(ApiPreview)
    await flushPromises()
    expect(failed.text()).toContain('offline')
    api.preview.document.mockResolvedValue(document)
    await failed.findAll('button').find((button) => button.text() === '重试')!.trigger('click')
    await flushPromises()

    const { wrapper } = await mountWithApp(ApiPreview, { stubs })
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
    const { wrapper: empty } = await mountWithApp(ApiPreview)
    await flushPromises()
    expect(empty.text()).not.toContain('发送请求')
    await (empty.vm as unknown as { runRequest: () => Promise<void> }).runRequest()
    api.preview.document.mockResolvedValue({ openapi: '3.0.3', paths: { '/v1/projects/{projectID}/realtime': { get: { operationId: 'rt', 'x-tryable': false, parameters: [{ name: 'projectID', in: 'path', required: true }] } } } })
    const { wrapper } = await mountWithApp(ApiPreview)
    await flushPromises()
    const vm = wrapper.vm as unknown as { requestRun: () => void; runRequest: () => Promise<void> }
    vm.requestRun()
    expect(api.preview.request).not.toHaveBeenCalled()
  })

  describe('参数下拉（x-sb-source）', () => {
    beforeEach(() => {
      api.preview.document.mockResolvedValue(sourceDocument)
    })

    function input(wrapper: Awaited<ReturnType<typeof mountWithApp>>['wrapper'], name: string) {
      return wrapper.get<HTMLInputElement>(`input[placeholder="${name}"]`)
    }

    it('projectID 默认为当前项目', async () => {
      const { wrapper } = await mountWithApp(ApiPreview, { stubs: { ...stubs, ...realCombobox }, projectId: 'prj-cur' })
      await flushPromises()
      expect(input(wrapper, 'projectID').element.value).toBe('prj-cur')
    })

    it('只有一个库时 databaseID 自动填入，并拉取 collection 选项', async () => {
      api.databases.list.mockResolvedValue([db('db1')])
      api.db.collections.mockResolvedValue(['users', 'orders'])
      const { wrapper } = await mountWithApp(ApiPreview, { stubs: { ...stubs, ...realCombobox } })
      await flushPromises()
      expect(input(wrapper, 'databaseID').element.value).toBe('db1')
      expect(api.db.collections).toHaveBeenCalledWith('testprj1', 'db1')
      expect(input(wrapper, 'collection').element.disabled).toBe(false)
    })

    it('有两个库时不自动填入，依赖参数为空时 collection 禁用并提示', async () => {
      api.databases.list.mockResolvedValue([db('db1'), db('db2')])
      const { wrapper } = await mountWithApp(ApiPreview, { stubs: { ...stubs, ...realCombobox } })
      await flushPromises()
      expect(input(wrapper, 'databaseID').element.value).toBe('')
      expect(input(wrapper, 'collection').element.disabled).toBe(true)
      expect(wrapper.text()).toContain('请先选择 databaseID')
      expect(api.db.collections).not.toHaveBeenCalled()
    })

    it('换库后 collection 被清空并按新库重新拉取', async () => {
      api.databases.list.mockResolvedValue([db('db1'), db('db2')])
      api.db.collections.mockImplementation(async (_p: string, d: string) => (d === 'db1' ? ['a'] : ['b']))
      const { wrapper } = await mountWithApp(ApiPreview, { stubs: { ...stubs, ...realCombobox } })
      await flushPromises()
      const vm = wrapper.vm as unknown as { onParamCommit: (n: string, v: string) => void }
      vm.onParamCommit('databaseID', 'db1')
      await flushPromises()
      vm.onParamCommit('collection', 'a')
      await flushPromises()
      expect(input(wrapper, 'collection').element.value).toBe('a')
      vm.onParamCommit('databaseID', 'db2')
      await flushPromises()
      expect(input(wrapper, 'collection').element.value).toBe('')
      expect(api.db.collections).toHaveBeenLastCalledWith('testprj1', 'db2')
    })

    it('数据源失败时退化为自由输入并提示，不弹 toast', async () => {
      api.databases.list.mockRejectedValue(new Error('boom'))
      const { toast } = await import('vue-sonner')
      const { wrapper } = await mountWithApp(ApiPreview, { stubs: { ...stubs, ...realCombobox } })
      await flushPromises()
      expect(wrapper.text()).toContain('候选项加载失败，可直接输入')
      expect(input(wrapper, 'databaseID').element.disabled).toBe(false)
      await input(wrapper, 'databaseID').setValue('typed-db')
      expect(input(wrapper, 'databaseID').element.value).toBe('typed-db')
      expect(toast.error).not.toHaveBeenCalled()
    })

    it('手动输入任意值能正常发出请求', async () => {
      api.databases.list.mockResolvedValue([db('db1'), db('db2')])
      const { wrapper } = await mountWithApp(ApiPreview, { stubs: { ...stubs, ...realCombobox } })
      await flushPromises()
      await input(wrapper, 'databaseID').setValue('ghost')
      const vm = wrapper.vm as unknown as { onParamCommit: (n: string, v: string) => void }
      vm.onParamCommit('databaseID', 'ghost')
      await flushPromises()
      await input(wrapper, 'collection').setValue('nope')
      await wrapper.findAll('button').find((b) => b.text() === '发送请求')!.trigger('click')
      await flushPromises()
      expect(api.preview.request).toHaveBeenCalledWith(
        'GET',
        '/v1/projects/testprj1/databases/ghost/data/collections/nope',
        undefined
      )
    })

    it('切换项目后重新注入默认值', async () => {
      api.databases.list.mockResolvedValue([db('db1')])
      const { wrapper } = await mountWithApp(ApiPreview, { stubs: { ...stubs, ...realCombobox }, projectId: 'prj-a' })
      await flushPromises()
      expect(input(wrapper, 'projectID').element.value).toBe('prj-a')
      useProjectStore().setProject('prj-b', 'B')
      await flushPromises()
      expect(input(wrapper, 'projectID').element.value).toBe('prj-b')
      expect(api.databases.list).toHaveBeenLastCalledWith('prj-b')
    })

    it('同一会话内按 kind 记住上次选择，切换端点后复用', async () => {
      api.databases.list.mockResolvedValue([db('db1'), db('db2')])
      api.preview.document.mockResolvedValue({
        openapi: '3.0.3',
        paths: {
          ...sourceDocument.paths,
          '/v1/projects/{projectID}/databases/{databaseID}/schema': {
            get: {
              operationId: 'schema',
              'x-tryable': true,
              parameters: [{ name: 'databaseID', in: 'path', required: true, 'x-sb-source': { kind: 'databases' } }]
            }
          }
        }
      })
      const { wrapper } = await mountWithApp(ApiPreview, { stubs: { ...stubs, ...realCombobox } })
      await flushPromises()
      const vm = wrapper.vm as unknown as { onParamCommit: (n: string, v: string) => void }
      vm.onParamCommit('databaseID', 'db2')
      await flushPromises()
      expect(JSON.parse(sessionStorage.getItem('sb.apiPreview.params.testprj1') || '{}')).toEqual({ databases: 'db2' })
      const other = wrapper.findAll('button').find((b) => b.text().includes('/schema'))!
      await other.trigger('click')
      await flushPromises()
      expect(input(wrapper, 'databaseID').element.value).toBe('db2')
    })
  })
})
