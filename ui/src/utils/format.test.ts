import { describe, expect, it } from 'vitest'
import { formatBytes, formatJson, formatTime } from './format'

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
    expect(formatTime('not-a-date')).toBe('not-a-date')
  })

  it('formats ISO timestamps in zh-CN', () => {
    const out = formatTime('2026-01-02T03:04:05.000Z')
    expect(out).not.toBe('-')
    expect(out).not.toBe('2026-01-02T03:04:05.000Z')
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
