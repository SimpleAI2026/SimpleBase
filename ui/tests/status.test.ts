import { describe, expect, it } from 'vitest'
import {
  cronStatusText,
  cronStatusVariant,
  logLevelText,
  logLevelVariant,
  statusBadgeVariant,
  statusText,
  statusTextMap,
} from './status'

describe('status helpers', () => {
  it('maps known statuses and falls back to raw text', () => {
    expect(statusText('ready')).toBe('就绪')
    expect(statusText('creating')).toBe('创建中')
    expect(statusText('degraded')).toBe('降级')
    expect(statusText('deleting')).toBe('删除中')
    expect(statusText('deleted')).toBe('已删除')
    expect(statusText('closed')).toBe('closed')
    expect(statusText('unknown-status')).toBe('unknown-status')
    expect(Object.keys(statusTextMap)).toEqual(['ready', 'creating', 'degraded', 'deleting', 'deleted'])
  })

  it('picks badge variants for status and log level', () => {
    expect(statusBadgeVariant('ready')).toBe('success')
    expect(statusBadgeVariant('degraded')).toBe('warning')
    expect(statusBadgeVariant('deleting')).toBe('warning')
    expect(statusBadgeVariant('creating')).toBe('outline')
    expect(statusBadgeVariant('deleted')).toBe('secondary')
    expect(logLevelVariant('error')).toBe('destructive')
    expect(logLevelVariant('warn')).toBe('warning')
    expect(logLevelVariant('info')).toBe('secondary')
    expect(logLevelText('info')).toBe('信息')
    expect(logLevelText('warn')).toBe('警告')
    expect(logLevelText('error')).toBe('错误')
    expect(logLevelText('debug')).toBe('debug')
    expect(logLevelText('')).toBe('-')
    expect(cronStatusText('completed')).toBe('成功')
    expect(cronStatusText('failed')).toBe('失败')
    expect(cronStatusText('running')).toBe('执行中')
    expect(cronStatusText('')).toBe('未运行')
    expect(cronStatusVariant('completed')).toBe('success')
    expect(cronStatusVariant('failed')).toBe('destructive')
    expect(cronStatusVariant('running')).toBe('secondary')
    expect(cronStatusVariant('')).toBe('outline')
  })
})
