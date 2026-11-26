package datahttp

import (
	"context"
	"errors"

	"github.com/linkxzhou/SimpleBase/godatabase/kv"
)

// kvListCommands 是列表命令（LPUSH / LPOP / LRANGE 等）。
var kvListCommands = map[string]kvCmdSpec{
	"LPUSH": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(argv, 2); err != nil {
				return nil, err
			}
			key := argv[0]
			elems := toBytes(argv[1:])
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return tx.List().PushFront(ctx, key, elems...) }, nil
		},
	},
	"RPUSH": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(argv, 2); err != nil {
				return nil, err
			}
			key := argv[0]
			elems := toBytes(argv[1:])
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return tx.List().PushBack(ctx, key, elems...) }, nil
		},
	},
	"LPOP": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				v, err := tx.List().PopFront(ctx, key)
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
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 1); err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				v, err := tx.List().PopBack(ctx, key)
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
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 3); err != nil {
				return nil, err
			}
			start, err := kvParseInt(argv[1])
			if err != nil {
				return nil, err
			}
			stop, err := kvParseInt(argv[2])
			if err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				elems, err := tx.List().Range(ctx, key, start, stop)
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
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 3); err != nil {
				return nil, err
			}
			idx, err := kvParseInt(argv[1])
			if err != nil {
				return nil, err
			}
			key, elem := argv[0], argv[2]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				if err := tx.List().Set(ctx, key, idx, []byte(elem)); err != nil {
					return nil, err
				}
				return "OK", nil
			}, nil
		},
	},
	"LTRIM": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 3); err != nil {
				return nil, err
			}
			start, err := kvParseInt(argv[1])
			if err != nil {
				return nil, err
			}
			stop, err := kvParseInt(argv[2])
			if err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				if err := tx.List().Trim(ctx, key, start, stop); err != nil {
					return nil, err
				}
				return "OK", nil
			}, nil
		},
	},
	"LLEN": {
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
}
