package datahttp

import (
	"context"
	"errors"

	"github.com/linkxzhou/SimpleBase/godatabase/kv"
)

// kvHashCommands 是哈希命令（HGET / HSET / HGETALL / HSCAN 等）。
var kvHashCommands = map[string]kvCmdSpec{
	"HGET": {
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 2); err != nil {
				return nil, err
			}
			key, field := argv[0], argv[1]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				v, err := tx.Hash().Get(ctx, key, field)
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
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(argv, 2); err != nil {
				return nil, err
			}
			key := argv[0]
			fields := append([]string(nil), argv[1:]...)
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				vals, _, err := tx.Hash().GetMany(ctx, key, fields...)
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
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			// HSET key field value [field value ...]：参数必须成对（key 之后为偶数个）
			if len(argv) < 3 || len(argv)%2 == 0 {
				return nil, kvArgErr("wrong number of arguments")
			}
			key := argv[0]
			fields := make(map[string][]byte, (len(argv)-1)/2)
			for i := 1; i+1 < len(argv); i += 2 {
				fields[argv[i]] = []byte(argv[i+1])
			}
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return tx.Hash().Set(ctx, key, fields) }, nil
		},
	},
	"HDEL": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(argv, 2); err != nil {
				return nil, err
			}
			key := argv[0]
			fields := append([]string(nil), argv[1:]...)
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return tx.Hash().Delete(ctx, key, fields...) }, nil
		},
	},
	"HGETALL": {
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				items, err := tx.Hash().Items(ctx, key)
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
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 3); err != nil {
				return nil, err
			}
			delta, err := kvParseInt(argv[2])
			if err != nil {
				return nil, err
			}
			key, field := argv[0], argv[1]
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return tx.Hash().Incr(ctx, key, field, delta) }, nil
		},
	},
	"HLEN": {
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
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			cursor, pattern, _, count, err := parseScanArgs(argv[1:])
			if err != nil {
				return nil, err
			}
			if argv[0] == "" {
				return nil, kvArgErr("key is required")
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				items, next, err := tx.Hash().Scan(ctx, key, cursor, pattern, count)
				if err != nil {
					return nil, err
				}
				return hashScanReply(items, next), nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return []any{"0", []any{}}, nil },
	},
}
