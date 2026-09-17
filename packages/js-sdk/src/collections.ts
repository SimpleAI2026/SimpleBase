import type { HttpClient } from './http.js'
import { projectPath } from './http.js'
import type { Json } from './types.js'

const NAME_RE = /^[A-Za-z_][A-Za-z0-9_]*$/

function assertCollectionName(name: string) {
  if (!NAME_RE.test(name)) {
    throw new Error(`invalid collection name: ${name}`)
  }
}

export interface CollectionApi {
  list(): Promise<{ rows: Record<string, unknown>[] }>
  insert(doc: Record<string, Json | undefined>): Promise<Record<string, unknown>>
  update(id: string, doc: Record<string, Json | undefined>): Promise<Record<string, unknown>>
  remove(id: string): Promise<void>
}

export interface CollectionsApi {
  list(): Promise<{ collections: string[] }>
  create(name: string): Promise<void>
  collection(name: string): CollectionApi
}

export function createCollectionsApi(http: HttpClient, databaseId: string): CollectionsApi {
  const db = encodeURIComponent(databaseId)
  const root = () =>
    projectPath(http.projectId, `/databases/${db}/data/collections`)

  function collection(name: string): CollectionApi {
    assertCollectionName(name)
    const col = encodeURIComponent(name)
    return {
      list() {
        return http.request('GET', `${root()}/${col}`)
      },
      insert(doc) {
        return http.request('POST', `${root()}/${col}/documents`, { body: doc })
      },
      update(id, doc) {
        return http.request(
          'PUT',
          `${root()}/${col}/documents/${encodeURIComponent(id)}`,
          { body: doc }
        )
      },
      remove(id) {
        return http.request(
          'DELETE',
          `${root()}/${col}/documents/${encodeURIComponent(id)}`
        )
      }
    }
  }

  return {
    list() {
      return http.request('GET', root())
    },
    async create(name) {
      assertCollectionName(name)
      await http.request('POST', root(), { body: { name } })
    },
    collection
  }
}
