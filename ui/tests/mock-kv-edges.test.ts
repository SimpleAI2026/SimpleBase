import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createKvMock } from '@/services/mock-kv'

// 补齐 mock-kv.js 的参数校验 / 边界分支：与 mock-kv.test.ts 的语义用例互补。
const kv = createKvMock({ delay: vi.fn().mockResolvedValue(undefined) })
const p = 'edge'
const cmd = (...argvs: (string | number)[]) => kv.exec(p, { type: 'cmd', argvs: argvs.map(String) })

beforeEach(() => kv.reset())
afterEach(() => vi.useRealTimers())

describe('mock-kv arity and argument validation', () => {
  // 每条命令给出一组错误参数个数，均应抛 wrong number of arguments。
  const wrongArity: [string, string[]][] = [
    ['DEL', []], ['EXISTS', []], ['EXPIRE', ['k']], ['PEXPIRE', ['k']], ['EXPIREAT', ['k']],
    ['PEXPIREAT', ['k']], ['PERSIST', []], ['TTL', []], ['PTTL', []], ['TYPE', []],
    ['RENAME', ['a']], ['RENAMENX', ['a']], ['DBSIZE', ['x']], ['GET', []], ['SET', ['k']],
    ['SETNX', ['k']], ['GETSET', ['k']], ['MGET', []], ['MSET', ['k']], ['MSET', []], ['INCR', []],
    ['INCRBY', ['k']], ['DECR', []], ['DECRBY', ['k']], ['INCRBYFLOAT', ['k']],
    ['HGET', ['h']], ['HMGET', ['h']], ['HSET', ['h', 'f']], ['HSET', ['h', 'f', 'v', 'g']], ['HDEL', ['h']],
    ['HGETALL', []], ['HINCRBY', ['h', 'f']], ['HLEN', []],
    ['LPUSH', ['l']], ['RPUSH', ['l']], ['LPOP', []], ['RPOP', []], ['LRANGE', ['l', '0']],
    ['LSET', ['l', '0']], ['LTRIM', ['l', '0']], ['LLEN', []],
    ['SADD', ['s']], ['SREM', ['s']], ['SISMEMBER', ['s']], ['SMEMBERS', []], ['SPOP', []], ['SCARD', []],
    ['SUNION', []], ['SINTER', []], ['SDIFF', []], ['SUNIONSTORE', ['d']], ['SINTERSTORE', ['d']], ['SDIFFSTORE', ['d']],
    ['ZADD', ['z', '1']], ['ZADD', ['z', '1', 'a', '2']], ['ZREM', ['z']], ['ZSCORE', ['z']], ['ZRANK', ['z']],
    ['ZRANGE', ['z', '0']], ['ZRANGEBYSCORE', ['z', '0']], ['ZINCRBY', ['z', '1']], ['ZCOUNT', ['z', '0']], ['ZCARD', []]
  ]
  it.each(wrongArity)('%s %j rejects wrong arity', async (name, argv) => {
    await expect(cmd(name, ...argv)).rejects.toThrow(/wrong number of arguments/)
  })

  it('validates exec envelopes and typed writes', async () => {
    await expect(kv.exec(p, null as never)).rejects.toThrow(/malformed body/)
    await expect(kv.exec(p, { type: 'cmd', argvs: [] })).rejects.toThrow(/non-empty array/)
    await expect(kv.exec(p, { type: 'cmd' } as never)).rejects.toThrow(/non-empty array/)
    await expect(kv.exec(p, { type: 'String' } as never)).rejects.toThrow(/args required/)
    await expect(kv.exec(p, { type: 'String', args: { key: 'k', value: 'v', ttl_ms: -1 } })).rejects.toThrow(/ttl_ms/)
    await expect(kv.exec(p, { type: 'String', args: { key: 'k', value: 'v', ttl_ms: 'x' as never } })).rejects.toThrow(/ttl_ms/)
    await expect(kv.exec(p, { type: 'String', args: { key: 'k' } })).rejects.toThrow(/value is required/)
    await expect(kv.exec(p, { type: 'String', args: { key: 'k', value: null } as never })).rejects.toThrow(/value is required/)
    await expect(kv.exec(p, { type: 'Hash', args: { key: 'h' } })).rejects.toThrow(/fields must be non-empty/)
    const many = Object.fromEntries(Array.from({ length: 1001 }, (_, i) => ['f' + i, 'v']))
    await expect(kv.exec(p, { type: 'Hash', args: { key: 'h', fields: many } })).rejects.toThrow(/1..1000/)
    const big = Array.from({ length: 1001 }, (_, i) => String(i))
    await expect(kv.exec(p, { type: 'List', args: { key: 'l' } })).rejects.toThrow(/elems must be non-empty/)
    await expect(kv.exec(p, { type: 'List', args: { key: 'l', elems: big } })).rejects.toThrow(/1..1000/)
    await expect(kv.exec(p, { type: 'List', args: { key: 'l', elems: ['a'], side: 'mid' } })).rejects.toThrow(/side must be/)
    await expect(kv.exec(p, { type: 'Set', args: { key: 's' } })).rejects.toThrow(/elems must be non-empty/)
    await expect(kv.exec(p, { type: 'Set', args: { key: 's', elems: big } })).rejects.toThrow(/1..1000/)
    await expect(kv.exec(p, { type: 'ZSet', args: { key: 'z' } })).rejects.toThrow(/items must be non-empty/)
    await expect(kv.exec(p, { type: 'ZSet', args: { key: 'z', items: big.map((e) => ({ elem: e, score: 1 })) } })).rejects.toThrow(/1..1000/)

    expect(await kv.exec(p, { type: 'String', args: { key: 'k', value: 1, nx: true } })).toBe('OK')
    expect(await kv.exec(p, { type: 'String', args: { key: 'k', value: 2, nx: true } })).toBeNull()
    expect(await kv.exec(p, { type: 'String', args: { key: 'k', value: 3, xx: true, keep_ttl: true } })).toBe('OK')
    expect(await kv.exec(p, { type: 'List', args: { key: 'l', elems: ['a'] } })).toBe(1)
    expect(await kv.exec(p, { type: 'String', args: { key: 'n', value: 'v', ttl_ms: null } as never })).toBe('OK')
    expect(await cmd('PTTL', 'n')).toBe(-1)
  })

  it('rejects malformed numbers, options and unknown scan options', async () => {
    await expect(cmd('EXPIRE', 'k', 'x')).resolves.toBe(0)
    await cmd('SET', 'k', 'v')
    await expect(cmd('EXPIRE', 'k', 'x')).rejects.toThrow(/not an integer/)
    await expect(cmd('INCRBYFLOAT', 'k', 'nan')).rejects.toThrow(/not a float/)
    await expect(cmd('INCR', 'k')).rejects.toThrow(/not an integer/)
    await expect(cmd('SET', 'k', 'v', 'GET')).rejects.toThrow(/GET option/)
    await expect(cmd('SCAN', '0', 'COUNT', '0')).rejects.toThrow(/COUNT/)
    await expect(cmd('SCAN', '0', 'COUNT', '501')).rejects.toThrow(/COUNT/)
    await expect(cmd('SCAN', '0', 'BOGUS')).rejects.toThrow(/unknown option/)
    await cmd('HSET', 'h', 'f', 'v')
    await expect(cmd('HSCAN', '')).rejects.toThrow(/key is required/)
    await expect(cmd('HSCAN', 'h', '0', 'COUNT', '0')).rejects.toThrow(/COUNT/)
    await expect(cmd('HSCAN', 'h', '0', 'BOGUS')).rejects.toThrow(/unknown option/)
    await cmd('HSET', 'h', 'bad', 'x')
    await expect(cmd('HINCRBY', 'h', 'bad', '1')).rejects.toThrow(/not an integer/)
    await cmd('ZADD', 'z', '1', 'a')
    await expect(cmd('ZRANGE', 'z', '0', '-1', 'BOGUS')).rejects.toThrow(/unknown option/)
    await expect(cmd('ZRANGEBYSCORE', 'z', '0', '1', 'BOGUS')).rejects.toThrow(/unknown option/)
    await expect(cmd('ZADD', 'z', 'abc', 'a')).rejects.toThrow(/not a float/)
    expect(await cmd('ZRANGEBYSCORE', 'z', '-inf', 'inf')).toEqual(['a'])
    expect(await cmd('ZRANGEBYSCORE', 'z', '-infinity', '+INF', 'WITHSCORES')).toEqual(['a', 1])
  })
})

describe('mock-kv expiry and missing-key edges', () => {
  it('covers every expire variant, TTL states and lazy expiry', async () => {
    expect(await cmd('PEXPIRE', 'none', '10')).toBe(0)
    expect(await cmd('EXPIREAT', 'none', '10')).toBe(0)
    expect(await cmd('PEXPIREAT', 'none', '10')).toBe(0)
    expect(await cmd('PERSIST', 'none')).toBe(0)
    expect(await cmd('TTL', 'none')).toBe(-2)
    expect(await cmd('PTTL', 'none')).toBe(-2)
    await cmd('SET', 'a', '1')
    expect(await cmd('PERSIST', 'a')).toBe(0)
    expect(await cmd('PTTL', 'a')).toBe(-1)
    expect(await cmd('EXPIRE', 'a', '0')).toBe(1)
    expect(await cmd('EXISTS', 'a')).toBe(0)
    await cmd('SET', 'a', '1')
    expect(await cmd('PEXPIRE', 'a', '-1')).toBe(1)
    expect(await cmd('GET', 'a')).toBeNull()
    await cmd('SET', 'a', '1')
    expect(await cmd('PEXPIRE', 'a', '60000')).toBe(1)
    expect(await cmd('PTTL', 'a')).toBeGreaterThan(0)
    expect(await cmd('EXPIREAT', 'a', '1')).toBe(1)
    expect(await cmd('GET', 'a')).toBeNull()
    await cmd('SET', 'b', '1')
    expect(await cmd('PEXPIREAT', 'b', String(Date.now() + 60000))).toBe(1)
    expect(await cmd('TTL', 'b')).toBeGreaterThan(0)
    expect(await cmd('PEXPIREAT', 'b', '1')).toBe(1)
    expect(await cmd('GET', 'b')).toBeNull()

    // SET 的 EX/PX/EXAT/PXAT/KEEPTTL 与到期即删分支
    expect(await cmd('SET', 'c', '1', 'EX', '60')).toBe('OK')
    expect(await cmd('SET', 'c', '2', 'KEEPTTL')).toBe('OK')
    expect(await cmd('TTL', 'c')).toBeGreaterThan(0)
    expect(await cmd('SET', 'c', '3', 'PX', '60000')).toBe('OK')
    expect(await cmd('SET', 'c', '4', 'EXAT', String(Math.floor(Date.now() / 1000) + 60))).toBe('OK')
    expect(await cmd('SET', 'c', '5', 'PXAT', String(Date.now() + 60000))).toBe('OK')
    expect(await cmd('SET', 'c', '6', 'XX')).toBe('OK')
    expect(await cmd('SET', 'c', '7', 'PX', '0')).toBe('OK')
    expect(await cmd('GET', 'c')).toBeNull()
    expect(await cmd('SET', 'd', '1', 'KEEPTTL')).toBe('OK')
    expect(await cmd('TTL', 'd')).toBe(-1)

    // 惰性过期：DBSIZE / SCAN 跳过已过期键
    vi.useFakeTimers()
    await cmd('SET', 'e', '1', 'PX', '10')
    vi.setSystemTime(Date.now() + 1000)
    expect(await cmd('DBSIZE')).toBe(1)
    const [, keys] = (await cmd('SCAN', '')) as [string, string[]]
    expect(keys).toEqual(['d'])
  })

  it('rename and missing keys raise not found; empty structures are deleted', async () => {
    await expect(cmd('RENAME', 'x', 'y')).rejects.toThrow(/not found/)
    await expect(cmd('RENAMENX', 'x', 'y')).rejects.toThrow(/not found/)
    await cmd('SET', 'x', '1')
    await cmd('SET', 'y', '2')
    expect(await cmd('RENAMENX', 'x', 'y')).toBe(0)
    await expect(cmd('LSET', 'nolist', '0', 'v')).rejects.toThrow(/not found/)
    await cmd('RPUSH', 'l', 'a', 'b')
    await expect(cmd('LSET', 'l', '5', 'v')).rejects.toThrow(/not found/)
    expect(await cmd('LSET', 'l', '-1', 'z')).toBe('OK')
    expect(await cmd('LRANGE', 'l', '0', '-1')).toEqual(['a', 'z'])
    expect(await cmd('LRANGE', 'l', '5', '9')).toEqual([])
    expect(await cmd('LRANGE', 'l', '-100', '100')).toEqual(['a', 'z'])
    expect(await cmd('LRANGE', 'nolist', '0', '1')).toEqual([])
    expect(await cmd('LTRIM', 'nolist', '0', '1')).toBe('OK')
    expect(await cmd('LLEN', 'nolist')).toBe(0)
    expect(await cmd('RPOP', 'l')).toBe('z')
    expect(await cmd('RPOP', 'l')).toBe('a')
    expect(await cmd('EXISTS', 'l')).toBe(0)
    await cmd('RPUSH', 'l', 'a')
    expect(await cmd('LTRIM', 'l', '3', '4')).toBe('OK')
    expect(await cmd('EXISTS', 'l')).toBe(0)

    expect(await cmd('HDEL', 'nohash', 'f')).toBe(0)
    expect(await cmd('HLEN', 'nohash')).toBe(0)
    expect(await cmd('HGETALL', 'nohash')).toEqual([])
    expect(await cmd('HMGET', 'nohash', 'f')).toEqual([null])
    await cmd('HSET', 'h', 'f', '1')
    expect(await cmd('HDEL', 'h', 'nope')).toBe(0)
    expect(await cmd('HDEL', 'h', 'f')).toBe(1)
    expect(await cmd('EXISTS', 'h')).toBe(0)
    expect(await cmd('HINCRBY', 'h2', 'n', '2')).toBe(2)
    expect(await cmd('HINCRBY', 'h2', 'n', '3')).toBe(5)

    expect(await cmd('SREM', 'noset', 'a')).toBe(0)
    expect(await cmd('SPOP', 'noset')).toBeNull()
    expect(await cmd('SCARD', 'noset')).toBe(0)
    expect(await cmd('SMEMBERS', 'noset')).toEqual([])
    expect(await cmd('SISMEMBER', 'noset', 'a')).toBe(0)
    await cmd('SADD', 's', 'a')
    expect(await cmd('SREM', 's', 'nope')).toBe(0)
    expect(await cmd('SPOP', 's')).toBe('a')
    expect(await cmd('EXISTS', 's')).toBe(0)
    expect(await cmd('SDIFF', 'noset', 'other')).toEqual([])
    expect(await cmd('SINTER', 'noset')).toEqual([])
    expect(await cmd('SUNIONSTORE', 'dest', 'noset')).toBe(0)
    expect(await cmd('EXISTS', 'dest')).toBe(0)

    expect(await cmd('ZREM', 'noz', 'a')).toBe(0)
    expect(await cmd('ZSCORE', 'noz', 'a')).toBeNull()
    expect(await cmd('ZRANK', 'noz', 'a')).toBeNull()
    expect(await cmd('ZRANGE', 'noz', '0', '-1')).toEqual([])
    expect(await cmd('ZRANGEBYSCORE', 'noz', '0', '1')).toEqual([])
    expect(await cmd('ZCOUNT', 'noz', '0', '1')).toBe(0)
    expect(await cmd('ZCARD', 'noz')).toBe(0)
    expect(await cmd('ZINCRBY', 'z', '2', 'new')).toBe(2)
    await cmd('ZADD', 'z', '2', 'b', '1', 'c')
    expect(await cmd('ZRANK', 'z', 'missing')).toBeNull()
    expect(await cmd('ZRANGE', 'z', '5', '9')).toEqual([])
    expect(await cmd('ZRANGE', 'z', '0', '-1', 'REV', 'WITHSCORES')).toEqual(['new', 2, 'b', 2, 'c', 1])
    expect(await cmd('ZRANGEBYSCORE', 'z', '0', '5', 'LIMIT', '1', '0')).toEqual(['b', 'new'])
    expect(await cmd('ZREM', 'z', 'nope')).toBe(0)
    expect(await cmd('ZREM', 'z', 'b', 'c', 'new')).toBe(3)
    expect(await cmd('EXISTS', 'z')).toBe(0)

    expect(await cmd('DECR', 'cnt')).toBe(-1)
    expect(await cmd('INCR', 'cnt')).toBe(0)
    expect(await cmd('INCRBYFLOAT', 'cnt', '0.5')).toBe(0.5)
    expect(await cmd('GETSET', 'fresh', 'v')).toBeNull()
    expect(await cmd('SETNX', 'fresh2', 'v')).toBe(1)
    await cmd('HSET', 'hh', 'f', 'v')
    expect(await cmd('MGET', 'hh')).toEqual([null])
    await expect(cmd('MSET', 'hh', 'x')).rejects.toThrow(/different type/)
  })
})

describe('mock-kv scan pagination and glob', () => {
  it('pages SCAN/HSCAN with cursors and translates glob specials', async () => {
    for (const k of ['a.1', 'a.2', 'a.3', 'b[1]', 'c?', 'x+y']) await cmd('SET', k, '1')
    const [c1, p1] = (await cmd('SCAN', '0', 'COUNT', '2')) as [string, string[]]
    expect(p1).toEqual(['a.1', 'a.2'])
    expect(c1).toBe('a.2')
    const [c2, p2] = (await cmd('SCAN', c1, 'COUNT', '2')) as [string, string[]]
    expect(p2).toEqual(['a.3', 'b[1]'])
    const [, p3] = (await cmd('SCAN', 'zzz')) as [string, string[]]
    expect(p3).toEqual([])
    expect(c2).toBe('b[1]')
    expect(((await cmd('SCAN', '0', 'MATCH', 'a.?')) as [string, string[]])[1]).toEqual(['a.1', 'a.2', 'a.3'])
    expect(((await cmd('SCAN', '0', 'MATCH', 'a.[12]')) as [string, string[]])[1]).toEqual(['a.1', 'a.2'])
    expect(((await cmd('SCAN', '0', 'MATCH', 'b[')) as [string, string[]])[1]).toEqual([])
    expect(((await cmd('SCAN', '0', 'MATCH', 'x+y')) as [string, string[]])[1]).toEqual(['x+y'])
    expect(((await cmd('SCAN', '0', 'TYPE', 'hash')) as [string, string[]])[1]).toEqual([])
    expect(((await cmd('SCAN')) as [string, string[]])[1]).toHaveLength(6)

    await cmd('HSET', 'h', 'f1', '1', 'f2', '2', 'f3', '3')
    const [hc, hp] = (await cmd('HSCAN', 'h', '0', 'COUNT', '2')) as [string, string[]]
    expect(hp).toEqual(['f1', '1', 'f2', '2'])
    const [hc2, hp2] = (await cmd('HSCAN', 'h', hc)) as [string, string[]]
    expect(hp2).toEqual(['f3', '3'])
    expect(hc2).toBe('0')
    expect(((await cmd('HSCAN', 'h', 'zz')) as [string, string[]])[1]).toEqual([])
    expect(((await cmd('HSCAN', 'nohash')) as [string, string[]])[1]).toEqual([])
  })
})
