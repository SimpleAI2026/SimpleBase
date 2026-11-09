import { t } from '../locales'

/** 相对时间格式化（会话列表等）。文案走 i18n（planv5.0 ui-optimization-v3-plan §3.2）。 */
export function relativeTime(iso?: string): string {
  if (!iso) return ''
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return ''
  const elapsed = Date.now() - then
  if (elapsed < 60_000) return t('relative.justNow')
  if (elapsed < 3_600_000) return t('relative.minutesAgo', { n: Math.floor(elapsed / 60_000) })
  if (elapsed < 86_400_000) return t('relative.hoursAgo', { n: Math.floor(elapsed / 3_600_000) })
  return t('relative.daysAgo', { n: Math.floor(elapsed / 86_400_000) })
}
