import { beforeEach, describe, expect, it } from 'vitest'
import { genDnsSafeId } from '@/utils/project-id'
import {
  clearPendingStarter,
  markStarterSent,
  savePendingStarter,
  starterAlreadySent,
  takePendingStarter
} from '@/utils/starter-storage'
import { STARTER_TEMPLATES, findStarterTemplate } from '@/constants/starter-prompts'

describe('genDnsSafeId', () => {
  it('只产出 [a-z0-9]{8}（站点发布要求 DNS 安全）', () => {
    for (let i = 0; i < 500; i++) {
      const id = genDnsSafeId()
      expect(id).toMatch(/^[a-z0-9]{8}$/)
    }
  })

  it('不含大写与连字符，首尾都是字母数字', () => {
    const id = genDnsSafeId(() => new Uint8Array([255, 255, 255, 255, 255, 255, 255, 255]))
    expect(id).toMatch(/^[a-z0-9]+$/)
    expect(id).toHaveLength(8)
  })

  it('没有 Web Crypto 时仍可生成', () => {
    const g = globalThis as { crypto?: unknown }
    const saved = g.crypto
    Object.defineProperty(globalThis, 'crypto', { value: undefined, configurable: true })
    try {
      expect(genDnsSafeId()).toMatch(/^[a-z0-9]{8}$/)
    } finally {
      Object.defineProperty(globalThis, 'crypto', { value: saved, configurable: true })
    }
  })
})

describe('starter-storage', () => {
  beforeEach(() => {
    sessionStorage.clear()
  })

  it('pending 读取即清除，只续跑一次', () => {
    savePendingStarter({ templateId: 'dashboard', name: '看板', engine: 'sqlite' })
    expect(takePendingStarter()).toEqual({ templateId: 'dashboard', name: '看板', engine: 'sqlite' })
    expect(takePendingStarter()).toBeNull()
  })

  it('格式损坏视为无', () => {
    sessionStorage.setItem('sb_pending_starter', '{坏')
    expect(takePendingStarter()).toBeNull()
    sessionStorage.setItem('sb_pending_starter', JSON.stringify({ name: 'x' }))
    expect(takePendingStarter()).toBeNull()
  })

  it('未知引擎回落 duckdb；clear 清除', () => {
    sessionStorage.setItem('sb_pending_starter', JSON.stringify({ templateId: 't', engine: 'oracle' }))
    expect(takePendingStarter()).toEqual({ templateId: 't', name: '', engine: 'duckdb' })
    savePendingStarter({ templateId: 'a', name: '', engine: 'duckdb' })
    clearPendingStarter()
    expect(takePendingStarter()).toBeNull()
  })

  it('去重键按「项目 + 模板」区分', () => {
    expect(starterAlreadySent('p1', 'dashboard')).toBe(false)
    markStarterSent('p1', 'dashboard')
    expect(starterAlreadySent('p1', 'dashboard')).toBe(true)
    expect(starterAlreadySent('p2', 'dashboard')).toBe(false)
    expect(starterAlreadySent('p1', 'notice')).toBe(false)
  })

  it('Storage 读写抛错（配额 / 隐私模式）时同样降级，不影响主流程', () => {
    const boom = () => {
      throw new Error('QuotaExceeded')
    }
    const real = Object.getOwnPropertyDescriptor(window, 'sessionStorage')!
    // jsdom 的 sessionStorage 是 getter，spy 不到实例方法：整体换成会抛错的实现。
    Object.defineProperty(window, 'sessionStorage', {
      configurable: true,
      value: { getItem: boom, setItem: boom, removeItem: boom }
    })
    try {
      expect(() => savePendingStarter({ templateId: 'a', name: '', engine: 'duckdb' })).not.toThrow()
      expect(takePendingStarter()).toBeNull()
      expect(() => clearPendingStarter()).not.toThrow()
      expect(starterAlreadySent('p', 'a')).toBe(false)
      expect(() => markStarterSent('p', 'a')).not.toThrow()
    } finally {
      Object.defineProperty(window, 'sessionStorage', real)
    }
  })

  it('sessionStorage 不可用时降级而不抛错', () => {
    const real = Object.getOwnPropertyDescriptor(window, 'sessionStorage')!
    Object.defineProperty(window, 'sessionStorage', {
      configurable: true,
      get() {
        throw new Error('denied')
      }
    })
    try {
      expect(() => savePendingStarter({ templateId: 'a', name: '', engine: 'duckdb' })).not.toThrow()
      expect(takePendingStarter()).toBeNull()
      expect(starterAlreadySent('p', 'a')).toBe(false)
      expect(() => markStarterSent('p', 'a')).not.toThrow()
      expect(() => clearPendingStarter()).not.toThrow()
    } finally {
      Object.defineProperty(window, 'sessionStorage', real)
    }
  })
})

describe('starter-prompts：模板', () => {
  it('B1 只展示数据通道为只读快照的三个模板', () => {
    expect(STARTER_TEMPLATES.map((t) => t.id)).toEqual(['dashboard', 'portfolio', 'notice'])
    // 需要匿名写入的模板依赖 B3，不能出现在前端
    for (const id of ['todo', 'booking', 'survey']) expect(findStarterTemplate(id)).toBeUndefined()
    expect(findStarterTemplate(' dashboard ')?.id).toBe('dashboard')
    expect(findStarterTemplate(undefined)).toBeUndefined()
  })
})
