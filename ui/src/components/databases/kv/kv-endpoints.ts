/**
 * KV 命令清单（key-value-ducklake-plan §3.3，与后端 kv_commands.go 命令表对应）。
 * 单端点 POST /v1/projects/:projectId/kv，body {type:"cmd",argvs:[...]}。
 * 供「Key-Value → API」子页签展示命令、生成 curl 片段与试调用。
 * 纯静态数据，不从后端拉取。
 */

export interface KvCommand {
  /** 命令名（展示用，大写） */
  name: string
  /** 是否写命令（决定 curl 示例与「数据」页签联动刷新） */
  write: boolean
  /** 分组 */
  group: KvCommandGroup
  /** 参数签名（展示用，如 "key seconds"） */
  args: string
  /** 一句话语义（中文原文，UI 已改用 descKey；保留作为数据参考） */
  desc: string
  /** 语义的 i18n key（components.kvApi.cmdDesc.<NAME>） */
  descKey: string
  /** 试调用的 argv 模板；"{key}"/"{value}" 由面板替换 */
  argv: string[]
  /** 回复形态说明 */
  reply: string
}

export const KV_COMMAND_GROUPS = [
  { value: 'key', label: 'Key' },
  { value: 'string', label: 'String' },
  { value: 'hash', label: 'Hash' },
  { value: 'list', label: 'List' },
  { value: 'set', label: 'Set' },
  { value: 'zset', label: 'ZSet' }
] as const

export type KvCommandGroup = (typeof KV_COMMAND_GROUPS)[number]['value']

export const KV_COMMANDS: KvCommand[] = [
  // —— key ——
  { name: 'SCAN', write: false, group: 'key', args: 'cursor [MATCH p] [COUNT n] [TYPE t]', desc: '增量遍历 key；游标用完为 "0"', descKey: 'components.kvApi.cmdDesc.SCAN', argv: ['SCAN', '0', 'COUNT', '10'], reply: '[游标, [key...]]' },
  { name: 'DBSIZE', write: false, group: 'key', args: '', desc: '当前 key 总数', descKey: 'components.kvApi.cmdDesc.DBSIZE', argv: ['DBSIZE'], reply: '数字' },
  { name: 'TYPE', write: false, group: 'key', args: 'key', desc: 'key 的存储类型', descKey: 'components.kvApi.cmdDesc.TYPE', argv: ['TYPE', '{key}'], reply: '"string" 等' },
  { name: 'TTL', write: false, group: 'key', args: 'key', desc: '剩余秒数；-1 永久；-2 不存在', descKey: 'components.kvApi.cmdDesc.TTL', argv: ['TTL', '{key}'], reply: '数字' },
  { name: 'PTTL', write: false, group: 'key', args: 'key', desc: '剩余毫秒数；-1 永久；-2 不存在', descKey: 'components.kvApi.cmdDesc.PTTL', argv: ['PTTL', '{key}'], reply: '数字' },
  { name: 'EXISTS', write: false, group: 'key', args: 'key [key...]', desc: '存在的 key 个数', descKey: 'components.kvApi.cmdDesc.EXISTS', argv: ['EXISTS', '{key}'], reply: '数字' },
  { name: 'DEL', write: true, group: 'key', args: 'key [key...]', desc: '删除 key，返回删除数', descKey: 'components.kvApi.cmdDesc.DEL', argv: ['DEL', '{key}'], reply: '数字' },
  { name: 'EXPIRE', write: true, group: 'key', args: 'key seconds', desc: '相对过期（秒）', descKey: 'components.kvApi.cmdDesc.EXPIRE', argv: ['EXPIRE', '{key}', '60'], reply: '1 / 0' },
  { name: 'PEXPIRE', write: true, group: 'key', args: 'key ms', desc: '相对过期（毫秒）', descKey: 'components.kvApi.cmdDesc.PEXPIRE', argv: ['PEXPIRE', '{key}', '60000'], reply: '1 / 0' },
  { name: 'EXPIREAT', write: true, group: 'key', args: 'key unix-seconds', desc: '绝对过期（Unix 秒）', descKey: 'components.kvApi.cmdDesc.EXPIREAT', argv: ['EXPIREAT', '{key}', '1790000000'], reply: '1 / 0' },
  { name: 'PERSIST', write: true, group: 'key', args: 'key', desc: '移除过期时间（转永久）', descKey: 'components.kvApi.cmdDesc.PERSIST', argv: ['PERSIST', '{key}'], reply: '1 / 0' },
  { name: 'RENAME', write: true, group: 'key', args: 'key newkey', desc: '重命名，目标存在则覆盖', descKey: 'components.kvApi.cmdDesc.RENAME', argv: ['RENAME', '{key}', 'renamed'], reply: '"OK"' },

  // —— string ——
  { name: 'GET', write: false, group: 'string', args: 'key', desc: '读取 string 值', descKey: 'components.kvApi.cmdDesc.GET', argv: ['GET', '{key}'], reply: '字符串或 null' },
  { name: 'SET', write: true, group: 'string', args: 'key value [EX s|PX ms|NX|XX|KEEPTTL|GET]', desc: '写入 string 值（可选条件与 TTL）', descKey: 'components.kvApi.cmdDesc.SET', argv: ['SET', '{key}', '{value}'], reply: '"OK" / null' },
  { name: 'SETNX', write: true, group: 'string', args: 'key value', desc: '仅不存在时写入', descKey: 'components.kvApi.cmdDesc.SETNX', argv: ['SETNX', '{key}', '{value}'], reply: '1 / 0' },
  { name: 'GETSET', write: true, group: 'string', args: 'key value', desc: '写入并返回旧值', descKey: 'components.kvApi.cmdDesc.GETSET', argv: ['GETSET', '{key}', '{value}'], reply: '旧值或 null' },
  { name: 'MGET', write: false, group: 'string', args: 'key [key...]', desc: '批量读取（缺失为 null）', descKey: 'components.kvApi.cmdDesc.MGET', argv: ['MGET', '{key}', 'other'], reply: '[值...]' },
  { name: 'MSET', write: true, group: 'string', args: 'key value [key value...]', desc: '批量写入', descKey: 'components.kvApi.cmdDesc.MSET', argv: ['MSET', '{key}', '{value}', 'k2', 'v2'], reply: '"OK"' },
  { name: 'INCR', write: true, group: 'string', args: 'key', desc: '自增 1', descKey: 'components.kvApi.cmdDesc.INCR', argv: ['INCR', '{key}'], reply: '数字' },
  { name: 'INCRBY', write: true, group: 'string', args: 'key delta', desc: '整数增减', descKey: 'components.kvApi.cmdDesc.INCRBY', argv: ['INCRBY', '{key}', '5'], reply: '数字' },
  { name: 'INCRBYFLOAT', write: true, group: 'string', args: 'key delta', desc: '浮点增减', descKey: 'components.kvApi.cmdDesc.INCRBYFLOAT', argv: ['INCRBYFLOAT', '{key}', '1.5'], reply: '数字' },

  // —— hash ——
  { name: 'HGET', write: false, group: 'hash', args: 'key field', desc: '读单个字段', descKey: 'components.kvApi.cmdDesc.HGET', argv: ['HGET', '{key}', 'field'], reply: '字符串或 null' },
  { name: 'HMGET', write: false, group: 'hash', args: 'key field [field...]', desc: '批量读字段', descKey: 'components.kvApi.cmdDesc.HMGET', argv: ['HMGET', '{key}', 'f1', 'f2'], reply: '[值...]' },
  { name: 'HSET', write: true, group: 'hash', args: 'key field value [field value...]', desc: '合并写字段，返回新增数', descKey: 'components.kvApi.cmdDesc.HSET', argv: ['HSET', '{key}', 'field', '{value}'], reply: '数字' },
  { name: 'HDEL', write: true, group: 'hash', args: 'key field [field...]', desc: '删字段', descKey: 'components.kvApi.cmdDesc.HDEL', argv: ['HDEL', '{key}', 'field'], reply: '数字' },
  { name: 'HGETALL', write: false, group: 'hash', args: 'key', desc: '全部字段（扁平数组）', descKey: 'components.kvApi.cmdDesc.HGETALL', argv: ['HGETALL', '{key}'], reply: '[field, value, ...]' },
  { name: 'HINCRBY', write: true, group: 'hash', args: 'key field delta', desc: '字段整数增减', descKey: 'components.kvApi.cmdDesc.HINCRBY', argv: ['HINCRBY', '{key}', 'field', '3'], reply: '数字' },
  { name: 'HLEN', write: false, group: 'hash', args: 'key', desc: '字段数', descKey: 'components.kvApi.cmdDesc.HLEN', argv: ['HLEN', '{key}'], reply: '数字' },
  { name: 'HSCAN', write: false, group: 'hash', args: 'key cursor [MATCH p] [COUNT n]', desc: '增量遍历字段', descKey: 'components.kvApi.cmdDesc.HSCAN', argv: ['HSCAN', '{key}', '0'], reply: '[游标, [field, value, ...]]' },

  // —— list ——
  { name: 'LPUSH', write: true, group: 'list', args: 'key elem [elem...]', desc: '左端插入，返回长度', descKey: 'components.kvApi.cmdDesc.LPUSH', argv: ['LPUSH', '{key}', '{value}'], reply: '数字' },
  { name: 'RPUSH', write: true, group: 'list', args: 'key elem [elem...]', desc: '右端插入，返回长度', descKey: 'components.kvApi.cmdDesc.RPUSH', argv: ['RPUSH', '{key}', '{value}'], reply: '数字' },
  { name: 'LPOP', write: true, group: 'list', args: 'key', desc: '左端弹出', descKey: 'components.kvApi.cmdDesc.LPOP', argv: ['LPOP', '{key}'], reply: '元素或 null' },
  { name: 'RPOP', write: true, group: 'list', args: 'key', desc: '右端弹出', descKey: 'components.kvApi.cmdDesc.RPOP', argv: ['RPOP', '{key}'], reply: '元素或 null' },
  { name: 'LRANGE', write: false, group: 'list', args: 'key start stop', desc: '区间读取（负数倒数）', descKey: 'components.kvApi.cmdDesc.LRANGE', argv: ['LRANGE', '{key}', '0', '-1'], reply: '[元素...]' },
  { name: 'LSET', write: true, group: 'list', args: 'key index elem', desc: '按下标改值', descKey: 'components.kvApi.cmdDesc.LSET', argv: ['LSET', '{key}', '0', '{value}'], reply: '"OK"' },
  { name: 'LTRIM', write: true, group: 'list', args: 'key start stop', desc: '区间保留其余删除', descKey: 'components.kvApi.cmdDesc.LTRIM', argv: ['LTRIM', '{key}', '0', '1'], reply: '"OK"' },
  { name: 'LLEN', write: false, group: 'list', args: 'key', desc: '长度', descKey: 'components.kvApi.cmdDesc.LLEN', argv: ['LLEN', '{key}'], reply: '数字' },

  // —— set ——
  { name: 'SADD', write: true, group: 'set', args: 'key elem [elem...]', desc: '添加成员，返回新增数', descKey: 'components.kvApi.cmdDesc.SADD', argv: ['SADD', '{key}', '{value}'], reply: '数字' },
  { name: 'SREM', write: true, group: 'set', args: 'key elem [elem...]', desc: '移除成员', descKey: 'components.kvApi.cmdDesc.SREM', argv: ['SREM', '{key}', '{value}'], reply: '数字' },
  { name: 'SISMEMBER', write: false, group: 'set', args: 'key elem', desc: '成员是否存在', descKey: 'components.kvApi.cmdDesc.SISMEMBER', argv: ['SISMEMBER', '{key}', '{value}'], reply: '1 / 0' },
  { name: 'SMEMBERS', write: false, group: 'set', args: 'key', desc: '全部成员（排序）', descKey: 'components.kvApi.cmdDesc.SMEMBERS', argv: ['SMEMBERS', '{key}'], reply: '[成员...]' },
  { name: 'SPOP', write: true, group: 'set', args: 'key', desc: '随机弹出一个成员', descKey: 'components.kvApi.cmdDesc.SPOP', argv: ['SPOP', '{key}'], reply: '成员或 null' },
  { name: 'SCARD', write: false, group: 'set', args: 'key', desc: '成员数', descKey: 'components.kvApi.cmdDesc.SCARD', argv: ['SCARD', '{key}'], reply: '数字' },
  { name: 'SUNION', write: false, group: 'set', args: 'key [key...]', desc: '并集', descKey: 'components.kvApi.cmdDesc.SUNION', argv: ['SUNION', '{key}', 'other'], reply: '[成员...]' },
  { name: 'SINTER', write: false, group: 'set', args: 'key [key...]', desc: '交集', descKey: 'components.kvApi.cmdDesc.SINTER', argv: ['SINTER', '{key}', 'other'], reply: '[成员...]' },
  { name: 'SDIFF', write: false, group: 'set', args: 'key [key...]', desc: '差集', descKey: 'components.kvApi.cmdDesc.SDIFF', argv: ['SDIFF', '{key}', 'other'], reply: '[成员...]' },
  { name: 'SUNIONSTORE', write: true, group: 'set', args: 'dest key [key...]', desc: '并集存入 dest', descKey: 'components.kvApi.cmdDesc.SUNIONSTORE', argv: ['SUNIONSTORE', 'dest', '{key}', 'other'], reply: '数字' },

  // —— zset ——
  { name: 'ZADD', write: true, group: 'zset', args: 'key score elem [score elem...]', desc: '添加/改分，返回新增数', descKey: 'components.kvApi.cmdDesc.ZADD', argv: ['ZADD', '{key}', '1.5', '{value}'], reply: '数字' },
  { name: 'ZREM', write: true, group: 'zset', args: 'key elem [elem...]', desc: '移除成员', descKey: 'components.kvApi.cmdDesc.ZREM', argv: ['ZREM', '{key}', '{value}'], reply: '数字' },
  { name: 'ZSCORE', write: false, group: 'zset', args: 'key elem', desc: '查分数', descKey: 'components.kvApi.cmdDesc.ZSCORE', argv: ['ZSCORE', '{key}', '{value}'], reply: '数字或 null' },
  { name: 'ZRANK', write: false, group: 'zset', args: 'key elem', desc: '按分数排名（0 起）', descKey: 'components.kvApi.cmdDesc.ZRANK', argv: ['ZRANK', '{key}', '{value}'], reply: '数字或 null' },
  { name: 'ZRANGE', write: false, group: 'zset', args: 'key start stop [REV] [WITHSCORES]', desc: '按排名区间取成员', descKey: 'components.kvApi.cmdDesc.ZRANGE', argv: ['ZRANGE', '{key}', '0', '-1', 'WITHSCORES'], reply: '[成员...] 或 [成员, 分数, ...]' },
  { name: 'ZRANGEBYSCORE', write: false, group: 'zset', args: 'key min max [WITHSCORES] [LIMIT o c]', desc: '按分数区间取成员', descKey: 'components.kvApi.cmdDesc.ZRANGEBYSCORE', argv: ['ZRANGEBYSCORE', '{key}', '-inf', '+inf'], reply: '[成员...]' },
  { name: 'ZINCRBY', write: true, group: 'zset', args: 'key delta elem', desc: '成员分数增减', descKey: 'components.kvApi.cmdDesc.ZINCRBY', argv: ['ZINCRBY', '{key}', '0.5', '{value}'], reply: '数字' },
  { name: 'ZCOUNT', write: false, group: 'zset', args: 'key min max', desc: '分数区间成员数', descKey: 'components.kvApi.cmdDesc.ZCOUNT', argv: ['ZCOUNT', '{key}', '-inf', '+inf'], reply: '数字' },
  { name: 'ZCARD', write: false, group: 'zset', args: 'key', desc: '成员数', descKey: 'components.kvApi.cmdDesc.ZCARD', argv: ['ZCARD', '{key}'], reply: '数字' }
]

/** 单端点路径：项目级，无 databaseId */
export function kvBasePath(projectId: string): string {
  return '/v1/projects/' + encodeURIComponent(projectId) + '/kv'
}

/** 生成 curl 片段（命令 → 单端点 body） */
export function kvCurlSnippet(cmd: KvCommand, baseFull: string): string {
  const argvs = JSON.stringify(cmd.argv)
  const body = `{"type":"cmd","argvs":${argvs}}`
  return [
    `curl -X POST '${baseFull}' \\`,
    `  -H 'Authorization: Bearer <API_KEY>' \\`,
    `  -H 'Content-Type: application/json' \\`,
    `  -d '${body}'`
  ].join('\n')
}
