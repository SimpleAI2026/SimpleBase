import type { Api } from './types'
import { httpApi } from './http-api'

export const api: Api = httpApi

export * from './types'
