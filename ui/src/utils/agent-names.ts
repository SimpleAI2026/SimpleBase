import { i18n } from '@/locales'
import type { LocaleCode } from '@/locales'
import type { CloudAgent } from '@/services/types'

/**
 * 后端种子助手的默认名（`internal/systemdb/agents.go` 的内置助手）。
 *
 * 用户没改过名字（等于种子名）时，界面按当前语言显示 `pages.agents.builtinNames.<key>`；
 * 改过名则尊重用户的名字。这张表必须与后端种子名保持逐字一致。
 */
export const BUILTIN_SEED_NAMES: Readonly<Record<string, string>> = {
  general: '通用助手',
  database: 'Database',
  s3: 'S3',
  logs: 'Logs',
  kv: '键值助手',
  gofunction: '云函数助手',
  cron: '定时任务助手',
  sandbox: '沙盒助手'
}

/** 历史版本里 kv 助手叫过 `KV助手`，旧草稿里的 @KV助手 仍要能解析。 */
const LEGACY_ALIASES: Readonly<Record<string, readonly string[]>> = { kv: ['KV助手'] }

/** 助手名仍是后端种子名（用户未改名）。 */
export function isSeedName(agent: Pick<CloudAgent, 'builtin_key' | 'name'>): boolean {
  const key = agent.builtin_key || ''
  return agent.name === BUILTIN_SEED_NAMES[key]
}

/** 种子名助手按指定语言显示，其余原样返回名字。 */
export function displayAgentName(agent: Pick<CloudAgent, 'builtin_key' | 'name'>, locale: LocaleCode): string {
  if (!isSeedName(agent)) return agent.name
  return i18n.global.t(`pages.agents.builtinNames.${agent.builtin_key}`, {}, { locale })
}

/** 一个内置助手在 @ 解析时可能出现的全部名字（两种界面语言 + 历史别名）。 */
export function builtinAliases(key: string): string[] {
  const names: string[] = [...(LEGACY_ALIASES[key] ?? [])]
  for (const locale of ['zh-CN', 'en-US'] as const) {
    const translated = i18n.global.t(`pages.agents.builtinNames.${key}`, {}, { locale })
    if (!names.includes(translated)) names.push(translated)
  }
  return names
}
