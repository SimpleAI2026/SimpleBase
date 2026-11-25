// 文件说明：http-api.ts — 控制台访问后端的唯一 API 实现（`Api` 的实现对象）。
//
// 按域拆成 http-api/platform.ts、http-api/data.ts、http-api/agent.ts 三个部分，这里只做组装；
// 页面与组件一律从 `@/services/api` 取 `api`，不要直接 import 各域文件。
// 每个 URL 的拼装在 `@/services/paths`，wire → UI 字段的转换在 `@/services/converters/*`。
import type { Api } from '@/services/types'
import { agentApi } from '@/services/http-api/agent'
import { dataApi } from '@/services/http-api/data'
import { platformApi } from '@/services/http-api/platform'

export const httpApi: Api = {
  ...platformApi,
  ...dataApi,
  ...agentApi
}
