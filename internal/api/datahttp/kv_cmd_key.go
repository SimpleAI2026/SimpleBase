package datahttp

import (
	"context"
	"errors"

	"github.com/linkxzhou/SimpleBase/godatabase/kv"
)

// kvKeyCommands 是键管理命令（DEL / EXPIRE / TTL / RENAME / SCAN 等）。
var kvKeyCommands = map[string]kvCmdSpec{
	"DEL": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(argv, 1); err != nil {
				return nil, err
			}
			keys := append([]string(nil), argv...)
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return tx.Key().Delete(ctx, keys...) }, nil
		},
	},
	"EXISTS": {
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(argv, 1); err != nil {
				return nil, err
			}
			keys := append([]string(nil), argv...)
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return tx.Key().Exists(ctx, keys...) }, nil
		},
		empty: func(ctx context.Context) (any, error) { return 0, nil },
	},
	"EXPIRE": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 2); err != nil {
				return nil, err
			}
			secs, err := kvParseInt(argv[1])
			if err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				ok, err := tx.Key().Expire(ctx, key, secs*1000)
				if errors.Is(err, kv.ErrNotFound) {
					return 0, nil
				}
				return kvBoolInt(ok), err
			}, nil
		},
	},
	"PEXPIRE": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 2); err != nil {
				return nil, err
			}
			ms, err := kvParseInt(argv[1])
			if err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				ok, err := tx.Key().Expire(ctx, key, ms)
				if errors.Is(err, kv.ErrNotFound) {
					return 0, nil
				}
				return kvBoolInt(ok), err
			}, nil
		},
	},
	"EXPIREAT": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 2); err != nil {
				return nil, err
			}
			secs, err := kvParseInt(argv[1])
			if err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				ok, err := tx.Key().ExpireAt(ctx, key, secs*1000)
				if errors.Is(err, kv.ErrNotFound) {
					return 0, nil
				}
				return kvBoolInt(ok), err
			}, nil
		},
	},
	"PEXPIREAT": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 2); err != nil {
				return nil, err
			}
			ms, err := kvParseInt(argv[1])
			if err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				ok, err := tx.Key().ExpireAt(ctx, key, ms)
				if errors.Is(err, kv.ErrNotFound) {
					return 0, nil
				}
				return kvBoolInt(ok), err
			}, nil
		},
	},
	"PERSIST": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				ok, err := tx.Key().Persist(ctx, key)
				if errors.Is(err, kv.ErrNotFound) {
					return 0, nil
				}
				return kvBoolInt(ok), err
			}, nil
		},
	},
	"TTL": {
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return kvTTL(ctx, tx, key, false) }, nil
		},
		empty: func(ctx context.Context) (any, error) { return -2, nil },
	},
	"PTTL": {
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return kvTTL(ctx, tx, key, true) }, nil
		},
		empty: func(ctx context.Context) (any, error) { return -2, nil },
	},
	"TYPE": {
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				m, err := tx.Key().Get(ctx, key)
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
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 2); err != nil {
				return nil, err
			}
			src, dst := argv[0], argv[1]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				if err := tx.Key().Rename(ctx, src, dst); err != nil {
					return nil, err
				}
				return "OK", nil
			}, nil
		},
	},
	"RENAMENX": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 2); err != nil {
				return nil, err
			}
			src, dst := argv[0], argv[1]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				if err := tx.Key().RenameNX(ctx, src, dst); err != nil {
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
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 0); err != nil {
				return nil, err
			}
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return tx.Key().Count(ctx, "") }, nil
		},
		empty: func(ctx context.Context) (any, error) { return 0, nil },
	},
	"SCAN": {
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			cursor, pattern, typ, count, err := parseScanArgs(argv)
			if err != nil {
				return nil, err
			}
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				metas, next, err := tx.Key().Scan(ctx, cursor, pattern, typ, count)
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
}
