import { describe, expect, it } from 'vitest'
import { relativeTime } from '@/utils/relative-time'

describe('relativeTime', () => {
  it('formats fresh/minutes/hours/days', () => {
    expect(relativeTime(new Date().toISOString())).toBe('刚刚')
    expect(relativeTime(new Date(Date.now() - 5 * 60_000).toISOString())).toBe('5分钟前')
    expect(relativeTime(new Date(Date.now() - 3 * 3_600_000).toISOString())).toBe('3小时前')
    expect(relativeTime(new Date(Date.now() - 2 * 86_400_000).toISOString())).toBe('2天前')
  })

  it('returns empty for invalid date', () => {
    expect(relativeTime('not-a-date')).toBe('')
  })
})
