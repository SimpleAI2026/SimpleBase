import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import SchemaPanel from '@/components/databases/SchemaPanel.vue'
import { api, resetApiMocks } from '@/test/api-mock'
import type { DatabaseItem } from '@/services/types'

vi.mock('@/services/api', async () => {
  const mocked = await import('@/test/api-mock')
  return { api: mocked.api }
})

const database: DatabaseItem = {
  id: 'db-1',
  name: 'shop',
  status: 'ready',
  createdAt: '2024-01-01T00:00:00Z',
  updatedAt: '2024-01-01T00:00:00Z',
  dataModel: 'sql'
}

function mountPanel(props: { readonly?: boolean; database?: DatabaseItem } = {}) {
  return mount(SchemaPanel, {
    props: {
      projectId: 'proj',
      database: props.database ?? database,
      readonly: props.readonly
    }
  })
}

describe('SchemaPanel', () => {
  beforeEach(() => {
    resetApiMocks()
  })

  it('shows a loading state, then an empty schema', async () => {
    let resolveSchema: (value: { tables: [] }) => void = () => undefined
    api.databases.schema.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveSchema = resolve
        })
    )
    const wrapper = mountPanel()
    expect(wrapper.find('[aria-busy="true"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('还没有表')
    expect(wrapper.find('form').exists()).toBe(false)
    resolveSchema({ tables: [] })
    await flushPromises()
    expect(wrapper.text()).toContain('还没有表')
    expect(wrapper.find('form.create-table').exists()).toBe(true)
    expect(wrapper.get('form.add-column button[type="submit"]').attributes('disabled')).toBeDefined()
    const vm = wrapper.vm as unknown as { submitColumn: () => Promise<void> }
    await vm.submitColumn()
    expect(api.databases.addColumn).not.toHaveBeenCalled()
  })

  it('toasts when the schema request fails and reloads for another database', async () => {
    api.databases.schema.mockRejectedValueOnce(new Error('schema-down'))
    const wrapper = mountPanel()
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('schema-down')
    api.databases.schema.mockResolvedValueOnce({ tables: [] })
    await wrapper.findAll('button').find((button) => button.text() === '重试')!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('还没有表')

    api.databases.schema.mockResolvedValueOnce({
      tables: [{ name: 'orders', columns: [{ name: 'id', type: 'INTEGER', nullable: true }] }]
    })
    await wrapper.setProps({
      database: { ...database, id: 'db-2' }
    })
    await flushPromises()
    expect(api.databases.schema).toHaveBeenCalledWith('proj', 'db-2')
    expect(wrapper.text()).toContain('orders')
    expect(wrapper.text()).not.toContain('NOT NULL')
    expect((wrapper.get('#schema-add-table').element as HTMLSelectElement).value).toBe('orders')
  })

  it('hides forms when readonly and marks NOT NULL columns', async () => {
    api.databases.schema.mockResolvedValue({
      tables: [{ name: 'orders', columns: [{ name: 'id', type: 'INTEGER', nullable: false, sensitive: true }] }]
    })
    const wrapper = mountPanel({ readonly: true })
    await flushPromises()
    expect(wrapper.text()).toContain('NOT NULL')
    expect(wrapper.text()).toContain('已隐藏')
    expect(wrapper.text()).toContain('系统库只读，不能新建表或修改字段')
    expect(wrapper.find('form').exists()).toBe(false)
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(api.databases.tableRows).toHaveBeenCalledWith('proj', 'db-1', 'orders', { limit: 50, offset: 0 })
    const modal = wrapper.findComponent({ name: 'SchemaRowsModal' })
    modal.vm.$emit('update:open', false)
    await flushPromises()
    expect(modal.props('open')).toBe(false)
  })

  it('validates table and column names, then creates a table', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.get('form.create-table').trigger('submit')
    expect(toast.warning).toHaveBeenCalledWith('请输入表名')

    await wrapper.get('#schema-table-name').setValue('orders')
    await wrapper.get('form.create-table').trigger('submit')
    expect(toast.warning).toHaveBeenCalledWith('请输入字段名')

    await wrapper.get('button[type="button"]').trigger('click')
    await wrapper.get('#schema-col-0').setValue('id')
    await wrapper.get('form.create-table').trigger('submit')
    expect(toast.warning).toHaveBeenCalledWith('请输入字段名')
    expect(api.databases.createTable).not.toHaveBeenCalled()

    await wrapper.get('#schema-type-0').setValue('INTEGER')
    await wrapper.get('#schema-null-0').setValue(false)
    await wrapper.get('#schema-col-1').setValue('email')
    api.databases.createTable.mockResolvedValue({ name: 'orders', columns: [] })
    api.databases.schema.mockResolvedValue({
      tables: [
        {
          name: 'orders',
          columns: [
            { name: 'id', type: 'INTEGER', nullable: false },
            { name: 'email', type: 'VARCHAR', nullable: true }
          ]
        }
      ]
    })
    await wrapper.get('form.create-table').trigger('submit')
    await flushPromises()
    expect(api.databases.createTable).toHaveBeenCalledWith('proj', 'db-1', {
      name: 'orders',
      columns: [
        { name: 'id', type: 'INTEGER', nullable: false },
        { name: 'email', type: 'VARCHAR', nullable: true }
      ]
    })
    expect(toast.success).toHaveBeenCalledWith('表 orders 已创建')
    expect((wrapper.get('#schema-table-name').element as HTMLInputElement).value).toBe('')
    expect(wrapper.find('#schema-col-1').exists()).toBe(false)
    expect(wrapper.text()).toContain('NOT NULL')
  })

  it('toasts when creating a table fails', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.get('#schema-table-name').setValue('orders')
    await wrapper.get('#schema-col-0').setValue('id')
    api.databases.createTable.mockRejectedValueOnce(new Error('create-table'))
    await wrapper.get('form.create-table').trigger('submit')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('create-table')
    expect(wrapper.get('form.create-table button[type="submit"]').attributes('disabled')).toBeUndefined()
  })

  it('caps draft columns at 16', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    for (let i = 0; i < 15; i += 1) {
      await wrapper.get('form.create-table button[type="button"]').trigger('click')
    }
    expect(wrapper.find('#schema-col-15').exists()).toBe(true)
    await wrapper.get('form.create-table button[type="button"]').trigger('click')
    expect(toast.warning).toHaveBeenCalledWith('最多 16 个字段')
    expect(wrapper.find('#schema-col-16').exists()).toBe(false)
  })

  it('adds a column to an existing table and reports errors', async () => {
    api.databases.schema.mockResolvedValue({
      tables: [
        { name: 'orders', columns: [{ name: 'id', type: 'INTEGER', nullable: true }] },
        { name: 'notes', columns: [{ name: 'id', type: 'INTEGER', nullable: true }] }
      ]
    })
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.get('form.add-column button[type="submit"]').attributes('disabled')).toBeUndefined()
    await wrapper.get('form.add-column').trigger('submit')
    expect(toast.warning).toHaveBeenCalledWith('请输入字段名')

    await wrapper.get('#schema-add-table').setValue('notes')
    await wrapper.get('#schema-add-name').setValue('note')
    await wrapper.get('#schema-add-type').setValue('JSON')
    await wrapper.get('#schema-add-nullable').setValue(false)
    api.databases.addColumn.mockRejectedValueOnce(new Error('add-column'))
    await wrapper.get('form.add-column').trigger('submit')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('add-column')

    let release: (value: { name: string; type: string; nullable: boolean }) => void = () => undefined
    api.databases.addColumn.mockImplementation(
      () =>
        new Promise((resolve) => {
          release = resolve
        })
    )
    api.databases.schema.mockResolvedValue({
      tables: [
        {
          name: 'orders',
          columns: [
            { name: 'id', type: 'INTEGER', nullable: true },
            { name: 'note', type: 'JSON', nullable: true }
          ]
        }
      ]
    })
    await wrapper.get('form.add-column').trigger('submit')
    expect(wrapper.get('form.add-column button[type="submit"]').attributes('disabled')).toBeDefined()
    release({ name: 'note', type: 'JSON', nullable: true })
    await flushPromises()
    expect(api.databases.addColumn).toHaveBeenCalledWith('proj', 'db-1', {
      table: 'notes',
      name: 'note',
      type: 'JSON',
      nullable: false
    })
    expect(toast.success).toHaveBeenCalledWith('字段 note 已添加')
    expect(wrapper.text()).toContain('note')
  })
})
