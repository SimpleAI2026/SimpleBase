import type { BadgeVariants } from '@/components/ui/badge'

export const statusTextMap: Record<string, string> = {
  ready: '就绪',
  creating: '创建中',
  opening: '打开中',
  closing: '关闭中',
  recovering: '恢复中',
  closed: '已关闭',
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
  if (['creating', 'opening', 'closing', 'recovering'].includes(s)) return 'outline'
  return 'secondary'
}

export function logLevelVariant(s: string): BadgeVariants['variant'] {
  if (s === 'error') return 'destructive'
  if (s === 'warn') return 'warning'
  return 'secondary'
}
