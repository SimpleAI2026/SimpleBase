import type { BadgeVariants } from '@/components/ui/badge'

export const statusTextMap: Record<string, string> = {
  ready: '就绪',
  creating: '创建中',
  degraded: '降级',
  deleting: '删除中',
  deleted: '已删除',
}

export function statusText(s: string) {
  return statusTextMap[s] || s
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

export const logLevelTextMap: Record<string, string> = {
  info: '信息',
  warn: '警告',
  error: '错误',
}

export function logLevelText(s: string) {
  return logLevelTextMap[s] || s || '-'
}

/** Cron job last-run status. Separate from database `statusText`. */
export function cronStatusText(status: string) {
  if (status === 'completed') return '成功'
  if (status === 'failed') return '失败'
  if (status === 'running') return '执行中'
  return '未运行'
}

export function cronStatusVariant(status: string): BadgeVariants['variant'] {
  if (status === 'completed') return 'success'
  if (status === 'failed') return 'destructive'
  if (status === 'running') return 'secondary'
  return 'outline'
}
