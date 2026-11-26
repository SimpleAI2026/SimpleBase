package datahttp

import (
	"context"
	"errors"
	"strings"

	"github.com/linkxzhou/SimpleBase/godatabase/kv"
)

// kvZSetCommands 是有序集合命令（ZADD / ZRANGE / ZRANGEBYSCORE 等）。
var kvZSetCommands = map[string]kvCmdSpec{
	"ZADD": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if len(argv) < 3 || (len(argv)-1)%2 != 0 {
				return nil, kvArgErr("wrong number of arguments")
			}
			key := argv[0]
			items := make([]kv.ZSetItem, 0, (len(argv)-1)/2)
			for i := 1; i < len(argv); i += 2 {
				score, err := kvParseFloat(argv[i])
				if err != nil {
					return nil, err
				}
				items = append(items, kv.ZSetItem{Elem: []byte(argv[i+1]), Score: score})
			}
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return tx.ZSet().Add(ctx, key, items...) }, nil
		},
	},
	"ZREM": {
		writable: true,
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedAtLeast(argv, 2); err != nil {
				return nil, err
			}
			key := argv[0]
			elems := toBytes(argv[1:])
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return tx.ZSet().Delete(ctx, key, elems...) }, nil
		},
	},
	"ZSCORE": {
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 2); err != nil {
				return nil, err
			}
			key, elem := argv[0], argv[1]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				s, err := tx.ZSet().Score(ctx, key, []byte(elem))
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
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 2); err != nil {
				return nil, err
			}
			key, elem := argv[0], argv[1]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				rank, _, err := tx.ZSet().Rank(ctx, key, []byte(elem))
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
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if len(argv) < 3 {
				return nil, kvArgErr("wrong number of arguments")
			}
			key := argv[0]
			start, err := kvParseInt(argv[1])
			if err != nil {
				return nil, err
			}
			stop, err := kvParseInt(argv[2])
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
					return nil, kvArgErr("unknown option " + opt)
				}
			}
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				items, err := tx.ZSet().RangeByRank(ctx, key, start, stop, rev)
				if err != nil {
					return nil, err
				}
				return zRangeReply(items, withScores), nil
			}, nil
		},
		empty: func(ctx context.Context) (any, error) { return []any{}, nil },
	},
	"ZRANGEBYSCORE": {
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if len(argv) < 3 {
				return nil, kvArgErr("wrong number of arguments")
			}
			key := argv[0]
			min, err := kvParseFloat(argv[1])
			if err != nil {
				return nil, err
			}
			max, err := kvParseFloat(argv[2])
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
						return nil, kvArgErr("LIMIT requires offset and count")
					}
					offset, err = kvParseInt(argv[i+1])
					if err != nil {
						return nil, err
					}
					count, err = kvParseInt(argv[i+2])
					if err != nil {
						return nil, err
					}
					i += 3
				default:
					return nil, kvArgErr("unknown option " + argv[i])
				}
			}
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				items, err := tx.ZSet().RangeByScore(ctx, key, min, max, offset, count)
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
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 3); err != nil {
				return nil, err
			}
			delta, err := kvParseFloat(argv[1])
			if err != nil {
				return nil, err
			}
			key, elem := argv[0], argv[2]
			return func(ctx context.Context, tx *kv.Tx) (any, error) {
				return tx.ZSet().Incr(ctx, key, []byte(elem), delta)
			}, nil
		},
	},
	"ZCOUNT": {
		parse: func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error) {
			if err := kvNeedArgs(argv, 3); err != nil {
				return nil, err
			}
			min, err := kvParseFloat(argv[1])
			if err != nil {
				return nil, err
			}
			max, err := kvParseFloat(argv[2])
			if err != nil {
				return nil, err
			}
			key := argv[0]
			return func(ctx context.Context, tx *kv.Tx) (any, error) { return tx.ZSet().CountByScore(ctx, key, min, max) }, nil
		},
		empty: func(ctx context.Context) (any, error) { return 0, nil },
	},
	"ZCARD": {
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
