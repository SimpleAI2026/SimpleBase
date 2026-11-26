package datahttp

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase/kv"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

// kvCmdSpec 描述一条命令。
type kvCmdSpec struct {
	// writable 为 true 时走写路径（Registry ReadWrite + Update + 惰性建表）。
	writable bool
	// parse 校验参数并返回执行函数（绑定 *kv.Tx）。
	parse func(argv []string) (func(ctx context.Context, tx *kv.Tx) (any, error), error)
	// empty 返回无 schema 空库时该读命令的回复（不触发建表）。
	empty func(ctx context.Context) (any, error)
}

// —— 参数解析辅助（错误统一为 kv_invalid_argument；只依赖 argv 与 context.Context，不依赖 Echo）——

// kvArgErr 返回参数错误（WriteError 统一映射为 400 kv_invalid_argument）；命令表不依赖 Echo。
func kvArgErr(msg string) error {
	return httpkit.NewKVArgumentError(msg)
}

// kvNeedArgs 校验剩余参数个数恰好为 n。
func kvNeedArgs(argv []string, n int) error {
	if len(argv) != n {
		return kvArgErr("wrong number of arguments")
	}
	return nil
}

// kvNeedAtLeast 校验剩余参数至少 n 个。
func kvNeedAtLeast(argv []string, n int) error {
	if len(argv) < n {
		return kvArgErr("wrong number of arguments")
	}
	return nil
}

func kvParseInt(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, kvArgErr("value is not an integer or out of range")
	}
	return n, nil
}

func kvParseFloat(s string) (float64, error) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, kvArgErr("value is not a float or out of range")
	}
	return f, nil
}

// kvCommandTable 是全部支持的命令，由各类型子表合并而成；重复命令名在包初始化时 panic。
var kvCommandTable = mergeKVCommands(kvKeyCommands, kvStringCommands, kvHashCommands, kvListCommands, kvSetCommands, kvZSetCommands)

// mergeKVCommands 合并命令子表，命令名重复视为编程错误。
func mergeKVCommands(parts ...map[string]kvCmdSpec) map[string]kvCmdSpec {
	out := make(map[string]kvCmdSpec)
	for _, part := range parts {
		for name, spec := range part {
			if _, dup := out[name]; dup {
				panic("kv: duplicate command " + name)
			}
			out[name] = spec
		}
	}
	return out
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
func parseScanArgs(argv []string) (cursor, pattern string, typ kv.TypeID, count int, err error) {
	if len(argv) == 0 {
		return "", "", 0, 0, kvArgErr("cursor is required")
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
				return "", "", 0, 0, kvArgErr("MATCH requires a pattern")
			}
			pattern = argv[i+1]
			i += 2
		case "COUNT":
			if i+1 >= len(argv) {
				return "", "", 0, 0, kvArgErr("COUNT requires a number")
			}
			n, perr := strconv.Atoi(argv[i+1])
			if perr != nil || n <= 0 || n > 500 {
				return "", "", 0, 0, kvArgErr("COUNT must be in 1..500")
			}
			count = n
			i += 2
		case "TYPE":
			if i+1 >= len(argv) {
				return "", "", 0, 0, kvArgErr("TYPE requires a name")
			}
			t, ok := kv.ParseType(argv[i+1])
			if !ok {
				return "", "", 0, 0, kvArgErr("unknown type")
			}
			typ = t
			i += 2
		default:
			return "", "", 0, 0, kvArgErr("unknown option " + argv[i])
		}
	}
	return cursor, pattern, typ, count, nil
}

// parseSetArgs 解析 SET 的选项：NX XX EX PX EXAT PXAT KEEPTTL GET。
func parseSetArgs(argv []string) (*kv.SetOptions, string, string, error) {
	if len(argv) < 2 {
		return nil, "", "", kvArgErr("wrong number of arguments")
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
				return nil, "", "", kvArgErr(strings.ToUpper(argv[i]) + " requires a number")
			}
			n, err := kvParseInt(argv[i+1])
			if err != nil {
				return nil, "", "", err
			}
			var ttlMs int64
			switch strings.ToUpper(argv[i]) {
			case "EX":
				if n <= 0 {
					return nil, "", "", kvArgErr("invalid expire time")
				}
				ttlMs = n * 1000
			case "PX":
				if n <= 0 {
					return nil, "", "", kvArgErr("invalid expire time")
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
			return nil, "", "", kvArgErr("unknown option " + argv[i])
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
	sort.Strings(fields)
	out := make([]any, 0, len(fields)*2)
	for _, f := range fields {
		out = append(out, f, string(items[f]))
	}
	return out
}

// hashScanReply 把 HSCAN 结果编码为 [cursor, [field, value, ...]]。
func hashScanReply(items map[string][]byte, next string) any {
	cursor := next
	if cursor == "" {
		cursor = "0"
	}
	return []any{cursor, hashFlatReply(items)}
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
