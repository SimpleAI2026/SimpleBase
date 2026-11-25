// 文件说明：kv_cmd_set.go — KV 命令表的集合命令（SADD / SMEMBERS / SUNION 系列）
//
// 用法
//
//	kvSetCommands 由 kv_commands.go 的 kvCommandTable 在包初始化时合并；每条命令是 kvCmdSpec（parse 校验参数并返回绑定 *kv.Tx 的执行函数）。
//
// 调用架构
//
//	[上游] kv_handler.go（按命令名查 kvCommandTable）
//	         ▼
//	[本文件] internal/api/kv_cmd_set.go
//	         ▼
//	[同包] kv_commands.go（kvCmdSpec / 参数解析与回复辅助）→ godatabase/kv
//
// 主要符号
//
//	var kvSetCommands = map[string]kvCmdSpec{…}
//	命令：SADD SREM SISMEMBER SMEMBERS SPOP SCARD SUNION SINTER SDIFF SUNIONSTORE SINTERSTORE SDIFFSTORE

package api

import (
	"context"
	"errors"

	"github.com/linkxzhou/SimpleBase/godatabase/kv"
)

// kvSetCommands 是集合命令（SADD / SMEMBERS / SUNION 系列）。
var kvSetCommands = map[string]kvCmdSpec{
	"SADD": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(argv, 2); err != nil {
				return nil, err
			}
			key := argv[0]
			elems := toBytes(argv[1:])
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return tx.Set().Add(ctx, key, elems...) }, nil
		},
	},
	"SREM": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(argv, 2); err != nil {
				return nil, err
			}
			key := argv[0]
			elems := toBytes(argv[1:])
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return tx.Set().Delete(ctx, key, elems...) }, nil
		},
	},
	"SISMEMBER": {
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 2); err != nil {
				return nil, err
			}
			key, elem := argv[0], argv[1]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				ok, err := tx.Set().IsMember(ctx, key, []byte(elem))
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
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				elems, err := tx.Set().Members(ctx, key)
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
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				v, err := tx.Set().Pop(ctx, key)
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
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				m, err := tx.Key().Get(ctx, key)
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
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(argv, 1); err != nil {
				return nil, err
			}
			keys := append([]string(nil), argv...)
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				elems, err := tx.Set().Union(ctx, keys...)
				if err != nil {
					return nil, err
				}
				return toStrings(elems), nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return []any{}, nil },
	},
	"SINTER": {
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(argv, 1); err != nil {
				return nil, err
			}
			keys := append([]string(nil), argv...)
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				elems, err := tx.Set().Inter(ctx, keys...)
				if err != nil {
					return nil, err
				}
				return toStrings(elems), nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return []any{}, nil },
	},
	"SDIFF": {
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(argv, 1); err != nil {
				return nil, err
			}
			keys := append([]string(nil), argv...)
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				elems, err := tx.Set().Diff(ctx, keys...)
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
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(argv, 2); err != nil {
				return nil, err
			}
			dest := argv[0]
			keys := append([]string(nil), argv[1:]...)
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return tx.Set().UnionStore(ctx, dest, keys...) }, nil
		},
	},
	"SINTERSTORE": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(argv, 2); err != nil {
				return nil, err
			}
			dest := argv[0]
			keys := append([]string(nil), argv[1:]...)
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return tx.Set().InterStore(ctx, dest, keys...) }, nil
		},
	},
	"SDIFFSTORE": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(argv, 2); err != nil {
				return nil, err
			}
			dest := argv[0]
			keys := append([]string(nil), argv[1:]...)
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return tx.Set().DiffStore(ctx, dest, keys...) }, nil
		},
	},
}
