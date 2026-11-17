import type { AgentToolCallCard } from '@/services/types'
import type { StarterStage } from '@/constants/starter-prompts'

/**
 * 从工具卡片推导搭建进度（planv6.6 home-starter-prompts §2.3 / §2.7）。
 *
 * 不依赖模型自报「我做完了」：只看真实的、成功的工具调用。工具卡的 arguments 是
 * `{"argv":[...],"stdin":"..."}` 的 JSON 字符串。
 */
export interface ParsedToolArgv {
  argv: string[]
  ok: boolean
}

/** 取出 simplebase 工具调用的 argv 与是否成功；非 simplebase / 解析失败返回 null。 */
export function parseToolCard(card: AgentToolCallCard): ParsedToolArgv | null {
  if (card.name !== 'simplebase') return null
  let argv: unknown
  try {
    argv = (JSON.parse(card.arguments || '{}') as { argv?: unknown }).argv
  } catch {
    return null
  }
  if (!Array.isArray(argv) || !argv.every((x) => typeof x === 'string')) return null
  return { argv: argv as string[], ok: card.status === 'success' && !toolResultFailed(card.content) }
}

/** CLI 结果信封 `{"ok":false,...}` 视为失败（工具本身 status 仍可能是 success）。 */
export function toolResultFailed(content: string | undefined): boolean {
  if (!content) return false
  try {
    const v = JSON.parse(content) as { ok?: unknown; is_error?: unknown }
    return v?.ok === false || v?.is_error === true
  } catch {
    return false
  }
}

/**
 * 某阶段是否已完成：出现该阶段对应的成功工具调用。
 * - plan：没有工具要求，由 run 正常结束 + 消息非空判定（见 useStarterRun）；
 * - verify：由交付块判定（见 useStarterRun），这里只在出现任一成功的只读校验时点亮。
 */
export function stageDoneByCards(stage: StarterStage, cards: AgentToolCallCard[]): boolean {
  const parsed = cards.map(parseToolCard).filter((p): p is ParsedToolArgv => p !== null && p.ok)
  const has = (pred: (argv: string[]) => boolean) => parsed.some((p) => pred(p.argv))
  switch (stage) {
    case 'data':
      return has((a) => (a[0] === 'sql' && (a[1] === 'exec' || a[1] === 'batch')) || (a[0] === 'collection' && a[1] === 'create') || (a[0] === 'schema' && a[1] === 'create-table'))
    case 'func':
      return has((a) => a[0] === 'function' && a[1] === 'test')
    case 'cron':
      return has((a) => a[0] === 'cron' && a[1] === 'create')
    case 'site':
      return has((a) => a[0] === 'site' && a[1] === 'publish')
    case 'verify':
      return has((a) => (a[0] === 'sql' && a[1] === 'query') || (a[0] === 'function' && a[1] === 'invoke') || (a[0] === 'site' && a[1] === 'status'))
    default:
      return false
  }
}

/** 阶段命令对应的进度条分段（用于工具调用实时点亮「正在进行」）。 */
export function stageForArgv(argv: string[]): StarterStage | null {
  switch (argv[0]) {
    case 'sql':
    case 'schema':
    case 'collection':
    case 'document':
    case 'database':
      return 'data'
    case 'function':
      return 'func'
    case 'cron':
      return 'cron'
    case 'object':
    case 'site':
      return 'site'
    default:
      return null
  }
}
