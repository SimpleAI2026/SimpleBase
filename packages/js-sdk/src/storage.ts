import type { HttpClient } from './http.js'
import { projectPath } from './http.js'
import type { S3ObjectMeta } from './types.js'

export type UploadBody = Blob | File | ArrayBuffer | Uint8Array | string

export interface StorageApi {
  list(opts?: { prefix?: string; refresh?: boolean }): Promise<S3ObjectMeta[]>
  upload(
    key: string,
    body: UploadBody,
    opts?: { contentType?: string; filename?: string }
  ): Promise<S3ObjectMeta>
  remove(key: string): Promise<{ ok?: boolean }>
  presign(key: string): Promise<{ url: string }>
}

function toBlob(body: UploadBody, contentType?: string): Blob {
  if (typeof Blob !== 'undefined' && body instanceof Blob) return body
  if (typeof body === 'string') {
    return new Blob([body], { type: contentType || 'text/plain' })
  }
  if (body instanceof ArrayBuffer) {
    return new Blob([body], { type: contentType || 'application/octet-stream' })
  }
  if (ArrayBuffer.isView(body)) {
    const view = body as ArrayBufferView
    const copy = new Uint8Array(view.byteLength)
    copy.set(new Uint8Array(view.buffer, view.byteOffset, view.byteLength))
    return new Blob([copy], { type: contentType || 'application/octet-stream' })
  }
  return new Blob([body as Blob], { type: contentType })
}

export function createStorageApi(http: HttpClient): StorageApi {
  const objects = () => projectPath(http.projectId, '/s3/objects')
  const presignPath = () => projectPath(http.projectId, '/s3/presign')

  return {
    list(opts) {
      return http.request('GET', objects(), {
        query: {
          prefix: opts?.prefix,
          refresh: opts?.refresh ? 1 : undefined
        }
      })
    },
    upload(key, body, opts) {
      const fd = new FormData()
      fd.set('key', key)
      const blob = toBlob(body, opts?.contentType)
      const filename = opts?.filename || key.split('/').pop() || 'file'
      fd.set('file', blob, filename)
      return http.request('POST', objects(), { formData: fd })
    },
    remove(key) {
      return http.request('DELETE', objects(), { query: { key } })
    },
    presign(key) {
      return http.request('GET', presignPath(), { query: { key } })
    }
  }
}
