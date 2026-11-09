export class SimpleBaseError extends Error {
  readonly status: number
  readonly code: string
  readonly requestId?: string
  readonly details?: unknown

  constructor(opts: {
    message: string
    status: number
    code: string
    requestId?: string
    details?: unknown
  }) {
    super(opts.message)
    this.name = 'SimpleBaseError'
    this.status = opts.status
    this.code = opts.code
    this.requestId = opts.requestId
    this.details = opts.details
  }
}

export function errorFromResponse(
  status: number,
  body: unknown,
  requestId?: string
): SimpleBaseError {
  const b = (body && typeof body === 'object' ? body : {}) as Record<string, unknown>
  const errObj = (b.error && typeof b.error === 'object' ? b.error : b) as Record<string, unknown>
  const code = String(errObj.code || b.code || 'http_error')
  const message = String(errObj.message || b.message || `HTTP ${status}`)
  const rid = requestId || (typeof b.request_id === 'string' ? b.request_id : undefined)
  return new SimpleBaseError({ message, status, code, requestId: rid, details: body })
}
