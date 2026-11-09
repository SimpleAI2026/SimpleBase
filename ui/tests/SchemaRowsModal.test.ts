import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SchemaRowsModal from '@/components/databases/SchemaRowsModal.vue'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'

vi.mock('@/services/api', async () => {
  const mocked = await import('@/test/api-mock')
  return { api: mocked.api }
})

const page = {
  table: 'sys_users',
  columns: [
    { name: 'id', type: 'VARCHAR', nullable: false },
    { name: 'password_hash', type: 'VARCHAR', nullable: false, sensitive: true },
    { name: 'note', type: 'VARCHAR', nullable: true },
    { name: 'meta', type: 'JSON', nullable: true },
    { name: 'blob', type: 'BLOB', nullable: true },
    { name: 'body', type: 'TEXT', nullable: true },
    { name: 'flag', type: 'BOOLEAN', nullable: true }
  ],
  rows: [['u1', null, 'hello', { a: 1 }, 42, 'long', true], ['u2', null, null, null, null, null, false]],
  limit: 50,
  offset: 0,
  total: 80
}

function mountModal(open = true) {
  return mount(SchemaRowsModal, {
    props: {
      open,
      projectId: 'sb-admin',
      databaseId: 'sys',
      table: 'sys_users',
      readonly: true
    },
    attachTo: document.body,
    global: {
      stubs: {
        ...uiStubs,
        SbModal: false,
        SchemaRowsModal: false,
        TablePager: false
      }
    }
  })
}

describe('SchemaRowsModal', () => {
  beforeEach(() => {
    document.body.innerHTML = ''
    resetApiMocks()
    api.databases.tableRows.mockResolvedValue(page)
  })

  it('loads the first page, hides credential values, and pages on the server', async () => {
    const wrapper = mountModal()
    expect(document.body.querySelector('[aria-busy="true"]')).toBeTruthy()
    await flushPromises()
    expect(api.databases.tableRows).toHaveBeenCalledWith('sb-admin', 'sys', 'sys_users', { limit: 50, offset: 0 })
    expect(document.body.textContent).toContain('只读')
    expect(document.body.textContent).toContain('已隐藏')
    expect(document.body.textContent).toContain('NULL')
    expect(document.body.textContent).toContain('{"a":1}')
    expect(document.body.textContent).toContain('42')
    expect(document.body.textContent).toContain('false')
    expect(document.body.textContent).not.toContain('secret')
    const note = Array.from(document.querySelectorAll('td')).find((cell) => cell.textContent?.includes('hello'))
    expect(note?.className).toContain('text-left')

    api.databases.tableRows.mockResolvedValueOnce({
      ...page,
      rows: [['u3', null, 'next']],
      offset: 50
    })
    const next = Array.from(document.querySelectorAll('button')).find((button) => button.textContent?.includes('下一页'))
    expect(next).toBeTruthy()
    await next!.click()
    await flushPromises()
    expect(api.databases.tableRows).toHaveBeenLastCalledWith('sb-admin', 'sys', 'sys_users', { limit: 50, offset: 50 })
    expect(document.body.textContent).toContain('next')

    await wrapper.find('.dialog-close').trigger('click')
    expect(wrapper.emitted('update:open')?.[0]).toEqual([false])
    wrapper.unmount()
  })

  it('shows an empty table and a retryable error', async () => {
    api.databases.tableRows.mockResolvedValueOnce({ ...page, rows: [], total: 0 })
    const wrapper = mountModal()
    await flushPromises()
    expect(document.body.textContent).toContain('这张表没有行')

    api.databases.tableRows.mockRejectedValueOnce(new Error('rows-down'))
    await wrapper.setProps({ table: 'sys_api_keys' })
    await flushPromises()
    expect(document.body.textContent).toContain('rows-down')
    api.databases.tableRows.mockResolvedValueOnce({
      ...page,
      table: 'sys_api_keys',
      rows: [['k1', null, 'ok']],
      total: 1
    })
    const retry = Array.from(document.querySelectorAll('button')).find((button) => button.textContent?.trim() === '重试')
    expect(retry).toBeTruthy()
    await retry!.click()
    await flushPromises()
    expect(document.body.textContent).toContain('ok')
    wrapper.unmount()
  })
})
