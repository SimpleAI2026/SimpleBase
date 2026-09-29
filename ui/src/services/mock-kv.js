/**
 * KV mock：内存 Map 实现 Redis 语义（TTL 惰性过期 + 类型守卫）。
 * 由 mock.js 装配：createKvMock({ delay })。
 *
 * 项目级单端点（key-value-ducklake-plan §3）：
 *   exec(projectId, body) —— body 为 {type:"cmd",argvs:[...]} 或
 *   {type:"String|Hash|List|Set|ZSet",args:{...}}。
 * 按项目存一份内存 KV（不再按 databaseId 分桶），返回 Redis 回复的 JSON 编码。
 */

// 美元符号在部分写入链路中会被转义破坏，统一用 fromCharCode 构造
const DOLLAR = String.fromCharCode(36)

// 需要转义的正则特殊字符（* ? [ 由 glob 分支单独处理）
const REGEX_SPECIALS = '.*+?^' + DOLLAR + '{}()|[]\\'

// glob 转正则：支持 * ? [abc]
function globToRegExp(pattern) {
  let re = ''
  for (let i = 0; i < pattern.length; i++) {
    const ch = pattern[i]
    if (ch === '*') {
      re += '.*'
    } else if (ch === '?') {
      re += '.'
    } else if (ch === '[') {
      const end = pattern.indexOf(']', i)
      if (end > i) {
        re += pattern.slice(i, end + 1)
        i = end
      } else {
        re += '\\['
      }
    } else if (REGEX_SPECIALS.includes(ch)) {
      re += '\\' + ch
    } else {
      re += ch
    }
  }
  return new RegExp('^' + re + DOLLAR)
}

// 惰性过期：到期即删并视为不存在
function kvGetAlive(kv, key) {
  const e = kv.get(key)
  if (!e) return null
  if (e.etime != null && e.etime <= Date.now()) {
    kv.delete(key)
    return null
  }
  return e
}

// 类型守卫：类型不符抛错（模拟后端 kv_type_mismatch）
function kvGuardType(kv, key, want) {
  const e = kvGetAlive(kv, key)
  if (e && e.type !== want) {
    throw new Error('key holds a different type (kv_type_mismatch)')
  }
  return e
}

function kvTouch(kv, key, type, len) {
  let e = kv.get(key)
  if (e) {
    e.version++
    e.mtime = Date.now()
    if (len !== undefined) e.len = len
  } else {
    e = { type, version: 1, etime: null, mtime: Date.now(), len: len ?? null }
    kv.set(key, e)
  }
  return e
}

// list 负下标归一化（对齐服务端 normalizeRange）
function normalizeRange(start, stop, length) {
  if (length <= 0) return null
  let lo = start
  let hi = stop
  if (lo < 0) lo = Math.max(0, length + lo)
  if (hi < 0) hi = length + hi
  if (hi >= length) hi = length - 1
  if (lo > hi || lo >= length) return null
  return [lo, hi]
}

function argErr(msg) {
  throw new Error('invalid argument (kv_invalid_argument): ' + msg)
}

function notFound() {
  throw new Error('key not found (kv_not_found)')
}

function parseIntStrict(s) {
  const n = Number.parseInt(String(s).trim(), 10)
  if (Number.isNaN(n)) argErr('value is not an integer or out of range')
  return n
}

function parseFloatStrict(s) {
  const raw = String(s).trim()
  // Redis 特殊值：+inf / -inf / inf
  if (/^[+-]?inf(inity)?$/i.test(raw)) {
    return raw.startsWith('-') ? Number.NEGATIVE_INFINITY : Number.POSITIVE_INFINITY
  }
  const f = Number.parseFloat(raw)
  if (Number.isNaN(f)) argErr('value is not a float or out of range')
  return f
}

export function createKvMock({ delay }) {
  // 项目 → Map（key → entry）；不再按 databaseId 分桶
  const projects = new Map()
  const store = (projectId) => {
    if (!projects.has(projectId)) projects.set(projectId, new Map())
    return projects.get(projectId)
  }

  // —— 各命令实现：入参 (kv, argv)，返回 Redis 回复（null/number/string/array）——
  const cmds = {
    // —— key ——
    DEL(kv, argv) {
      if (argv.length < 1) argErr('wrong number of arguments')
      let n = 0
      for (const k of argv) {
        if (kvGetAlive(kv, k)) {
          kv.delete(k)
          n++
        }
      }
      return n
    },
    EXISTS(kv, argv) {
      if (argv.length < 1) argErr('wrong number of arguments')
      return argv.reduce((n, k) => n + (kvGetAlive(kv, k) ? 1 : 0), 0)
    },
    EXPIRE(kv, argv) {
      if (argv.length !== 2) argErr('wrong number of arguments')
      const e = kvGetAlive(kv, argv[0])
      if (!e) return 0
      const ms = parseIntStrict(argv[1]) * 1000
      if (ms <= 0) {
        kv.delete(argv[0])
        return 1
      }
      e.etime = Date.now() + ms
      e.version++
      e.mtime = Date.now()
      return 1
    },
    PEXPIRE(kv, argv) {
      if (argv.length !== 2) argErr('wrong number of arguments')
      const e = kvGetAlive(kv, argv[0])
      if (!e) return 0
      const ms = parseIntStrict(argv[1])
      if (ms <= 0) {
        kv.delete(argv[0])
        return 1
      }
      e.etime = Date.now() + ms
      e.version++
      e.mtime = Date.now()
      return 1
    },
    EXPIREAT(kv, argv) {
      if (argv.length !== 2) argErr('wrong number of arguments')
      const e = kvGetAlive(kv, argv[0])
      if (!e) return 0
      const ms = parseIntStrict(argv[1]) * 1000
      if (ms <= Date.now()) {
        kv.delete(argv[0])
        return 1
      }
      e.etime = ms
      e.version++
      e.mtime = Date.now()
      return 1
    },
    PEXPIREAT(kv, argv) {
      if (argv.length !== 2) argErr('wrong number of arguments')
      const e = kvGetAlive(kv, argv[0])
      if (!e) return 0
      const ms = parseIntStrict(argv[1])
      if (ms <= Date.now()) {
        kv.delete(argv[0])
        return 1
      }
      e.etime = ms
      e.version++
      e.mtime = Date.now()
      return 1
    },
    PERSIST(kv, argv) {
      if (argv.length !== 1) argErr('wrong number of arguments')
      const e = kvGetAlive(kv, argv[0])
      if (!e || e.etime == null) return 0
      e.etime = null
      e.version++
      e.mtime = Date.now()
      return 1
    },
    TTL(kv, argv) {
      if (argv.length !== 1) argErr('wrong number of arguments')
      const e = kvGetAlive(kv, argv[0])
      if (!e) return -2
      if (e.etime == null) return -1
      return Math.max(0, Math.ceil((e.etime - Date.now()) / 1000))
    },
    PTTL(kv, argv) {
      if (argv.length !== 1) argErr('wrong number of arguments')
      const e = kvGetAlive(kv, argv[0])
      if (!e) return -2
      if (e.etime == null) return -1
      return Math.max(0, e.etime - Date.now())
    },
    TYPE(kv, argv) {
      if (argv.length !== 1) argErr('wrong number of arguments')
      const e = kvGetAlive(kv, argv[0])
      return e ? e.type : 'none'
    },
    RENAME(kv, argv) {
      if (argv.length !== 2) argErr('wrong number of arguments')
      const src = kvGetAlive(kv, argv[0])
      if (!src) notFound()
      kv.set(argv[1], { ...src, version: src.version + 1, mtime: Date.now() })
      kv.delete(argv[0])
      return 'OK'
    },
    RENAMENX(kv, argv) {
      if (argv.length !== 2) argErr('wrong number of arguments')
      const src = kvGetAlive(kv, argv[0])
      if (!src) notFound()
      if (kvGetAlive(kv, argv[1])) return 0
      kv.set(argv[1], { ...src, version: src.version + 1, mtime: Date.now() })
      kv.delete(argv[0])
      return 1
    },
    DBSIZE(kv, argv) {
      if (argv.length !== 0) argErr('wrong number of arguments')
      let n = 0
      for (const [k] of kv.entries()) if (kvGetAlive(kv, k)) n++
      return n
    },
    SCAN(kv, argv) {
      let cursor = argv[0] === undefined || argv[0] === '' || argv[0] === '0' ? '' : argv[0]
      let pattern = '*'
      let typ = ''
      let count = 100
      let i = 1
      while (i < argv.length) {
        const opt = String(argv[i]).toUpperCase()
        if (opt === 'MATCH') {
          pattern = argv[i + 1]
          i += 2
        } else if (opt === 'COUNT') {
          count = parseIntStrict(argv[i + 1])
          if (count <= 0 || count > 500) argErr('COUNT must be in 1..500')
          i += 2
        } else if (opt === 'TYPE') {
          typ = argv[i + 1]
          i += 2
        } else {
          argErr('unknown option ' + argv[i])
        }
      }
      const re = globToRegExp(pattern)
      const all = []
      for (const [key] of kv.entries()) {
        const e = kvGetAlive(kv, key)
        if (!e) continue
        if (typ && e.type !== typ) continue
        if (!re.test(key)) continue
        all.push(key)
      }
      all.sort()
      const from = cursor ? all.findIndex((k) => k > cursor) : 0
      const page = all.slice(from < 0 ? all.length : from, (from < 0 ? all.length : from) + count)
      const start = cursor ? (from < 0 ? all.length : from) : 0
      const hasMore = start + count < all.length
      return [hasMore ? page[page.length - 1] : '0', page]
    },

    // —— string ——
    GET(kv, argv) {
      if (argv.length !== 1) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'string')
      return e ? e.value : null
    },
    SET(kv, argv) {
      if (argv.length < 2) argErr('wrong number of arguments')
      const key = argv[0]
      const value = String(argv[1])
      const existing = kvGuardType(kv, key, 'string')
      let nx = false
      let xx = false
      let keepTtl = false
      let ttlMs = null
      let i = 2
      while (i < argv.length) {
        const opt = String(argv[i]).toUpperCase()
        if (opt === 'NX') {
          nx = true
          i++
        } else if (opt === 'XX') {
          xx = true
          i++
        } else if (opt === 'KEEPTTL') {
          keepTtl = true
          i++
        } else if (opt === 'GET') {
          argErr('GET option not supported by mock')
        } else if (opt === 'EX' || opt === 'PX' || opt === 'EXAT' || opt === 'PXAT') {
          const n = parseIntStrict(argv[i + 1])
          if (opt === 'EX') ttlMs = n * 1000
          else if (opt === 'PX') ttlMs = n
          else if (opt === 'EXAT') ttlMs = n * 1000 - Date.now()
          else ttlMs = n - Date.now()
          i += 2
        } else {
          argErr('unknown option ' + argv[i])
        }
      }
      if (nx && existing) return null
      if (xx && !existing) return null
      const e = kvTouch(kv, key, 'string', undefined)
      e.value = value
      e.len = null
      if (keepTtl && existing) {
        e.etime = existing.etime
      } else if (ttlMs != null) {
        if (ttlMs <= 0) {
          kv.delete(key)
          return 'OK'
        }
        e.etime = Date.now() + ttlMs
      } else {
        e.etime = null
      }
      return 'OK'
    },
    SETNX(kv, argv) {
      if (argv.length !== 2) argErr('wrong number of arguments')
      if (kvGetAlive(kv, argv[0])) return 0
      const e = kvTouch(kv, argv[0], 'string', undefined)
      e.value = String(argv[1])
      e.len = null
      return 1
    },
    GETSET(kv, argv) {
      if (argv.length !== 2) argErr('wrong number of arguments')
      const existing = kvGuardType(kv, argv[0], 'string')
      const old = existing ? existing.value : null
      const e = kvTouch(kv, argv[0], 'string', undefined)
      e.value = String(argv[1])
      e.len = null
      e.etime = null
      return old
    },
    MGET(kv, argv) {
      if (argv.length < 1) argErr('wrong number of arguments')
      return argv.map((k) => {
        const e = kvGetAlive(kv, k)
        return e && e.type === 'string' ? e.value : null
      })
    },
    MSET(kv, argv) {
      if (argv.length === 0 || argv.length % 2 !== 0) argErr('wrong number of arguments')
      for (let i = 0; i < argv.length; i += 2) {
        kvGuardType(kv, argv[i], 'string')
        const e = kvTouch(kv, argv[i], 'string', undefined)
        e.value = String(argv[i + 1])
        e.len = null
        e.etime = null
      }
      return 'OK'
    },
    INCR(kv, argv) {
      if (argv.length !== 1) argErr('wrong number of arguments')
      return incrBy(kv, argv[0], 1)
    },
    INCRBY(kv, argv) {
      if (argv.length !== 2) argErr('wrong number of arguments')
      return incrBy(kv, argv[0], parseIntStrict(argv[1]))
    },
    DECR(kv, argv) {
      if (argv.length !== 1) argErr('wrong number of arguments')
      return incrBy(kv, argv[0], -1)
    },
    DECRBY(kv, argv) {
      if (argv.length !== 2) argErr('wrong number of arguments')
      return incrBy(kv, argv[0], -parseIntStrict(argv[1]))
    },
    INCRBYFLOAT(kv, argv) {
      if (argv.length !== 2) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'string')
      const cur = e ? Number.parseFloat(String(e.value).trim()) : 0
      if (e && Number.isNaN(cur)) argErr('value is not a float')
      const next = cur + parseFloatStrict(argv[1])
      const entry = kvTouch(kv, argv[0], 'string', undefined)
      entry.value = String(next)
      entry.len = null
      return next
    },

    // —— hash ——
    HGET(kv, argv) {
      if (argv.length !== 2) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'hash')
      const v = e ? e.value[argv[1]] : undefined
      return v === undefined ? null : v
    },
    HMGET(kv, argv) {
      if (argv.length < 2) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'hash')
      return argv.slice(1).map((f) => (e && f in e.value ? e.value[f] : null))
    },
    HSET(kv, argv) {
      if (argv.length < 3 || argv.length % 2 === 0) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'hash')
      const obj = e?.value || {}
      let added = 0
      for (let i = 1; i + 1 < argv.length + 1 && i + 1 <= argv.length; i += 2) {
        if (i >= argv.length) break
        if (!(argv[i] in obj)) added++
        obj[argv[i]] = String(argv[i + 1])
      }
      const entry = kvTouch(kv, argv[0], 'hash', Object.keys(obj).length)
      entry.value = obj
      return added
    },
    HDEL(kv, argv) {
      if (argv.length < 2) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'hash')
      if (!e) return 0
      let n = 0
      for (const f of argv.slice(1)) {
        if (f in e.value) {
          delete e.value[f]
          n++
        }
      }
      if (n > 0) kvTouch(kv, argv[0], 'hash', Object.keys(e.value).length)
      if (Object.keys(e.value).length === 0) kv.delete(argv[0])
      return n
    },
    HGETALL(kv, argv) {
      if (argv.length !== 1) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'hash')
      const out = []
      for (const f of Object.keys(e?.value || {}).sort()) {
        out.push(f, e.value[f])
      }
      return out
    },
    HINCRBY(kv, argv) {
      if (argv.length !== 3) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'hash')
      const obj = e?.value || {}
      const delta = parseIntStrict(argv[2])
      const cur = argv[1] in obj ? Number.parseInt(String(obj[argv[1]]).trim(), 10) : 0
      if (argv[1] in obj && Number.isNaN(cur)) argErr('value is not an integer')
      const next = cur + delta
      obj[argv[1]] = String(next)
      const entry = kvTouch(kv, argv[0], 'hash', Object.keys(obj).length)
      entry.value = obj
      return next
    },
    HLEN(kv, argv) {
      if (argv.length !== 1) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'hash')
      return e ? Object.keys(e.value).length : 0
    },
    HSCAN(kv, argv) {
      if (argv.length < 1 || argv[0] === '') argErr('key is required')
      const key = argv[0]
      let cursor = argv[1] === undefined || argv[1] === '' || argv[1] === '0' ? '' : argv[1]
      let pattern = '*'
      let count = 100
      let i = 2
      while (i < argv.length) {
        const opt = String(argv[i]).toUpperCase()
        if (opt === 'MATCH') {
          pattern = argv[i + 1]
          i += 2
        } else if (opt === 'COUNT') {
          count = parseIntStrict(argv[i + 1])
          if (count <= 0 || count > 500) argErr('COUNT must be in 1..500')
          i += 2
        } else {
          argErr('unknown option ' + argv[i])
        }
      }
      const e = kvGuardType(kv, key, 'hash')
      const re = globToRegExp(pattern)
      const fields = Object.keys(e?.value || {}).sort().filter((f) => re.test(f))
      const from = cursor ? fields.findIndex((f) => f > cursor) : 0
      const start = cursor ? (from < 0 ? fields.length : from) : 0
      const page = fields.slice(start, start + count)
      const flat = []
      for (const f of page) flat.push(f, e.value[f])
      const hasMore = start + count < fields.length
      return [hasMore ? page[page.length - 1] : '0', flat]
    },

    // —— list ——
    LPUSH(kv, argv) {
      if (argv.length < 2) argErr('wrong number of arguments')
      return pushList(kv, argv[0], argv.slice(1), 'front')
    },
    RPUSH(kv, argv) {
      if (argv.length < 2) argErr('wrong number of arguments')
      return pushList(kv, argv[0], argv.slice(1), 'back')
    },
    LPOP(kv, argv) {
      if (argv.length !== 1) argErr('wrong number of arguments')
      return popList(kv, argv[0], 'front')
    },
    RPOP(kv, argv) {
      if (argv.length !== 1) argErr('wrong number of arguments')
      return popList(kv, argv[0], 'back')
    },
    LRANGE(kv, argv) {
      if (argv.length !== 3) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'list')
      if (!e) return []
      const range = normalizeRange(parseIntStrict(argv[1]), parseIntStrict(argv[2]), e.value.length)
      return range ? e.value.slice(range[0], range[1] + 1) : []
    },
    LSET(kv, argv) {
      if (argv.length !== 3) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'list')
      if (!e) notFound()
      const idx = parseIntStrict(argv[1])
      const i = idx < 0 ? e.value.length + idx : idx
      if (i < 0 || i >= e.value.length) notFound()
      e.value[i] = String(argv[2])
      kvTouch(kv, argv[0], 'list', e.value.length)
      return 'OK'
    },
    LTRIM(kv, argv) {
      if (argv.length !== 3) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'list')
      if (!e) return 'OK'
      const range = normalizeRange(parseIntStrict(argv[1]), parseIntStrict(argv[2]), e.value.length)
      if (!range) {
        kv.delete(argv[0])
        return 'OK'
      }
      e.value = e.value.slice(range[0], range[1] + 1)
      kvTouch(kv, argv[0], 'list', e.value.length)
      return 'OK'
    },
    LLEN(kv, argv) {
      if (argv.length !== 1) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'list')
      return e ? e.value.length : 0
    },

    // —— set ——
    SADD(kv, argv) {
      if (argv.length < 2) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'set')
      const arr = e?.value || []
      let added = 0
      for (const el of argv.slice(1)) {
        if (!arr.includes(String(el))) {
          arr.push(String(el))
          added++
        }
      }
      const entry = kvTouch(kv, argv[0], 'set', arr.length)
      entry.value = arr
      return added
    },
    SREM(kv, argv) {
      if (argv.length < 2) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'set')
      if (!e) return 0
      let n = 0
      for (const el of argv.slice(1)) {
        const idx = e.value.indexOf(String(el))
        if (idx >= 0) {
          e.value.splice(idx, 1)
          n++
        }
      }
      if (n > 0) kvTouch(kv, argv[0], 'set', e.value.length)
      if (e.value.length === 0) kv.delete(argv[0])
      return n
    },
    SISMEMBER(kv, argv) {
      if (argv.length !== 2) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'set')
      return e && e.value.includes(String(argv[1])) ? 1 : 0
    },
    SMEMBERS(kv, argv) {
      if (argv.length !== 1) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'set')
      return e ? [...e.value].sort() : []
    },
    SPOP(kv, argv) {
      if (argv.length !== 1) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'set')
      if (!e || e.value.length === 0) return null
      const idx = Math.floor(Math.random() * e.value.length)
      const v = e.value.splice(idx, 1)[0]
      kvTouch(kv, argv[0], 'set', e.value.length)
      if (e.value.length === 0) kv.delete(argv[0])
      return v
    },
    SCARD(kv, argv) {
      if (argv.length !== 1) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'set')
      return e ? e.value.length : 0
    },
    SUNION(kv, argv) {
      if (argv.length < 1) argErr('wrong number of arguments')
      const u = new Set()
      for (const k of argv) {
        const e = kvGuardType(kv, k, 'set')
        if (e) for (const v of e.value) u.add(v)
      }
      return [...u].sort()
    },
    SINTER(kv, argv) {
      if (argv.length < 1) argErr('wrong number of arguments')
      const sets = argv.map((k) => {
        const e = kvGuardType(kv, k, 'set')
        return e ? new Set(e.value) : new Set()
      })
      if (sets.length === 0) return []
      const [first, ...rest] = sets
      return [...first].filter((v) => rest.every((s) => s.has(v))).sort()
    },
    SDIFF(kv, argv) {
      if (argv.length < 1) argErr('wrong number of arguments')
      const sets = argv.map((k) => {
        const e = kvGuardType(kv, k, 'set')
        return e ? new Set(e.value) : new Set()
      })
      const [first, ...rest] = sets
      return first ? [...first].filter((v) => !rest.some((s) => s.has(v))).sort() : []
    },
    SUNIONSTORE(kv, argv) {
      if (argv.length < 2) argErr('wrong number of arguments')
      const result = cmds.SUNION(kv, argv.slice(1))
      return storeSet(kv, argv[0], result)
    },
    SINTERSTORE(kv, argv) {
      if (argv.length < 2) argErr('wrong number of arguments')
      const result = cmds.SINTER(kv, argv.slice(1))
      return storeSet(kv, argv[0], result)
    },
    SDIFFSTORE(kv, argv) {
      if (argv.length < 2) argErr('wrong number of arguments')
      const result = cmds.SDIFF(kv, argv.slice(1))
      return storeSet(kv, argv[0], result)
    },

    // —— zset ——
    ZADD(kv, argv) {
      if (argv.length < 3 || (argv.length - 1) % 2 !== 0) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'zset')
      const arr = e?.value || []
      let added = 0
      for (let i = 1; i < argv.length; i += 2) {
        const score = parseFloatStrict(argv[i])
        const elem = String(argv[i + 1])
        const existing = arr.find((x) => x.elem === elem)
        if (existing) {
          existing.score = score
        } else {
          arr.push({ elem, score })
          added++
        }
      }
      const entry = kvTouch(kv, argv[0], 'zset', arr.length)
      entry.value = arr
      return added
    },
    ZREM(kv, argv) {
      if (argv.length < 2) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'zset')
      if (!e) return 0
      let n = 0
      for (const el of argv.slice(1)) {
        const idx = e.value.findIndex((x) => x.elem === String(el))
        if (idx >= 0) {
          e.value.splice(idx, 1)
          n++
        }
      }
      if (n > 0) kvTouch(kv, argv[0], 'zset', e.value.length)
      if (e.value.length === 0) kv.delete(argv[0])
      return n
    },
    ZSCORE(kv, argv) {
      if (argv.length !== 2) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'zset')
      const it = e?.value.find((x) => x.elem === argv[1])
      return it ? it.score : null
    },
    ZRANK(kv, argv) {
      if (argv.length !== 2) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'zset')
      if (!e || !e.value.some((x) => x.elem === argv[1])) return null
      return sortedZ(e.value).findIndex((x) => x.elem === argv[1])
    },
    ZRANGE(kv, argv) {
      if (argv.length < 3) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'zset')
      if (!e) return []
      let items = sortedZ(e.value)
      let rev = false
      let withScores = false
      for (const opt of argv.slice(3)) {
        const o = String(opt).toUpperCase()
        if (o === 'REV') rev = true
        else if (o === 'WITHSCORES') withScores = true
        else argErr('unknown option ' + opt)
      }
      if (rev) items = [...items].reverse()
      const range = normalizeRange(parseIntStrict(argv[1]), parseIntStrict(argv[2]), items.length)
      items = range ? items.slice(range[0], range[1] + 1) : []
      if (!withScores) return items.map((it) => it.elem)
      const out = []
      for (const it of items) out.push(it.elem, it.score)
      return out
    },
    ZRANGEBYSCORE(kv, argv) {
      if (argv.length < 3) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'zset')
      const min = parseFloatStrict(argv[1])
      const max = parseFloatStrict(argv[2])
      let offset = 0
      let count = 0
      let withScores = false
      let i = 3
      while (i < argv.length) {
        const opt = String(argv[i]).toUpperCase()
        if (opt === 'WITHSCORES') {
          withScores = true
          i++
        } else if (opt === 'LIMIT') {
          offset = parseIntStrict(argv[i + 1])
          count = parseIntStrict(argv[i + 2])
          i += 3
        } else {
          argErr('unknown option ' + argv[i])
        }
      }
      let items = (e?.value || []).filter((it) => it.score >= min && it.score <= max)
      items = sortedZ(items)
      items = count > 0 ? items.slice(offset, offset + count) : items.slice(offset)
      if (!withScores) return items.map((it) => it.elem)
      const out = []
      for (const it of items) out.push(it.elem, it.score)
      return out
    },
    ZINCRBY(kv, argv) {
      if (argv.length !== 3) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'zset')
      const arr = e?.value || []
      const delta = parseFloatStrict(argv[1])
      let it = arr.find((x) => x.elem === argv[2])
      if (it) {
        it.score += delta
      } else {
        it = { elem: String(argv[2]), score: delta }
        arr.push(it)
      }
      const entry = kvTouch(kv, argv[0], 'zset', arr.length)
      entry.value = arr
      return it.score
    },
    ZCOUNT(kv, argv) {
      if (argv.length !== 3) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'zset')
      const min = parseFloatStrict(argv[1])
      const max = parseFloatStrict(argv[2])
      return (e?.value || []).filter((it) => it.score >= min && it.score <= max).length
    },
    ZCARD(kv, argv) {
      if (argv.length !== 1) argErr('wrong number of arguments')
      const e = kvGuardType(kv, argv[0], 'zset')
      return e ? e.value.length : 0
    }
  }

  function incrBy(kv, key, delta) {
    const e = kvGuardType(kv, key, 'string')
    const cur = e ? Number.parseInt(String(e.value).trim(), 10) : 0
    if (e && Number.isNaN(cur)) argErr('value is not an integer')
    const next = cur + delta
    const entry = kvTouch(kv, key, 'string', undefined)
    entry.value = String(next)
    entry.len = null
    return next
  }

  function pushList(kv, key, elems, side) {
    const e = kvGuardType(kv, key, 'list')
    const arr = e?.value || []
    for (const el of side === 'front' ? [...elems].reverse() : elems) {
      if (side === 'front') arr.unshift(String(el))
      else arr.push(String(el))
    }
    const entry = kvTouch(kv, key, 'list', arr.length)
    entry.value = arr
    return arr.length
  }

  function popList(kv, key, side) {
    const e = kvGuardType(kv, key, 'list')
    if (!e || e.value.length === 0) return null
    const v = side === 'front' ? e.value.shift() : e.value.pop()
    kvTouch(kv, key, 'list', e.value.length)
    if (e.value.length === 0) kv.delete(key)
    return v
  }

  function storeSet(kv, dest, result) {
    if (result.length === 0) {
      kv.delete(dest)
      return 0
    }
    const e = kvTouch(kv, dest, 'set', result.length)
    e.value = [...result]
    return result.length
  }

  function sortedZ(arr) {
    return [...arr].sort(
      (a, b) => a.score - b.score || (a.elem < b.elem ? -1 : a.elem > b.elem ? 1 : 0)
    )
  }

  // —— 类型化写入（type=String/Hash/...）：转发为等价 cmd 实现 ——
  function execTyped(kv, type, args) {
    if (!args || typeof args !== 'object') argErr('args required')
    if (!args.key) argErr('key is required')
    const ttl = args.ttl_ms
    if (ttl !== undefined && ttl !== null && (typeof ttl !== 'number' || ttl < 0)) {
      argErr('ttl_ms must be >= 0')
    }
    let result
    switch (type) {
      case 'String': {
        if (args.value === undefined || args.value === null) argErr('value is required')
        const argv = ['SET', args.key, String(args.value)]
        if (args.nx) argv.push('NX')
        if (args.xx) argv.push('XX')
        if (args.keep_ttl) argv.push('KEEPTTL')
        const raw = cmds.SET(kv, argv.slice(1))
        result = raw === null ? null : 'OK'
        break
      }
      case 'Hash': {
        const fields = args.fields || {}
        const names = Object.keys(fields)
        if (names.length === 0) argErr('fields must be non-empty')
        if (names.length > 1000) argErr('fields must be 1..1000')
        const argv = ['HSET', args.key]
        for (const f of names) argv.push(f, String(fields[f]))
        result = cmds.HSET(kv, argv.slice(1))
        break
      }
      case 'List': {
        const elems = args.elems || []
        if (elems.length === 0) argErr('elems must be non-empty')
        if (elems.length > 1000) argErr('elems must be 1..1000')
        const side = args.side || 'back'
        if (side !== 'back' && side !== 'front') argErr('side must be back|front')
        const argv = [side === 'front' ? 'LPUSH' : 'RPUSH', args.key, ...elems.map(String)]
        result = cmds[argv[0]](kv, argv.slice(1))
        break
      }
      case 'Set': {
        const elems = args.elems || []
        if (elems.length === 0) argErr('elems must be non-empty')
        if (elems.length > 1000) argErr('elems must be 1..1000')
        const argv = ['SADD', args.key, ...elems.map(String)]
        result = cmds.SADD(kv, argv.slice(1))
        break
      }
      case 'ZSet': {
        const items = args.items || []
        if (items.length === 0) argErr('items must be non-empty')
        if (items.length > 1000) argErr('items must be 1..1000')
        const argv = ['ZADD', args.key]
        for (const it of items) argv.push(String(Number(it.score)), String(it.elem))
        result = cmds.ZADD(kv, argv.slice(1))
        break
      }
      default:
        argErr('unknown type ' + type)
    }
    if (ttl !== undefined && ttl !== null) {
      if (ttl === 0) {
        cmds.DEL(kv, [args.key])
      } else {
        cmds.PEXPIRE(kv, [args.key, String(ttl)])
      }
    }
    return result
  }

  return {
    /** 测试辅助：清空全部项目的内存 KV */
    reset() {
      projects.clear()
    },

    async exec(projectId, body) {
      await delay()
      const kv = store(projectId)
      if (!body || typeof body !== 'object') argErr('malformed body')
      if (body.type === 'cmd') {
        if (!Array.isArray(body.argvs) || body.argvs.length === 0) {
          argErr('argvs must be a non-empty array')
        }
        const name = String(body.argvs[0]).toUpperCase()
        const fn = cmds[name]
        if (!fn) throw new Error('unknown command (kv_unknown_command): ' + name)
        return fn(kv, body.argvs.slice(1).map(String))
      }
      if (['String', 'Hash', 'List', 'Set', 'ZSet'].includes(body.type)) {
        return execTyped(kv, body.type, body.args)
      }
      argErr('type is required')
    },

    async execBatch(projectId, bodies) {
      const out = []
      for (const b of bodies) {
        out.push(await this.exec(projectId, b))
      }
      return out
    }
  }
}
