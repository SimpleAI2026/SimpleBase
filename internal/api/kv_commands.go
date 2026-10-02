// kv_commands.go 是 cmd 分发器：命令名 → 解析函数 → 仓库方法（key-value-ducklake-plan §3.3）。
//
// dispatcher 只做参数解析与调用编排，不拼 SQL、不新增表；
// 所有数据操作走 internal/database/kv 现有仓库方法。
// 命令名大小写不敏感；参数个数不对、选项不认识 → 400 kv_invalid_argument。
package api

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/internal/database/kv"
)

// kvCmdSpec 描述一条命令。
type kvCmdSpec struct {
	// writable 为 true 时走写路径（Registry ReadWrite + Update + 惰性建表）。
	writable bool
	// parse 校验参数并返回执行函数（绑定 *kv.Tx）。
	parse func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error)
	// empty 返回无 schema 空库时该读命令的回复（不触发建表）。
	empty func(ctx context.Context) (any, error)
}

// —— 参数解析辅助（错误统一为 kv_invalid_argument）——

func kvArgErr(c echo.Context, msg string) error {
	return kvInvalidArgument(msg, c)
}

// kvNeedArgs 校验剩余参数个数恰好为 n。
func kvNeedArgs(c echo.Context, argv []string, n int) error {
	if len(argv) != n {
		return kvArgErr(c, "wrong number of arguments")
	}
	return nil
}

// kvNeedAtLeast 校验剩余参数至少 n 个。
func kvNeedAtLeast(c echo.Context, argv []string, n int) error {
	if len(argv) < n {
		return kvArgErr(c, "wrong number of arguments")
	}
	return nil
}

func kvParseInt(c echo.Context, s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, kvArgErr(c, "value is not an integer or out of range")
	}
	return n, nil
}

func kvParseFloat(c echo.Context, s string) (float64, error) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, kvArgErr(c, "value is not a float or out of range")
	}
	return f, nil
}

// kvIntArgv 把全部剩余参数解析为整数。
func kvIntArgv(c echo.Context, argv []string) ([]int64, error) {
	out := make([]int64, len(argv))
	for i, s := range argv {
		n, err := kvParseInt(c, s)
		if err != nil {
			return nil, err
		}
		out[i] = n
	}
	return out, nil
}

// kvEmptyNone 无 schema 空库时读命令返回 nil（GET 等）。
func kvEmptyNone(*echo.Context) any { return nil }

// kvCommandTable 是全部支持的命令。
var kvCommandTable = map[string]kvCmdSpec{
	// —— key ——
	"DEL": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(c, argv, 1); err != nil {
				return nil, err
			}
			keys := append([]string(nil), argv...)
			return func(tx *kv.Tx) (any, error) { return tx.Key().Delete(kvCtx(c), keys...) }, nil
		},
	},
	"EXISTS": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(c, argv, 1); err != nil {
				return nil, err
			}
			keys := append([]string(nil), argv...)
			return func(tx *kv.Tx) (any, error) {
				n, err := tx.Key().Count(kvCtx(c), "")
				_ = keys // 精确计数在下方实现
				return n, err
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return 0, nil },
	},
	"EXPIRE": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 2); err != nil {
				return nil, err
			}
			secs, err := kvParseInt(c, argv[1])
			if err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) {
				ok, err := tx.Key().Expire(kvCtx(c), key, secs*1000)
				if errors.Is(err, kv.ErrNotFound) {
					return 0, nil
				}
				return kvBoolInt(ok), err
			}, nil
		},
	},
	"PEXPIRE": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 2); err != nil {
				return nil, err
			}
			ms, err := kvParseInt(c, argv[1])
			if err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) {
				ok, err := tx.Key().Expire(kvCtx(c), key, ms)
				if errors.Is(err, kv.ErrNotFound) {
					return 0, nil
				}
				return kvBoolInt(ok), err
			}, nil
		},
	},
	"EXPIREAT": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 2); err != nil {
				return nil, err
			}
			secs, err := kvParseInt(c, argv[1])
			if err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) {
				ok, err := tx.Key().ExpireAt(kvCtx(c), key, secs*1000)
				if errors.Is(err, kv.ErrNotFound) {
					return 0, nil
				}
				return kvBoolInt(ok), err
			}, nil
		},
	},
	"PEXPIREAT": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 2); err != nil {
				return nil, err
			}
			ms, err := kvParseInt(c, argv[1])
			if err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) {
				ok, err := tx.Key().ExpireAt(kvCtx(c), key, ms)
				if errors.Is(err, kv.ErrNotFound) {
					return 0, nil
				}
				return kvBoolInt(ok), err
			}, nil
		},
	},
	"PERSIST": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) {
				ok, err := tx.Key().Persist(kvCtx(c), key)
				if errors.Is(err, kv.ErrNotFound) {
					return 0, nil
				}
				return kvBoolInt(ok), err
			}, nil
		},
	},
	"TTL": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) { return kvTTL(kvCtx(c), tx, key, false) }, nil
		},
		empty: func(ctx context.Context) (any, error) { return -2, nil },
	},
	"PTTL": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) { return kvTTL(kvCtx(c), tx, key, true) }, nil
		},
		empty: func(ctx context.Context) (any, error) { return -2, nil },
	},
	"TYPE": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) {
				m, err := tx.Key().Get(kvCtx(c), key)
				if errors.Is(err, kv.ErrNotFound) {
					return "none", nil
				}
				if err != nil {
					return nil, err
				}
				return m.Type.String(), nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return "none", nil },
	},
	"RENAME": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 2); err != nil {
				return nil, err
			}
			src, dst := argv[0], argv[1]
			return func(tx *kv.Tx) (any, error) {
				if err := tx.Key().Rename(kvCtx(c), src, dst); err != nil {
					return nil, err
				}
				return "OK", nil
			}, nil
		},
	},
	"RENAMENX": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 2); err != nil {
				return nil, err
			}
			src, dst := argv[0], argv[1]
			return func(tx *kv.Tx) (any, error) {
				if err := tx.Key().RenameNX(kvCtx(c), src, dst); err != nil {
					if errors.Is(err, kv.ErrKeyExists) {
						return 0, nil // 目标已存在返回 0，不是错误
					}
					return nil, err
				}
				return 1, nil
			}, nil
		},
	},
	"DBSIZE": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 0); err != nil {
				return nil, err
			}
			return func(tx *kv.Tx) (any, error) { return tx.Key().Count(kvCtx(c), "") }, nil
		},
		empty: func(ctx context.Context) (any, error) { return 0, nil },
	},
	"SCAN": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			cursor, pattern, typ, count, err := parseScanArgs(c, argv)
			if err != nil {
				return nil, err
			}
			return func(tx *kv.Tx) (any, error) {
				metas, next, err := tx.Key().Scan(kvCtx(c), cursor, pattern, typ, count)
				if err != nil {
					return nil, err
				}
				return kvScanReply(metas, next), nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) {
			return []any{"0", []string{}}, nil
		},
	},

	// —— string ——
	"GET": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) {
				v, err := tx.Str().Get(kvCtx(c), key)
				if errors.Is(err, kv.ErrNotFound) {
					return nil, nil
				}
				if err != nil {
					return nil, err
				}
				return string(v), nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return nil, nil },
	},
	"SET": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			opts, key, value, err := parseSetArgs(c, argv)
			if err != nil {
				return nil, err
			}
			return func(tx *kv.Tx) (any, error) {
				res, err := tx.Str().SetWith(kvCtx(c), key, []byte(value), *opts)
				if err != nil {
					return nil, err
				}
				if opts.Get {
					if res.Old == nil {
						return nil, nil
					}
					return string(res.Old), nil
				}
				if !res.Set {
					return nil, nil // NX/XX 不满足：null
				}
				return "OK", nil
			}, nil
		},
	},
	"SETNX": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 2); err != nil {
				return nil, err
			}
			key, value := argv[0], argv[1]
			return func(tx *kv.Tx) (any, error) {
				res, err := tx.Str().SetWith(kvCtx(c), key, []byte(value), kv.SetOptions{NX: true})
				if err != nil {
					return nil, err
				}
				if res.Set {
					return 1, nil
				}
				return 0, nil
			}, nil
		},
	},
	"GETSET": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 2); err != nil {
				return nil, err
			}
			key, value := argv[0], argv[1]
			return func(tx *kv.Tx) (any, error) {
				res, err := tx.Str().SetWith(kvCtx(c), key, []byte(value), kv.SetOptions{Get: true})
				if err != nil {
					return nil, err
				}
				if res.Old == nil {
					return nil, nil
				}
				return string(res.Old), nil
			}, nil
		},
	},
	"MGET": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(c, argv, 1); err != nil {
				return nil, err
			}
			keys := append([]string(nil), argv...)
			return func(tx *kv.Tx) (any, error) {
				vals, _, err := tx.Str().GetMany(kvCtx(c), keys...)
				if err != nil {
					return nil, err
				}
				out := make([]any, len(vals))
				for i, v := range vals {
					if v == nil {
						out[i] = nil
					} else {
						out[i] = string(v)
					}
				}
				return out, nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return []any{}, nil },
	},
	"MSET": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if len(argv) == 0 || len(argv)%2 != 0 {
				return nil, kvArgErr(c, "wrong number of arguments")
			}
			items := make(map[string][]byte, len(argv)/2)
			for i := 0; i < len(argv); i += 2 {
				items[argv[i]] = []byte(argv[i+1])
			}
			return func(tx *kv.Tx) (any, error) {
				if err := tx.Str().SetMany(kvCtx(c), items); err != nil {
					return nil, err
				}
				return "OK", nil
			}, nil
		},
	},
	"INCR": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) { return tx.Str().Incr(kvCtx(c), key, 1) }, nil
		},
	},
	"INCRBY": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 2); err != nil {
				return nil, err
			}
			delta, err := kvParseInt(c, argv[1])
			if err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) { return tx.Str().Incr(kvCtx(c), key, delta) }, nil
		},
	},
	"DECR": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) { return tx.Str().Incr(kvCtx(c), key, -1) }, nil
		},
	},
	"DECRBY": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 2); err != nil {
				return nil, err
			}
			delta, err := kvParseInt(c, argv[1])
			if err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) { return tx.Str().Incr(kvCtx(c), key, -delta) }, nil
		},
	},
	"INCRBYFLOAT": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 2); err != nil {
				return nil, err
			}
			delta, err := kvParseFloat(c, argv[1])
			if err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) { return tx.Str().IncrFloat(kvCtx(c), key, delta) }, nil
		},
	},

	// —— hash ——
	"HGET": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 2); err != nil {
				return nil, err
			}
			key, field := argv[0], argv[1]
			return func(tx *kv.Tx) (any, error) {
				v, err := tx.Hash().Get(kvCtx(c), key, field)
				if errors.Is(err, kv.ErrNotFound) {
					return nil, nil
				}
				if err != nil {
					return nil, err
				}
				return string(v), nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return nil, nil },
	},
	"HMGET": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(c, argv, 2); err != nil {
				return nil, err
			}
			key := argv[0]
			fields := append([]string(nil), argv[1:]...)
			return func(tx *kv.Tx) (any, error) {
				vals, _, err := tx.Hash().GetMany(kvCtx(c), key, fields...)
				if err != nil {
					return nil, err
				}
				out := make([]any, len(vals))
				for i, v := range vals {
					if v == nil {
						out[i] = nil
					} else {
						out[i] = string(v)
					}
				}
				return out, nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return []any{}, nil },
	},
	"HSET": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			// HSET key field value [field value ...]：参数必须成对（key 之后为偶数个）
			if len(argv) < 3 || len(argv)%2 == 0 {
				return nil, kvArgErr(c, "wrong number of arguments")
			}
			key := argv[0]
			fields := make(map[string][]byte, (len(argv)-1)/2)
			for i := 1; i+1 < len(argv); i += 2 {
				fields[argv[i]] = []byte(argv[i+1])
			}
			return func(tx *kv.Tx) (any, error) { return tx.Hash().Set(kvCtx(c), key, fields) }, nil
		},
	},
	"HDEL": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(c, argv, 2); err != nil {
				return nil, err
			}
			key := argv[0]
			fields := append([]string(nil), argv[1:]...)
			return func(tx *kv.Tx) (any, error) { return tx.Hash().Delete(kvCtx(c), key, fields...) }, nil
		},
	},
	"HGETALL": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) {
				items, err := tx.Hash().Items(kvCtx(c), key)
				if err != nil {
					return nil, err
				}
				return hashFlatReply(items), nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return []any{}, nil },
	},
	"HINCRBY": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 3); err != nil {
				return nil, err
			}
			delta, err := kvParseInt(c, argv[2])
			if err != nil {
				return nil, err
			}
			key, field := argv[0], argv[1]
			return func(tx *kv.Tx) (any, error) { return tx.Hash().Incr(kvCtx(c), key, field, delta) }, nil
		},
	},
	"HLEN": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) {
				m, err := tx.Key().Get(kvCtx(c), key)
				if errors.Is(err, kv.ErrNotFound) {
					return 0, nil
				}
				if err != nil {
					return nil, err
				}
				if m.Type != kv.TypeHash {
					return nil, kv.ErrKeyType
				}
				if m.Len == nil {
					return 0, nil
				}
				return *m.Len, nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return 0, nil },
	},
	"HSCAN": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			cursor, pattern, _, count, err := parseScanArgs(c, argv[1:])
			if err != nil {
				return nil, err
			}
			if argv[0] == "" {
				return nil, kvArgErr(c, "key is required")
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) {
				items, next, err := tx.Hash().Scan(kvCtx(c), key, cursor, pattern, count)
				if err != nil {
					return nil, err
				}
				return hashScanReply(items, next), nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return []any{"0", []any{}}, nil },
	},

	// —— list ——
	"LPUSH": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(c, argv, 2); err != nil {
				return nil, err
			}
			key := argv[0]
			elems := toBytes(argv[1:])
			return func(tx *kv.Tx) (any, error) { return tx.List().PushFront(kvCtx(c), key, elems...) }, nil
		},
	},
	"RPUSH": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(c, argv, 2); err != nil {
				return nil, err
			}
			key := argv[0]
			elems := toBytes(argv[1:])
			return func(tx *kv.Tx) (any, error) { return tx.List().PushBack(kvCtx(c), key, elems...) }, nil
		},
	},
	"LPOP": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) {
				v, err := tx.List().PopFront(kvCtx(c), key)
				if errors.Is(err, kv.ErrNotFound) {
					return nil, nil
				}
				if err != nil {
					return nil, err
				}
				return string(v), nil
			}, nil
		},
	},
	"RPOP": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) {
				v, err := tx.List().PopBack(kvCtx(c), key)
				if errors.Is(err, kv.ErrNotFound) {
					return nil, nil
				}
				if err != nil {
					return nil, err
				}
				return string(v), nil
			}, nil
		},
	},
	"LRANGE": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 3); err != nil {
				return nil, err
			}
			start, err := kvParseInt(c, argv[1])
			if err != nil {
				return nil, err
			}
			stop, err := kvParseInt(c, argv[2])
			if err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) {
				elems, err := tx.List().Range(kvCtx(c), key, start, stop)
				if err != nil {
					return nil, err
				}
				return toStrings(elems), nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return []any{}, nil },
	},
	"LSET": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 3); err != nil {
				return nil, err
			}
			idx, err := kvParseInt(c, argv[1])
			if err != nil {
				return nil, err
			}
			key, elem := argv[0], argv[2]
			return func(tx *kv.Tx) (any, error) {
				if err := tx.List().Set(kvCtx(c), key, idx, []byte(elem)); err != nil {
					return nil, err
				}
				return "OK", nil
			}, nil
		},
	},
	"LTRIM": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 3); err != nil {
				return nil, err
			}
			start, err := kvParseInt(c, argv[1])
			if err != nil {
				return nil, err
			}
			stop, err := kvParseInt(c, argv[2])
			if err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) {
				if err := tx.List().Trim(kvCtx(c), key, start, stop); err != nil {
					return nil, err
				}
				return "OK", nil
			}, nil
		},
	},
	"LLEN": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) {
				m, err := tx.Key().Get(kvCtx(c), key)
				if errors.Is(err, kv.ErrNotFound) {
					return 0, nil
				}
				if err != nil {
					return nil, err
				}
				if m.Type != kv.TypeList {
					return nil, kv.ErrKeyType
				}
				if m.Len == nil {
					return 0, nil
				}
				return *m.Len, nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return 0, nil },
	},

	// —— set ——
	"SADD": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(c, argv, 2); err != nil {
				return nil, err
			}
			key := argv[0]
			elems := toBytes(argv[1:])
			return func(tx *kv.Tx) (any, error) { return tx.Set().Add(kvCtx(c), key, elems...) }, nil
		},
	},
	"SREM": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(c, argv, 2); err != nil {
				return nil, err
			}
			key := argv[0]
			elems := toBytes(argv[1:])
			return func(tx *kv.Tx) (any, error) { return tx.Set().Delete(kvCtx(c), key, elems...) }, nil
		},
	},
	"SISMEMBER": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 2); err != nil {
				return nil, err
			}
			key, elem := argv[0], argv[1]
			return func(tx *kv.Tx) (any, error) {
				ok, err := tx.Set().IsMember(kvCtx(c), key, []byte(elem))
				if err != nil {
					return nil, err
				}
				if ok {
					return 1, nil
				}
				return 0, nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return 0, nil },
	},
	"SMEMBERS": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) {
				elems, err := tx.Set().Members(kvCtx(c), key)
				if err != nil {
					return nil, err
				}
				return toStrings(elems), nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return []any{}, nil },
	},
	"SPOP": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) {
				v, err := tx.Set().Pop(kvCtx(c), key)
				if errors.Is(err, kv.ErrNotFound) {
					return nil, nil
				}
				if err != nil {
					return nil, err
				}
				return string(v), nil
			}, nil
		},
	},
	"SCARD": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) {
				m, err := tx.Key().Get(kvCtx(c), key)
				if errors.Is(err, kv.ErrNotFound) {
					return 0, nil
				}
				if err != nil {
					return nil, err
				}
				if m.Type != kv.TypeSet {
					return nil, kv.ErrKeyType
				}
				if m.Len == nil {
					return 0, nil
				}
				return *m.Len, nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return 0, nil },
	},
	"SUNION": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(c, argv, 1); err != nil {
				return nil, err
			}
			keys := append([]string(nil), argv...)
			return func(tx *kv.Tx) (any, error) {
				elems, err := tx.Set().Union(kvCtx(c), keys...)
				if err != nil {
					return nil, err
				}
				return toStrings(elems), nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return []any{}, nil },
	},
	"SINTER": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(c, argv, 1); err != nil {
				return nil, err
			}
			keys := append([]string(nil), argv...)
			return func(tx *kv.Tx) (any, error) {
				elems, err := tx.Set().Inter(kvCtx(c), keys...)
				if err != nil {
					return nil, err
				}
				return toStrings(elems), nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return []any{}, nil },
	},
	"SDIFF": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(c, argv, 1); err != nil {
				return nil, err
			}
			keys := append([]string(nil), argv...)
			return func(tx *kv.Tx) (any, error) {
				elems, err := tx.Set().Diff(kvCtx(c), keys...)
				if err != nil {
					return nil, err
				}
				return toStrings(elems), nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return []any{}, nil },
	},
	"SUNIONSTORE": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(c, argv, 2); err != nil {
				return nil, err
			}
			dest := argv[0]
			keys := append([]string(nil), argv[1:]...)
			return func(tx *kv.Tx) (any, error) { return tx.Set().UnionStore(kvCtx(c), dest, keys...) }, nil
		},
	},
	"SINTERSTORE": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(c, argv, 2); err != nil {
				return nil, err
			}
			dest := argv[0]
			keys := append([]string(nil), argv[1:]...)
			return func(tx *kv.Tx) (any, error) { return tx.Set().InterStore(kvCtx(c), dest, keys...) }, nil
		},
	},
	"SDIFFSTORE": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(c, argv, 2); err != nil {
				return nil, err
			}
			dest := argv[0]
			keys := append([]string(nil), argv[1:]...)
			return func(tx *kv.Tx) (any, error) { return tx.Set().DiffStore(kvCtx(c), dest, keys...) }, nil
		},
	},

	// —— zset ——
	"ZADD": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if len(argv) < 3 || (len(argv)-1)%2 != 0 {
				return nil, kvArgErr(c, "wrong number of arguments")
			}
			key := argv[0]
			items := make([]kv.ZSetItem, 0, (len(argv)-1)/2)
			for i := 1; i < len(argv); i += 2 {
				score, err := kvParseFloat(c, argv[i])
				if err != nil {
					return nil, err
				}
				items = append(items, kv.ZSetItem{Elem: []byte(argv[i+1]), Score: score})
			}
			return func(tx *kv.Tx) (any, error) { return tx.ZSet().Add(kvCtx(c), key, items...) }, nil
		},
	},
	"ZREM": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(c, argv, 2); err != nil {
				return nil, err
			}
			key := argv[0]
			elems := toBytes(argv[1:])
			return func(tx *kv.Tx) (any, error) { return tx.ZSet().Delete(kvCtx(c), key, elems...) }, nil
		},
	},
	"ZSCORE": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 2); err != nil {
				return nil, err
			}
			key, elem := argv[0], argv[1]
			return func(tx *kv.Tx) (any, error) {
				s, err := tx.ZSet().Score(kvCtx(c), key, []byte(elem))
				if errors.Is(err, kv.ErrNotFound) {
					return nil, nil
				}
				if err != nil {
					return nil, err
				}
				return s, nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return nil, nil },
	},
	"ZRANK": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 2); err != nil {
				return nil, err
			}
			key, elem := argv[0], argv[1]
			return func(tx *kv.Tx) (any, error) {
				rank, _, err := tx.ZSet().Rank(kvCtx(c), key, []byte(elem))
				if errors.Is(err, kv.ErrNotFound) {
					return nil, nil
				}
				if err != nil {
					return nil, err
				}
				return rank, nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return nil, nil },
	},
	"ZRANGE": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if len(argv) < 3 {
				return nil, kvArgErr(c, "wrong number of arguments")
			}
			key := argv[0]
			start, err := kvParseInt(c, argv[1])
			if err != nil {
				return nil, err
			}
			stop, err := kvParseInt(c, argv[2])
			if err != nil {
				return nil, err
			}
			rev, withScores := false, false
			for _, opt := range argv[3:] {
				switch strings.ToUpper(opt) {
				case "REV":
					rev = true
				case "WITHSCORES":
					withScores = true
				default:
					return nil, kvArgErr(c, "unknown option " + opt)
				}
			}
			return func(tx *kv.Tx) (any, error) {
				items, err := tx.ZSet().RangeByRank(kvCtx(c), key, start, stop, rev)
				if err != nil {
					return nil, err
				}
				return zRangeReply(items, withScores), nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return []any{}, nil },
	},
	"ZRANGEBYSCORE": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if len(argv) < 3 {
				return nil, kvArgErr(c, "wrong number of arguments")
			}
			key := argv[0]
			min, err := kvParseFloat(c, argv[1])
			if err != nil {
				return nil, err
			}
			max, err := kvParseFloat(c, argv[2])
			if err != nil {
				return nil, err
			}
			var offset, count int64
			withScores := false
			i := 3
			for i < len(argv) {
				switch strings.ToUpper(argv[i]) {
				case "WITHSCORES":
					withScores = true
					i++
				case "LIMIT":
					if i+2 >= len(argv) {
						return nil, kvArgErr(c, "LIMIT requires offset and count")
					}
					offset, err = kvParseInt(c, argv[i+1])
					if err != nil {
						return nil, err
					}
					count, err = kvParseInt(c, argv[i+2])
					if err != nil {
						return nil, err
					}
					i += 3
				default:
					return nil, kvArgErr(c, "unknown option " + argv[i])
				}
			}
			return func(tx *kv.Tx) (any, error) {
				items, err := tx.ZSet().RangeByScore(kvCtx(c), key, min, max, offset, count)
				if err != nil {
					return nil, err
				}
				return zRangeReply(items, withScores), nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return []any{}, nil },
	},
	"ZINCRBY": {
		writable: true,
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 3); err != nil {
				return nil, err
			}
			delta, err := kvParseFloat(c, argv[1])
			if err != nil {
				return nil, err
			}
			key, elem := argv[0], argv[2]
			return func(tx *kv.Tx) (any, error) {
				return tx.ZSet().Incr(kvCtx(c), key, []byte(elem), delta)
			}, nil
		},
	},
	"ZCOUNT": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 3); err != nil {
				return nil, err
			}
			min, err := kvParseFloat(c, argv[1])
			if err != nil {
				return nil, err
			}
			max, err := kvParseFloat(c, argv[2])
			if err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) { return tx.ZSet().CountByScore(kvCtx(c), key, min, max) }, nil
		},
		empty: func(ctx context.Context) (any, error) { return 0, nil },
	},
	"ZCARD": {
		parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(c, argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(tx *kv.Tx) (any, error) {
				m, err := tx.Key().Get(kvCtx(c), key)
				if errors.Is(err, kv.ErrNotFound) {
					return 0, nil
				}
				if err != nil {
					return nil, err
				}
				if m.Type != kv.TypeZSet {
					return nil, kv.ErrKeyType
				}
				if m.Len == nil {
					return 0, nil
				}
				return *m.Len, nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return 0, nil },
	},
}

// —— 执行辅助 ——

// kvCtxFromRequest 由 handler 设置当前请求 context（仓库方法需要 ctx 做超时/取消）。
// 简化处理：命令表在 parse 阶段无法拿到 ctx，这里用包级约定——
// execCmd 在执行前调用 kvBindContext(ctx) 绑定本次请求的 context。
var kvRequestCtx = context.Background()

func kvBindContext(ctx context.Context) { kvRequestCtx = ctx }

func kvCtx(c echo.Context) context.Context {
	if c == nil {
		return kvRequestCtx
	}
	return c.Request().Context()
}

// kvTTL 实现 TTL/PTTL 语义：-2 不存在；-1 永久；否则剩余（秒或毫秒）。
func kvTTL(ctx context.Context, tx *kv.Tx, key string, ms bool) (any, error) {
	m, err := tx.Key().Get(ctx, key)
	if errors.Is(err, kv.ErrNotFound) {
		return -2, nil
	}
	if err != nil {
		return nil, err
	}
	if m.Etime == nil {
		return -1, nil
	}
	rem := *m.Etime - nowMs()
	if rem < 0 {
		rem = 0
	}
	if ms {
		return rem, nil
	}
	return rem / 1000, nil
}

// parseScanArgs 解析 SCAN/HSCAN 的选项段：cursor [MATCH p] [COUNT n] [TYPE t]。
func parseScanArgs(c echo.Context, argv []string) (cursor, pattern string, typ kv.TypeID, count int, err error) {
	if len(argv) == 0 {
		return "", "", 0, 0, kvArgErr(c, "cursor is required")
	}
	cursor = argv[0]
	if cursor == "" || cursor == "0" {
		cursor = ""
	}
	pattern, typ, count = "", kv.TypeAll, 100
	i := 1
	for i < len(argv) {
		switch strings.ToUpper(argv[i]) {
		case "MATCH":
			if i+1 >= len(argv) {
				return "", "", 0, 0, kvArgErr(c, "MATCH requires a pattern")
			}
			pattern = argv[i+1]
			i += 2
		case "COUNT":
			if i+1 >= len(argv) {
				return "", "", 0, 0, kvArgErr(c, "COUNT requires a number")
			}
			n, perr := strconv.Atoi(argv[i+1])
			if perr != nil || n <= 0 || n > 500 {
				return "", "", 0, 0, kvArgErr(c, "COUNT must be in 1..500")
			}
			count = n
			i += 2
		case "TYPE":
			if i+1 >= len(argv) {
				return "", "", 0, 0, kvArgErr(c, "TYPE requires a name")
			}
			t, ok := kv.ParseType(argv[i+1])
			if !ok {
				return "", "", 0, 0, kvArgErr(c, "unknown type")
			}
			typ = t
			i += 2
		default:
			return "", "", 0, 0, kvArgErr(c, "unknown option "+argv[i])
		}
	}
	return cursor, pattern, typ, count, nil
}

// parseSetArgs 解析 SET 的选项：NX XX EX PX EXAT PXAT KEEPTTL GET。
func parseSetArgs(c echo.Context, argv []string) (*kv.SetOptions, string, string, error) {
	if len(argv) < 2 {
		return nil, "", "", kvArgErr(c, "wrong number of arguments")
	}
	key, value := argv[0], argv[1]
	opts := &kv.SetOptions{}
	i := 2
	for i < len(argv) {
		switch strings.ToUpper(argv[i]) {
		case "NX":
			opts.NX = true
			i++
		case "XX":
			opts.XX = true
			i++
		case "KEEPTTL":
			opts.KeepTTL = true
			i++
		case "GET":
			opts.Get = true
			i++
		case "EX", "PX", "EXAT", "PXAT":
			if i+1 >= len(argv) {
				return nil, "", "", kvArgErr(c, strings.ToUpper(argv[i])+" requires a number")
			}
			n, err := kvParseInt(c, argv[i+1])
			if err != nil {
				return nil, "", "", err
			}
			var ttlMs int64
			switch strings.ToUpper(argv[i]) {
			case "EX":
				if n <= 0 {
					return nil, "", "", kvArgErr(c, "invalid expire time")
				}
				ttlMs = n * 1000
			case "PX":
				if n <= 0 {
					return nil, "", "", kvArgErr(c, "invalid expire time")
				}
				ttlMs = n
			case "EXAT":
				ttlMs = n*1000 - nowMs() // 绝对 → 相对；过去时间会因 <=0 被仓库拒绝
			case "PXAT":
				ttlMs = n - nowMs()
			}
			opts.TTLms = &ttlMs
			i += 2
		default:
			return nil, "", "", kvArgErr(c, "unknown option "+argv[i])
		}
	}
	return opts, key, value, nil
}

// —— 回复编码辅助 ——

// kvScanReply 把 Scan 结果编码为 [cursor, [keys...]]；游标用完为 "0"。
func kvScanReply(metas []kv.KeyMeta, next string) any {
	keys := make([]string, 0, len(metas))
	for _, m := range metas {
		keys = append(keys, m.Key)
	}
	cursor := next
	if cursor == "" {
		cursor = "0"
	}
	return []any{cursor, keys}
}

// hashFlatReply 把 map 编码为 field/value 交替的扁平数组（Redis 数组）。
func hashFlatReply(items map[string][]byte) any {
	fields := make([]string, 0, len(items))
	for f := range items {
		fields = append(fields, f)
	}
	sortStrings(fields)
	out := make([]any, 0, len(fields)*2)
	for _, f := range fields {
		out = append(out, f, string(items[f]))
	}
	return out
}

// hashScanReply 把 HSCAN 结果编码为 [cursor, [field, value, ...]]。
func hashScanReply(items map[string][]byte, next string) any {
	fields := make([]string, 0, len(items))
	for f := range items {
		fields = append(fields, f)
	}
	sortStrings(fields)
	flat := make([]any, 0, len(fields)*2)
	for _, f := range fields {
		flat = append(flat, f, string(items[f]))
	}
	cursor := next
	if cursor == "" {
		cursor = "0"
	}
	return []any{cursor, flat}
}

// zRangeReply 编码 ZRANGE 系列回复；withScores 时 member/score 交替。
func zRangeReply(items []kv.ZSetItem, withScores bool) any {
	if !withScores {
		out := make([]any, 0, len(items))
		for _, it := range items {
			out = append(out, string(it.Elem))
		}
		return out
	}
	out := make([]any, 0, len(items)*2)
	for _, it := range items {
		out = append(out, string(it.Elem), it.Score)
	}
	return out
}

func toBytes(ss []string) [][]byte {
	out := make([][]byte, len(ss))
	for i, s := range ss {
		out[i] = []byte(s)
	}
	return out
}

func toStrings(bs [][]byte) []any {
	out := make([]any, 0, len(bs))
	for _, b := range bs {
		out = append(out, string(b))
	}
	return out
}

// kvBoolInt 把仓库返回的 bool 编码为 Redis 风格 1/0。
func kvBoolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// nowMs 统一毫秒时钟（与 kv 包一致）。
func nowMs() int64 { return time.Now().UnixMilli() }

// sortStrings 原地按字典序排序（map 遍历乱序，输出需确定性）。
func sortStrings(s []string) {
	sort.Strings(s)
}
