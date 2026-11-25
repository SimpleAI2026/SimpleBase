// 文件说明：kv_cmd_key.go — KV 命令表的键管理命令（DEL / EXPIRE / TTL / RENAME / SCAN 等）
//
// 用法
//
//	kvKeyCommands 由 kv_commands.go 的 kvCommandTable 在包初始化时合并；每条命令是 kvCmdSpec（parse 校验参数并返回绑定 *kv.Tx 的执行函数）。
//
// 调用架构
//
//	[上游] kv_handler.go（按命令名查 kvCommandTable）
//	         ▼
//	[本文件] internal/api/kv_cmd_key.go
//	         ▼
//	[同包] kv_commands.go（kvCmdSpec / 参数解析与回复辅助）→ godatabase/kv
//
// 主要符号
//
//	var kvKeyCommands = map[string]kvCmdSpec{…}
//	命令：DEL EXISTS EXPIRE PEXPIRE EXPIREAT PEXPIREAT PERSIST TTL PTTL TYPE RENAME RENAMENX DBSIZE SCAN

package api

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
