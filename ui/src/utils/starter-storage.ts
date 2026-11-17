/**
 * 搭建流程的 sessionStorage 状态（planv6.6 home-starter-prompts §2.2 / §2.3）。
 *
 * - pendingStarter：未登录时点了卡片，登录后续跑同一张卡片，不丢意图；
 * - 去重键 `starter:<projectId>:<templateId>`：自动发送过一次后不再重复（刷新 / 返回）。
 *
 * sessionStorage 在隐私模式或被禁用时可能抛错，全部降级为「不记」，不能影响主流程。
 */
const PENDING_KEY = 'sb_pending_starter'

export interface PendingStarter {
  templateId: string
  name: string
  engine: 'duckdb' | 'sqlite'
}

function store(): Storage | null {
  try {
    return typeof sessionStorage === 'undefined' ? null : sessionStorage
  } catch {
    return null
  }
}

export function savePendingStarter(p: PendingStarter): void {
  try {
    store()?.setItem(PENDING_KEY, JSON.stringify(p))
  } catch {
    /* 忽略 */
  }
}

/** 读取并清除；格式不对视为无。 */
export function takePendingStarter(): PendingStarter | null {
  const s = store()
  if (!s) return null
  let raw: string | null = null
  try {
    raw = s.getItem(PENDING_KEY)
    s.removeItem(PENDING_KEY)
  } catch {
    return null
  }
  if (!raw) return null
  try {
    const v = JSON.parse(raw) as Partial<PendingStarter>
    if (typeof v.templateId !== 'string' || !v.templateId) return null
    return {
      templateId: v.templateId,
      name: typeof v.name === 'string' ? v.name : '',
      engine: v.engine === 'sqlite' ? 'sqlite' : 'duckdb'
    }
  } catch {
    return null
  }
}

export function clearPendingStarter(): void {
  try {
    store()?.removeItem(PENDING_KEY)
  } catch {
    /* 忽略 */
  }
}

function sentKey(projectId: string, templateId: string): string {
  return `starter:${projectId}:${templateId}`
}

/** 该项目的该模板是否已经自动发送过种子提示词。 */
export function starterAlreadySent(projectId: string, templateId: string): boolean {
  try {
    return store()?.getItem(sentKey(projectId, templateId)) === '1'
  } catch {
    return false
  }
}

export function markStarterSent(projectId: string, templateId: string): void {
  try {
    store()?.setItem(sentKey(projectId, templateId), '1')
  } catch {
    /* 忽略 */
  }
}
