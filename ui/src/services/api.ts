import { http, wsBase } from './http'

export const api = {
  metrics: { summary: () => http.get('/metrics/summary').then(r => r.data) },
  db: {
    collections: () => http.get('/db/collections').then(r => r.data),
    rows: (c: string, q?: any) => http.get('/db/collections/' + encodeURIComponent(c), { params: q }).then(r => r.data),
    insert: (c: string, payload: any) => http.post('/db/collections/' + encodeURIComponent(c), payload).then(r => r.data),
    remove: (c: string, id: string) => http.delete('/db/collections/' + encodeURIComponent(c) + '/' + encodeURIComponent(id)).then(r => r.data)
  },
  s3: {
    list: (prefix?: string) => http.get('/s3/list', { params: { prefix } }).then(r => r.data),
    presign: (key: string) => http.get('/s3/presign', { params: { key } }).then(r => r.data),
    remove: (key: string) => http.delete('/s3/object', { params: { key } }).then(r => r.data),
    upload: (key: string, file: File) => {
      const fd = new FormData()
      fd.append('key', key)
      fd.append('file', file)
      return http.post('/s3/upload', fd).then(r => r.data)
    }
  },
  project: {
    list: () => http.get('/projects').then(r => r.data),
    create: (payload: any) => http.post('/projects', payload).then(r => r.data),
    update: (id: string, payload: any) => http.put('/projects/' + encodeURIComponent(id), payload).then(r => r.data)
  },
  faas: {
    list: () => http.get('/faas/functions').then(r => r.data),
    deploy: (name: string, file: File) => {
      const fd = new FormData()
      fd.append('name', name)
      fd.append('file', file)
      return http.post('/faas/deploy', fd).then(r => r.data)
    },
    invoke: (name: string, payload: any) => http.post('/faas/invoke/' + encodeURIComponent(name), payload).then(r => r.data)
  },
  logs: {
    streamUrl: () => wsBase + '/ws/logs'
  }
}