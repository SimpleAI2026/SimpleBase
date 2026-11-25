// 文件说明：kv_cmd_string.go — KV 命令表的字符串命令（GET / SET / MGET / INCR 系列）
//
// 用法
//
//	kvStringCommands 由 kv_commands.go 的 kvCommandTable 在包初始化时合并；每条命令是 kvCmdSpec（parse 校验参数并返回绑定 *kv.Tx 的执行函数）。
//
// 调用架构
//
//	[上游] kv_handler.go（按命令名查 kvCommandTable）
//	         ▼
//	[本文件] internal/api/kv_cmd_string.go
//	         ▼
//	[同包] kv_commands.go（kvCmdSpec / 参数解析与回复辅助）→ godatabase/kv
//
// 主要符号
//
//	var kvStringCommands = map[string]kvCmdSpec{…}
//	命令：GET SET SETNX GETSET MGET MSET INCR INCRBY DECR DECRBY INCRBYFLOAT

package api

import (
	"context"
	"errors"

	"github.com/linkxzhou/SimpleBase/godatabase/kv"
)

// kvStringCommands 是字符串命令（GET / SET / MGET / INCR 系列）。
var kvStringCommands = map[string]kvCmdSpec{
	"GET": {
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				v, err := tx.Str().Get(ctx, key)
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
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			opts, key, value, err := parseSetArgs(argv)
			if err != nil {
				return nil, err
			}
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				res, err := tx.Str().SetWith(ctx, key, []byte(value), *opts)
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
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 2); err != nil {
				return nil, err
			}
			key, value := argv[0], argv[1]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				res, err := tx.Str().SetWith(ctx, key, []byte(value), kv.SetOptions{NX: true})
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
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 2); err != nil {
				return nil, err
			}
			key, value := argv[0], argv[1]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				res, err := tx.Str().SetWith(ctx, key, []byte(value), kv.SetOptions{Get: true})
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
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(argv, 1); err != nil {
				return nil, err
			}
			keys := append([]string(nil), argv...)
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				vals, _, err := tx.Str().GetMany(ctx, keys...)
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
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if len(argv) == 0 || len(argv)%2 != 0 {
				return nil, kvArgErr("wrong number of arguments")
			}
			items := make(map[string][]byte, len(argv)/2)
			for i := 0; i < len(argv); i += 2 {
				items[argv[i]] = []byte(argv[i+1])
			}
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				if err := tx.Str().SetMany(ctx, items); err != nil {
					return nil, err
				}
				return "OK", nil
			}, nil
		},
	},
	"INCR": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return tx.Str().Incr(ctx, key, 1) }, nil
		},
	},
	"INCRBY": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 2); err != nil {
				return nil, err
			}
			delta, err := kvParseInt(argv[1])
			if err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return tx.Str().Incr(ctx, key, delta) }, nil
		},
	},
	"DECR": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return tx.Str().Incr(ctx, key, -1) }, nil
		},
	},
	"DECRBY": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 2); err != nil {
				return nil, err
			}
			delta, err := kvParseInt(argv[1])
			if err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return tx.Str().Incr(ctx, key, -delta) }, nil
		},
	},
	"INCRBYFLOAT": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 2); err != nil {
				return nil, err
			}
			delta, err := kvParseFloat(argv[1])
			if err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return tx.Str().IncrFloat(ctx, key, delta) }, nil
		},
	},
}
