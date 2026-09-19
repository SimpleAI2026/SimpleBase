import { describe, expect, it } from 'vitest'
import { logLevelVariant, statusBadgeVariant, statusText, statusTextMap } from './status'

describe('status helpers', () => {
  it('maps known statuses and falls back to raw text', () => {
    expect(statusText('ready')).toBe('就绪')
    expect(statusText('unknown-status')).toBe('unknown-status')
    expect(Object.keys(statusTextMap).length).toBeGreaterThan(5)
  })

  it('picks badge variants for status and log level', () => {
    expect(statusBadgeVariant('ready')).toBe('success')
    expect(statusBadgeVariant('degraded')).toBe('warning')
    expect(statusBadgeVariant('deleting')).toBe('warning')
    expect(statusBadgeVariant('creating')).toBe('outline')
    expect(statusBadgeVariant('opening')).toBe('outline')
    expect(statusBadgeVariant('closing')).toBe('outline')
    expect(statusBadgeVariant('recovering')).toBe('outline')
    expect(statusBadgeVariant('deleted')).toBe('secondary')
    expect(logLevelVariant('error')).toBe('destructive')
    expect(logLevelVariant('warn')).toBe('warning')
    expect(logLevelVariant('info')).toBe('secondary')
  })
})
