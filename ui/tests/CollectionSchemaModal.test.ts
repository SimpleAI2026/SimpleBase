import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import CollectionSchemaModal from '@/components/databases/CollectionSchemaModal.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

function mountSchema() {
  return mount(CollectionSchemaModal, {
    props: { open: true, projectId: 'p', databaseId: 'd', collection: 'users' },
    global: { stubs: { ...uiStubs, ConfirmAction: false } }
  })
}

describe('CollectionSchemaModal', () => {
  beforeEach(() => resetApiMocks())

  it('loads, edits, dry-runs and saves a schema', async () => {
    api.db.getCollectionSchema.mockResolvedValue({
      name: 'users',
      fields: [{ name: 'email', type: 'email', required: true, unique: true, hidden: false, readonly: false, options: { min: 3, max: 100 } }]
    })
    api.db.putCollectionSchema
      .mockResolvedValueOnce({ invalid: [], scanned: 1, truncated: false })
      .mockResolvedValueOnce({ name: 'users', fields: [] })
    const w = mountSchema()
    await flushPromises()
    expect(w.find('input').element.value).toBe('email')
    await w.findAll('button').find((b) => b.text().includes('添加字段'))!.trigger('click')
    const inputs = w.findAll('input')
    await inputs[6].setValue('age')
    await w.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(w.text()).toContain('新增 age')
    await w.findAll('.ad-ok').at(-1)!.trigger('click')
    await flushPromises()
    expect(api.db.putCollectionSchema).toHaveBeenCalledTimes(2)
    expect(w.emitted('saved')).toBeTruthy()
    expect(toast.success).toHaveBeenCalledWith('Schema 已保存')
  })

  it('blocks incompatible dry runs and reports JSON errors', async () => {
    api.db.putCollectionSchema.mockResolvedValue({ invalid: [{ id: 'r1', field: 'age', reason: 'field_type_mismatch' }], scanned: 1, truncated: false })
    const w = mountSchema()
    await flushPromises()
    await w.findAll('button').find((b) => b.text().includes('添加字段'))!.trigger('click')
    await w.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('现有数据有 1 条不兼容记录，未保存')
    expect(api.db.putCollectionSchema).toHaveBeenCalledTimes(1)
    const inputs = w.findAll('input')
    await inputs[2].setValue('{bad')
    await w.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalled()
  })

  it('handles unregistered schemas, load failures and unregister', async () => {
    api.db.getCollectionSchema.mockRejectedValue(Object.assign(new Error('not found'), { status: 404 }))
    const w = mountSchema()
    await flushPromises()
    expect(w.text()).toContain('暂无字段')
    api.db.getCollectionSchema.mockRejectedValueOnce(new Error('load'))
    await w.setProps({ collection: 'other' })
    await flushPromises()
    expect(w.text()).toContain('load')
    api.db.getCollectionSchema.mockResolvedValue({ name: 'users', fields: [] })
    await w.setProps({ collection: 'users' })
    await flushPromises()
    await w.findAll('button').find((b) => b.text() === '注销 Schema')!.trigger('click')
    await w.get('.ad-ok').trigger('click')
    await flushPromises()
    expect(api.db.deleteCollectionSchema).toHaveBeenCalled()
    api.db.deleteCollectionSchema.mockRejectedValueOnce(new Error('delete'))
    await w.findAll('button').find((b) => b.text() === '注销 Schema')!.trigger('click')
    await w.get('.ad-ok').trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('delete')
  })

  it('normalizes date/select/default options and covers truncated or failed saves', async () => {
    api.db.getCollectionSchema.mockResolvedValue({
      name: 'users',
      fields: [
        { name: 'due', type: 'date', required: false, unique: false, hidden: false, readonly: false, options: {} },
        { name: 'state', type: 'select', required: false, unique: false, hidden: false, readonly: false, options: { values: ['open'] } }
      ]
    })
    api.db.putCollectionSchema.mockResolvedValue({ invalid: [], scanned: 0, truncated: false })
    const w = mountSchema()
    await flushPromises()
    const inputs = w.findAll('input')
    await inputs[1].setValue('"2026-01-01T00:00:00Z"')
    await inputs[2].setValue('2026-01-01T00:00:00Z')
    await inputs[3].setValue('2027-01-01T00:00:00Z')
    await inputs[7].setValue('"open"')
    await inputs[11].setValue('open,closed')
    await w.get('.sb-ok').trigger('click')
    await flushPromises()
    const payload = api.db.putCollectionSchema.mock.calls.at(-1)?.[2]
    expect(payload?.fields[0].options.min).toBe('2026-01-01T00:00:00Z')
    expect(payload?.fields[1].options.values).toEqual(['open', 'closed'])

    api.db.putCollectionSchema.mockResolvedValueOnce({ invalid: [], scanned: 1, truncated: true })
    await w.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('现有数据有 0 条不兼容记录，未保存')
    api.db.putCollectionSchema.mockRejectedValueOnce('bad')
    await w.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('Schema 保存失败')
  })
})
