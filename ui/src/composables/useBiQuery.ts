// useBiQuery.ts — 单个聚合请求的生命周期（planv6.7 §4.2）。
//
// 每个 widget 一个实例：pending / error / 结果，自带竞态令牌（旧响应不覆盖新请求）。
// 全局并发上限 4（BiQueue），避免默认看板首屏同时打满只读 lease。

import { ref, type Ref } from 'vue'
import { api } from '@/services/api'
import { errorMessage } from '@/utils/format'
import type { AggregateQuery, AggregateResult, ApiError } from '@/services/types'
import type { BiSourceKind } from '@/utils/bi-source'

/** 全局并发上限（planv6.7 §3.3）。 */
export const MAX_CONCURRENT_QUERIES = 4

/** 排队中的任务在轮到它之前已被作废（切数据源 / 条件变化 / 卸载）：跳过，不再发请求。 */
export class BiStaleError extends Error {
  constructor() {
    super('stale query skipped')
    this.name = 'BiStaleError'
  }
}

/** 简单的并发闸：同时在飞的请求不超过 limit，其余排队，保持先进先出。 */
export class BiQueue {
  private active = 0
  private waiting: Array<() => void> = []
  constructor(private readonly limit = MAX_CONCURRENT_QUERIES) {}

  get inFlight(): number {
    return this.active
  }
  get queued(): number {
    return this.waiting.length
  }

  /**
   * stale 在「轮到它执行」那一刻再问一次：已过期就不执行 task，把名额让给下一个等待者并抛 BiStaleError。
   * 否则旧筛选 / 旧数据源的排队请求会照样发出去，白白占用只读 lease、拖慢新看板。
   */
  async run<T>(task: () => Promise<T>, stale?: () => boolean): Promise<T> {
    if (this.active >= this.limit) {
      await new Promise<void>((resolve) => this.waiting.push(resolve))
      if (stale?.()) {
        this.waiting.shift()?.()
        throw new BiStaleError()
      }
    }
    this.active++
    try {
      return await task()
    } finally {
      this.active--
      this.waiting.shift()?.()
    }
  }
}

/** 页面级共享队列。 */
export const biQueue = new BiQueue()

export interface BiQueryTarget {
  projectId: string
  databaseId: string
  /** 数据源名：集合名或 SQL 表名。 */
  collection: string
  /** 缺省为集合。 */
  kind?: BiSourceKind
}

/** 按数据源种类选择聚合端点：集合走文档聚合，SQL 表走表聚合。两者参数与响应同形。 */
function runAggregate(target: BiQueryTarget, query: AggregateQuery): Promise<AggregateResult> {
  return target.kind === 'table'
    ? api.databases.aggregateTable(target.projectId, target.databaseId, target.collection, query)
    : api.db.aggregate(target.projectId, target.databaseId, target.collection, query)
}

export interface BiQueryState {
  result: Ref<AggregateResult | null>
  pending: Ref<boolean>
  error: Ref<string>
  /** 后端错误码（如 schema_not_registered / filter_scan_limit_exceeded），供上层区分提示。 */
  errorCode: Ref<string>
  run: (target: BiQueryTarget, query: AggregateQuery) => Promise<void>
  /** 作废在途请求（切换集合 / 卸载时调用）。 */
  cancel: () => void
}

/** 创建一个查询状态。queue 可注入，测试里用独立队列。 */
export function useBiQuery(fallback: string, queue: BiQueue = biQueue): BiQueryState {
  const result = ref<AggregateResult | null>(null)
  const pending = ref(false)
  const error = ref('')
  const errorCode = ref('')
  let token = 0

  async function run(target: BiQueryTarget, query: AggregateQuery) {
    const mine = ++token
    pending.value = true
    error.value = ''
    errorCode.value = ''
    try {
      const data = await queue.run(() => runAggregate(target, query), () => mine !== token)
      if (mine !== token) return
      result.value = data
    } catch (e) {
      if (mine !== token) return
      result.value = null
      // 空 message 会让错误区一片空白：统一回落到调用方给的兜底文案。
      error.value = errorMessage(e, fallback) || fallback
      errorCode.value = (e as ApiError)?.code ?? ''
    } finally {
      if (mine === token) pending.value = false
    }
  }

  function cancel() {
    token++
    pending.value = false
  }

  return { result, pending, error, errorCode, run, cancel }
}
