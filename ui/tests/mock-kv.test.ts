import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createKvMock } from '@/services/mock-kv'

// mock-kv.js 是项目级内存 KV（cmd 分发），直接驱动 exec 单端点
const delay = vi.fn().mockResolvedValue(undefined)
const kv = createKvMock({ delay })

beforeEach(() => {
  vi.clearAllMocks()
  kv.reset()
})

/** 执行一条 cmd，返回解析后的回复 */
function cmd(projectId: string, ...argvs: (string | number)[]) {
  return kv.exec(projectId, { type: 'cmd', argvs: argvs.map(String) })
}

describe('mock-kv (项目级 cmd 分发)', () => {
  const p = 'p1'

  it('string 基本读写与 TTL 到期', async () => {
    expect(await cmd(p, 'SET', 'k', 'v')).toBe('OK')
    expect(await cmd(p, 'GET', 'k')).toBe('v')
    expect(await cmd(p, 'EXPIRE', 'k', 1)).toBe(1)
    expect(await cmd(p, 'TTL', 'k')).toBeGreaterThan(0)
    vi.useFakeTimers()
    vi.setSystemTime(Date.now() + 2000)
    expect(await cmd(p, 'GET', 'k')).toBeNull()
    vi.useRealTimers()
  })

  it('PERSIST 清除 TTL；RENAME 移动并删旧 key', async () => {
    await cmd(p, 'SET', 'a', '1')
    await cmd(p, 'EXPIREAT', 'a', Math.floor(Date.now() / 1000) + 60)
    expect(await cmd(p, 'PERSIST', 'a')).toBe(1)
    expect(await cmd(p, 'TTL', 'a')).toBe(-1)
    expect(await cmd(p, 'RENAME', 'a', 'b')).toBe('OK')
    expect(await cmd(p, 'GET', 'a')).toBeNull()
    expect(await cmd(p, 'GET', 'b')).toBe('1')
    expect(await cmd(p, 'RENAMENX', 'b', 'b2')).toBe(1)
  })

  it('类型守卫：string 上读 hash 抛类型错误', async () => {
    await cmd(p, 'SET', 's', 'x')
    await expect(cmd(p, 'HGET', 's', 'f')).rejects.toThrow(/different type/)
  })

  it('incr 与 mget/mset 批量', async () => {
    expect(await cmd(p, 'INCRBY', 'c', 5)).toBe(5)
    expect(await cmd(p, 'DECRBY', 'c', 2)).toBe(3)
    expect(await cmd(p, 'SETNX', 'c', 'other')).toBe(0)
    await cmd(p, 'MSET', 'm1', 'a', 'm2', 'b')
    expect(await cmd(p, 'MGET', 'm1', 'm2', 'missing')).toEqual(['a', 'b', null])
    expect(await cmd(p, 'INCRBYFLOAT', 'f', '1.5')).toBe(1.5)
  })

  it('SET 条件选项：NX/XX 不满足返回 null', async () => {
    await cmd(p, 'SET', 'x', '1')
    expect(await cmd(p, 'SET', 'x', '2', 'NX')).toBeNull()
    expect(await cmd(p, 'GET', 'x')).toBe('1')
    expect(await cmd(p, 'SET', 'y', '2', 'XX')).toBeNull()
    expect(await cmd(p, 'GET', 'y')).toBeNull()
  })

  it('GETSET 返回旧值；TYPE 识别类型', async () => {
    await cmd(p, 'SET', 'g', 'old')
    expect(await cmd(p, 'GETSET', 'g', 'new')).toBe('old')
    expect(await cmd(p, 'TYPE', 'g')).toBe('string')
    expect(await cmd(p, 'TYPE', 'nope')).toBe('none')
  })

  it('hash：HSET 计数新增、HGETALL 排序扁平、HINCRBY/HDEL/HLEN', async () => {
    expect(await cmd(p, 'HSET', 'h', 'f1', 'v1', 'f2', 'v2')).toBe(2)
    expect(await cmd(p, 'HSET', 'h', 'f1', 'v1b', 'f3', 'v3')).toBe(1)
    expect(await cmd(p, 'HGET', 'h', 'f1')).toBe('v1b')
    expect(await cmd(p, 'HMGET', 'h', 'f1', 'f3', 'nope')).toEqual(['v1b', 'v3', null])
    expect(await cmd(p, 'HGETALL', 'h')).toEqual(['f1', 'v1b', 'f2', 'v2', 'f3', 'v3'])
    expect(await cmd(p, 'HINCRBY', 'h', 'n', '5')).toBe(5)
    expect(await cmd(p, 'HDEL', 'h', 'f1', 'nope')).toBe(1)
    expect(await cmd(p, 'HLEN', 'h')).toBe(3)
    expect(await cmd(p, 'HGET', 'no-such', 'f')).toBeNull()
  })

  it('HSCAN：MATCH pattern 与游标', async () => {
    await cmd(p, 'HSET', 'hs', 'a1', '1', 'a2', '2', 'b1', '3')
    const [cursor, flat] = (await cmd(p, 'HSCAN', 'hs', '0', 'MATCH', 'a*')) as [string, string[]]
    expect(cursor).toBe('0')
    expect(flat).toEqual(['a1', '1', 'a2', '2'])
  })

  it('list：LPUSH/RPUSH/LRANGE/LSET/LPOP/RPOP/LTRIM/LLEN', async () => {
    expect(await cmd(p, 'RPUSH', 'q', 'a', 'b')).toBe(2)
    expect(await cmd(p, 'LPUSH', 'q', 'head')).toBe(3)
    expect(await cmd(p, 'LRANGE', 'q', '0', '-1')).toEqual(['head', 'a', 'b'])
    expect(await cmd(p, 'LSET', 'q', '1', 'mid')).toBe('OK')
    expect(await cmd(p, 'LRANGE', 'q', '1', '2')).toEqual(['mid', 'b'])
    expect(await cmd(p, 'LLEN', 'q')).toBe(3)
    expect(await cmd(p, 'RPOP', 'q')).toBe('b')
    expect(await cmd(p, 'LPOP', 'q')).toBe('head')
    expect(await cmd(p, 'LTRIM', 'q', '0', '0')).toBe('OK')
    expect(await cmd(p, 'LRANGE', 'q', '0', '-1')).toEqual(['mid'])
    expect(await cmd(p, 'LPOP', 'no-list')).toBeNull()
  })

  it('set：SADD 计数、SISMEMBER、并交差与 STORE 变体', async () => {
    expect(await cmd(p, 'SADD', 's1', 'a', 'b', 'b')).toBe(2)
    expect(await cmd(p, 'SISMEMBER', 's1', 'a')).toBe(1)
    expect(await cmd(p, 'SISMEMBER', 's1', 'zzz')).toBe(0)
    await cmd(p, 'SADD', 's2', 'b', 'c')
    expect(await cmd(p, 'SINTER', 's1', 's2')).toEqual(['b'])
    expect(await cmd(p, 'SUNION', 's1', 's2')).toEqual(['a', 'b', 'c'])
    expect(await cmd(p, 'SDIFF', 's1', 's2')).toEqual(['a'])
    expect(await cmd(p, 'SUNIONSTORE', 'u', 's1', 's2')).toBe(3)
    expect(await cmd(p, 'SCARD', 'u')).toBe(3)
    expect(await cmd(p, 'SINTERSTORE', 'i', 's1', 's2')).toBe(1)
    expect(await cmd(p, 'SDIFFSTORE', 'd', 's1', 's2')).toBe(1)
    expect(await cmd(p, 'SMEMBERS', 'u')).toEqual(['a', 'b', 'c'])
    expect(await cmd(p, 'SREM', 'u', 'a', 'nope')).toBe(1)
    const popped = await cmd(p, 'SPOP', 'u')
    expect(['b', 'c']).toContain(popped)
  })

  it('zset：ZADD 新增计数、ZRANGE/WITHSCORES/REV、ZRANGEBYSCORE LIMIT、ZINCRBY', async () => {
    expect(await cmd(p, 'ZADD', 'z', '1.5', 'a', '3', 'b')).toBe(2)
    expect(await cmd(p, 'ZADD', 'z', '2', 'a')).toBe(0) // 改分不增
    expect(await cmd(p, 'ZSCORE', 'z', 'a')).toBe(2)
    expect(await cmd(p, 'ZRANK', 'z', 'b')).toBe(1)
    expect(await cmd(p, 'ZRANGE', 'z', '0', '-1')).toEqual(['a', 'b'])
    expect(await cmd(p, 'ZRANGE', 'z', '0', '-1', 'WITHSCORES')).toEqual(['a', 2, 'b', 3])
    expect(await cmd(p, 'ZRANGE', 'z', '0', '-1', 'REV')).toEqual(['b', 'a'])
    expect(await cmd(p, 'ZRANGEBYSCORE', 'z', '2.5', '+inf')).toEqual(['b'])
    expect(await cmd(p, 'ZRANGEBYSCORE', 'z', '2.5', '+inf', 'LIMIT', '0', '1')).toEqual(['b'])
    expect(await cmd(p, 'ZINCRBY', 'z', '0.5', 'a')).toBe(2.5)
    expect(await cmd(p, 'ZCOUNT', 'z', '2.5', '3')).toBe(2)
    expect(await cmd(p, 'ZCARD', 'z')).toBe(2)
    expect(await cmd(p, 'ZREM', 'z', 'b')).toBe(1)
    expect(await cmd(p, 'ZSCORE', 'z', 'b')).toBeNull()
  })

  it('key：DEL/EXISTS/DBSIZE/SCAN（pattern、TYPE 过滤、游标分页）', async () => {
    await cmd(p, 'MSET', 'sess:1', 'a', 'sess:2', 'b', 'user:1', 'c')
    expect(await cmd(p, 'EXISTS', 'sess:1', 'nope')).toBe(1)
    expect(await cmd(p, 'DBSIZE')).toBe(3)
    const [c1, k1] = (await cmd(p, 'SCAN', '0', 'MATCH', 'sess:*')) as [string, string[]]
    expect(c1).toBe('0')
    expect(k1).toEqual(['sess:1', 'sess:2'])
    const [, k2] = (await cmd(p, 'SCAN', '0', 'MATCH', '*', 'TYPE', 'string')) as [string, string[]]
    expect(k2.length).toBe(3)
    expect(await cmd(p, 'DEL', 'sess:1', 'nope')).toBe(1)
    expect(await cmd(p, 'DBSIZE')).toBe(2)
  })

  it('类型化写入：String/Hash/List/Set/ZSet + ttl_ms', async () => {
    expect(await kv.exec(p, { type: 'String', args: { key: 'ts', value: 'hello' } })).toBe('OK')
    expect(await cmd(p, 'GET', 'ts')).toBe('hello')

    expect(await kv.exec(p, { type: 'Hash', args: { key: 'th', fields: { f: 'v' } } })).toBe(1)
    expect(await cmd(p, 'HGET', 'th', 'f')).toBe('v')

    expect(
      await kv.exec(p, { type: 'List', args: { key: 'tl', elems: ['a', 'b'], side: 'front' } })
    ).toBe(2)
    expect(await cmd(p, 'LRANGE', 'tl', '0', '-1')).toEqual(['a', 'b'])

    expect(await kv.exec(p, { type: 'Set', args: { key: 'tset', elems: ['x', 'y'] } })).toBe(2)

    expect(
      await kv.exec(p, {
        type: 'ZSet',
        args: { key: 'tz', items: [{ elem: 'a', score: 1 }, { elem: 'b', score: 2 }] }
      })
    ).toBe(2)

    // ttl_ms=0 立即过期
    await kv.exec(p, { type: 'String', args: { key: 'gone', value: 'v', ttl_ms: 0 } })
    expect(await cmd(p, 'GET', 'gone')).toBeNull()
    // ttl_ms>0 相对过期
    await kv.exec(p, { type: 'String', args: { key: 'ttl', value: 'v', ttl_ms: 60000 } })
    expect(await cmd(p, 'PTTL', 'ttl')).toBeGreaterThan(0)
  })

  it('参数校验与未知命令', async () => {
    await expect(cmd(p, 'BOGUS')).rejects.toThrow(/unknown command/)
    await expect(cmd(p, 'SET', 'only-key')).rejects.toThrow(/wrong number/)
    await expect(cmd(p, 'SET', 'k', 'v', 'BOGUSOPT')).rejects.toThrow(/unknown option/)
    await expect(cmd(p, 'INCRBY', 's', 'not-a-num')).rejects.toThrow(/not an integer/)
    await expect(kv.exec(p, {} as never)).rejects.toThrow(/type is required/)
    await expect(
      kv.exec(p, { type: 'String', args: {} as never })
    ).rejects.toThrow(/key is required/)
  })

  it('execBatch 顺序执行多条', async () => {
    const out = await kv.execBatch(p, [
      { type: 'cmd', argvs: ['INCR', 'seq'] },
      { type: 'cmd', argvs: ['INCR', 'seq'] },
      { type: 'cmd', argvs: ['GET', 'seq'] }
    ])
    expect(out).toEqual([1, 2, '2'])
  })

  it('项目间隔离：同 key 不同项目互不可见', async () => {
    await cmd('pa', 'SET', 'shared', 'A')
    await cmd('pb', 'SET', 'shared', 'B')
    expect(await cmd('pa', 'GET', 'shared')).toBe('A')
    expect(await cmd('pb', 'GET', 'shared')).toBe('B')
  })
})
