import type { BadgeVariants } from '@/components/ui/badge'
import { t } from '../locales'

/**
 * 状态/级别文案统一走 i18n（plan/planv5.0/ui-optimization-v3-plan.md §3.2）。
 *
 * 这里是「出口」：返回已翻译文案而不是中文字面量，消费者不用各自翻译。
 * 组件外没有 useI18n 上下文，因此用 i18n.global.t。
 */

const STATUS_KEYS: Record<string, string> = {
  ready: 'status.ready',
  creating: 'status.creating',
  degraded: 'status.degraded',
  deleting: 'status.deleting',
  deleted: 'status.deleted'
}

export function statusText(s: string) {
  const key = STATUS_KEYS[s]
  return key ? t(key) : s
}

export function statusBadgeVariant(s: string): BadgeVariants['variant'] {
  if (s === 'ready') return 'success'
  if (['degraded', 'deleting'].includes(s)) return 'warning'
  if (s === 'creating') return 'outline'
  return 'secondary'
}

export function logLevelVariant(s: string): BadgeVariants['variant'] {
  if (s === 'error') return 'destructive'
  if (s === 'warn') return 'warning'
  return 'secondary'
}

const LOG_LEVEL_KEYS: Record<string, string> = {
  info: 'logLevel.info',
  warn: 'logLevel.warn',
  error: 'logLevel.error'
}

export function logLevelText(s: string) {
  const key = LOG_LEVEL_KEYS[s]
  if (key) return t(key)
  return s || '-'
}

/** Cron job last-run status. Separate from database `statusText`. */
export function cronStatusText(status: string) {
  if (status === 'completed') return t('cron.completed')
  if (status === 'failed') return t('cron.failed')
  if (status === 'running') return t('cron.running')
  return t('cron.idle')
}

export function runStatusVariant(status: string): BadgeVariants['variant'] {
  if (status === 'completed') return 'default'
  if (status === 'failed') return 'destructive'
  if (status === 'running' || status === 'queued') return 'secondary'
  return 'outline'
}

export function cronStatusVariant(status: string): BadgeVariants['variant'] {
  if (status === 'completed') return 'success'
  if (status === 'failed') return 'destructive'
  if (status === 'running') return 'secondary'
  return 'outline'
}
