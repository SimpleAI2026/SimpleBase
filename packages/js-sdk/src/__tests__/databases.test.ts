import { describe, expect, it, vi } from 'vitest'
import { createClient } from '../index.js'

describe('databases data model and schema', () => {
  it('sends init SQL and calls schema routes', async () => {
    const fetchMock = vi.fn(async (url: string) => {
      const path = String(url)
      if (path.endsWith('/schema/tables')) {
        return new Response(JSON.stringify({ name: 't', columns: [] }), { status: 201 })
      }
      if (path.endsWith('/schema/columns')) {
        return new Response(JSON.stringify({ name: 'email', type: 'VARCHAR', nullable: true }), { status: 201 })
      }
      if (path.endsWith('/schema')) {
        return new Response(JSON.stringify({ tables: [{ name: 't', columns: [] }] }), { status: 200 })
      }
      return new Response(JSON.stringify({ id: 'db', data_model: 'sql' }), { status: 201 })
    })
    const sb = createClient({
      url: 'http://example.test',
      apiKey: 'k',
      projectId: 'p',
      fetch: fetchMock as unknown as typeof fetch
    })

    const created = await sb.databases.create({
      name: 'shop',
      data_model: 'sql',
      init_sql: 'CREATE TABLE t (id INTEGER)'
    })
    expect(created.data_model).toBe('sql')
    const createCall = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(JSON.parse(String(createCall[1].body))).toEqual({
      name: 'shop',
      data_model: 'sql',
      init_sql: 'CREATE TABLE t (id INTEGER)'
    })

    const schema = await sb.databases.schema('db/1')
    expect(schema.tables[0].name).toBe('t')
    expect(String((fetchMock.mock.calls[1] as unknown as [string])[0])).toContain('/databases/db%2F1/schema')

    await sb.databases.createTable('db/1', { name: 't', columns: [{ name: 'id', type: 'INTEGER' }] })
    expect(String((fetchMock.mock.calls[2] as unknown as [string])[0])).toContain('/schema/tables')

    const column = await sb.databases.addColumn('db/1', { table: 't', name: 'email', type: 'VARCHAR', nullable: true })
    expect(column.name).toBe('email')
    expect(String((fetchMock.mock.calls[3] as unknown as [string])[0])).toContain('/schema/columns')
  })
})
