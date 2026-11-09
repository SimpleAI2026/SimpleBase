import { describe, expect, it } from 'vitest'
import { errorMessage, formatBytes, formatCount, formatErrorRate, formatJson, formatTime, shortTime } from '@/utils/format'

describe('formatErrorRate', () => {
  it('converts ratio to percent with 4 decimals', () => {
    expect(formatErrorRate(0)).toBe('0.0000')
    expect(formatErrorRate(0.2)).toBe('20.0000')
    expect(formatErrorRate(2 / 7)).toBe('28.5714')
    expect(formatErrorRate(1)).toBe('100.0000')
  })

  it('falls back to zero for non-finite values', () => {
    expect(formatErrorRate(Number.NaN)).toBe('0.0000')
    expect(formatErrorRate(Number.POSITIVE_INFINITY)).toBe('0.0000')
    expect(formatErrorRate(undefined)).toBe('0.0000')
    expect(formatErrorRate(null)).toBe('0.0000')
  })
})

describe('formatCount', () => {
  it('formats counts and unknown', () => {
    expect(formatCount(undefined)).toBe('—')
    expect(formatCount(0)).toBe('0 条')
    expect(formatCount(42)).toBe('42 条')
  })
})

describe('formatBytes', () => {
  it('formats zero and missing values', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(undefined as unknown as number)).toBe('-')
    expect(formatBytes(Number.NaN)).toBe('-')
  })

  it('picks the matching unit', () => {
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(1024)).toBe('1.0 KB')
    expect(formatBytes(1024 * 1024)).toBe('1.0 MB')
    expect(formatBytes(1024 ** 3)).toBe('1.0 GB')
    expect(formatBytes(1024 ** 4)).toBe('1.0 TB')
    expect(formatBytes(1024 ** 6)).toMatch(/TB$/)
  })
})

describe('formatTime', () => {
  it('handles empty and invalid dates', () => {
    expect(formatTime()).toBe('-')
    expect(formatTime('')).toBe('-')
    expect(formatTime('', '—')).toBe('—')
    expect(formatTime('not-a-date')).toBe('not-a-date')
  })

  it('formats ISO timestamps in zh-CN', () => {
    const out = formatTime('2026-01-02T03:04:05.000Z')
    expect(out).not.toBe('-')
    expect(out).not.toBe('2026-01-02T03:04:05.000Z')
  })
})

describe('shortTime', () => {
  it('handles missing and invalid timestamps', () => {
    expect(shortTime('')).toBe('—')
    expect(shortTime('bad')).toBe('—')
    expect(shortTime('2024-01-15T08:05:00Z')).toMatch(/\d{2}-\d{2} \d{2}:\d{2}/)
  })
})

describe('errorMessage', () => {
  it('uses Error messages and falls back for other values', () => {
    expect(errorMessage(new Error('failed'), '默认')).toBe('failed')
    expect(errorMessage('failed', '默认')).toBe('默认')
    expect(errorMessage(null, '默认')).toBe('默认')
  })
})

describe('formatJson', () => {
  it('pretty-prints objects and falls back for circular values', () => {
    expect(formatJson({ a: 1 })).toBe('{\n  "a": 1\n}')
    const cyclic: { self?: unknown } = {}
    cyclic.self = cyclic
    expect(formatJson(cyclic)).toContain('[object Object]')
  })
})
