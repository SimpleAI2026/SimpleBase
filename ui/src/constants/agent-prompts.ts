/**
 * 云助手空会话的提示词（plan/planv6.0/cloud-assistant-chat-ux-plan.md §3.5）。
 *
 * 只登记 i18n key，文案在 `locales/zh-CN.ts` + `en-US.ts` 的
 * `pages.agents.prompts.<module>.<n>`。按 Agent 的 module 取，
 * 未知模块（含 `general`）统一回落到通用助手的提示词。
 *
 * 点击 = 写入输入框并聚焦，**不自动发送**，回车才执行。
 */
export interface AgentPromptSet {
  /** 对应的 Agent module（constants 里的值，小写）。 */
  module: string
  /** i18n key 后缀，按顺序展示。 */
  keys: string[]
}

/** 通用助手：覆盖数据库 / 对象存储 / 日志的只读探索，外加两条能看见「对话即操作」的技能示例。 */
const generalKeys = ['listData', 'recentErrors', 'largestObjects', 'kvWriteRead', 'gofunctionCreate']

export const AGENT_PROMPTS: AgentPromptSet[] = [
  { module: 'general', keys: generalKeys },
  {
    module: 'database',
    keys: ['listTables', 'rowCounts', 'emptyTables', 'previewRows']
  },
  { module: 'kv', keys: ['listKeys', 'cacheStats', 'counter'] },
  { module: 'gofunction', keys: ['listFunctions', 'buildApi', 'inspectExports'] },
  { module: 'cron', keys: ['listJobs', 'dailyAggregation', 'recentRuns'] },
  {
    module: 's3',
    keys: ['listRoot', 'largestObjects', 'byExtension', 'recentUploads']
  },
  {
    module: 'logs',
    keys: ['recentErrors', 'byLevel', 'topError', 'searchKeyword']
  },
  {
    module: 'sandbox',
    keys: ['pythonVersion', 'runScript', 'listWorkspace']
  }
]

/** 取某个 module 的提示词 key 列表；未登记的 module 回落到通用助手。 */
export function promptKeysForModule(module?: string): string[] {
  const key = (module || '').trim().toLowerCase()
  const hit = AGENT_PROMPTS.find((p) => p.module === key)
  return (hit || AGENT_PROMPTS[0]).keys
}

/** 实际命中的 module（未登记时回落 general），也是 i18n 的命名段。 */
export function promptSetForModule(module?: string): string {
  const key = (module || '').trim().toLowerCase()
  return AGENT_PROMPTS.some((p) => p.module === key) ? key : AGENT_PROMPTS[0].module
}

/** 完整 i18n key 列表，直接喂给 t()。 */
export function promptI18nKeys(module?: string): string[] {
  const set = promptSetForModule(module)
  return promptKeysForModule(module).map((k) => `pages.agents.prompts.${set}.${k}`)
}
