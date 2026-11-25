import { describe, expect, it } from 'vitest'
import { agentPath, cronJobsPath, dataCollectionsPath, databaseResourcePath, gofunctionsPath, kvPath, projectPath } from '@/services/paths'
import { listOf, objectOf, orDefault, orElse, orUndefined, pick } from '@/services/wire'
import { errorCode, isConflict } from '@/utils/api-errors'

describe('paths', () => {
  it('projectPath 每一段单独编码，段内的 / 不会被当成分隔符', () => {
    expect(projectPath('p')).toBe('/v1/projects/p')
    expect(projectPath('a/b', 'agents', '中文 x')).toBe('/v1/projects/a%2Fb/agents/%E4%B8%AD%E6%96%87%20x')
    expect(projectPath('p', 'agent-threads', 7)).toBe('/v1/projects/p/agent-threads/7')
  })

  it('各资源路径与改造前逐字一致', () => {
    expect(kvPath('a b')).toBe('/v1/projects/a%20b/kv')
    expect(agentPath('p', '/agents/x')).toBe('/v1/projects/p/agents/x')
    expect(agentPath('p')).toBe('/v1/projects/p')
    expect(cronJobsPath('p')).toBe('/v1/projects/p/cron-jobs')
    expect(cronJobsPath('p', 'j/1', 'runs')).toBe('/v1/projects/p/cron-jobs/j%2F1/runs')
    expect(cronJobsPath('p', undefined, 'runs')).toBe('/v1/projects/p/cron-jobs')
    expect(gofunctionsPath('p', 'f', 3, 'test')).toBe('/v1/projects/p/gofunctions/f/versions/3/test')
    expect(gofunctionsPath('p', 'f', undefined, 'versions')).toBe('/v1/projects/p/gofunctions/f/versions')
    expect(databaseResourcePath('p', 'd', '/schema')).toBe('/v1/projects/p/databases/d/schema')
    expect(dataCollectionsPath('p', 'd')).toBe('/v1/projects/p/databases/d/data/collections')
    expect(dataCollectionsPath('p', 'd', 'c/1')).toBe('/v1/projects/p/databases/d/data/collections/c%2F1')
  })
})

describe('wire.listOf', () => {
  it('非数组一律得到空数组', () => {
    for (const v of [undefined, null, 0, '', 'x', {}, { length: 1 }]) expect(listOf(v)).toEqual([])
  })

  it('数组原样返回（同一个引用），带 map 时逐项映射', () => {
    const arr = [1, 2]
    expect(listOf(arr)).toBe(arr)
    expect(listOf([1, 2], (n: number) => n * 2)).toEqual([2, 4])
    expect(listOf(undefined, String)).toEqual([])
    expect(listOf([1, null], String)).toEqual(['1', 'null'])
  })
})

describe('api-errors', () => {
  it('errorCode 只认字符串 code', () => {
    expect(errorCode({ code: 'x' })).toBe('x')
    expect(errorCode({ code: 1 })).toBeUndefined()
    expect(errorCode(null)).toBeUndefined()
    expect(errorCode('x')).toBeUndefined()
  })

  it('isConflict 仅 status===409', () => {
    expect(isConflict({ status: 409 })).toBe(true)
    expect(isConflict({ status: 404 })).toBe(false)
    expect(isConflict(new Error('x'))).toBe(false)
    expect(isConflict(undefined)).toBe(false)
    expect(isConflict('409')).toBe(false)
  })
})

describe('逐字段读取器：语义与改造前的 raw.x / ?? / || 一一对应', () => {
  const raw = { a: 'x', zero: 0, empty: '', no: false, nil: null, nested: { k: 1 } }

  it('pick 原样读取，缺失为 undefined；raw 为空也不抛', () => {
    expect(pick(raw, 'a')).toBe('x')
    expect(pick(raw, 'zero')).toBe(0)
    expect(pick(raw, 'missing')).toBeUndefined()
    expect(pick(undefined, 'a')).toBeUndefined()
  })

  it('orDefault 只在 null / undefined 时用默认值（0、空串、false 原样保留）', () => {
    expect(orDefault(raw, 'zero', 9)).toBe(0)
    expect(orDefault(raw, 'empty', 'd')).toBe('')
    expect(orDefault(raw, 'no', true)).toBe(false)
    expect(orDefault(raw, 'nil', 'd')).toBe('d')
    expect(orDefault(raw, 'missing', 'd')).toBe('d')
  })

  it('orUndefined / orElse 把所有假值当作未设置', () => {
    expect(orUndefined(raw, 'a')).toBe('x')
    for (const key of ['zero', 'empty', 'no', 'nil', 'missing']) expect(orUndefined(raw, key)).toBeUndefined()
    expect(orElse(raw, 'a', 'd')).toBe('x')
    for (const key of ['zero', 'empty', 'no', 'nil', 'missing']) expect(orElse(raw, key, 'd')).toBe('d')
  })

  it('objectOf 只接受对象，其余（含 null / 字符串 / 数字）得到 undefined', () => {
    expect(objectOf(raw.nested)).toEqual({ k: 1 })
    for (const v of [null, undefined, 'str', 0, false]) expect(objectOf(v)).toBeUndefined()
  })
})
