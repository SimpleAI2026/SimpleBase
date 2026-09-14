import type { Api } from './types'
import { httpApi } from './http-api'
import { mockApi } from './mock'

/** 是否使用 Mock 数据：.env 中设置 VITE_USE_MOCK=true */
export const isMock = import.meta.env.VITE_USE_MOCK === 'true'

export const api: Api = isMock ? mockApi : httpApi

export * from './types'
