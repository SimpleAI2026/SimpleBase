// 文件说明：serialize.go — godatabase 中 BlobValue、SerializeValue 等符号的实现
//
// 用法
//
//	type BlobValue struct{…}  // BlobValue 表示二进制列值
//	SerializeValue(v any) (any, error)  // SerializeValue 将驱动返回的列值转为 JSON 安全类型
//	SerializeRow(row []any) ([]any, error)  // SerializeRow 对一行驱动值逐列序列化
//	SerializeRows(rows [][]any) ([][]any, error)  // SerializeRows 对全部行执行序列化
//	同包调用：serializeMap() → coverage_test.go；serializeDuckMap() → coverage_test.go；mustSerialize() → coverage_test.go
//
// 调用架构
//
//	[上游] godatabase/cache, godatabase/cmd, godatabase/ducklake, godatabase/filedb 等 16 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/serialize.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] coverage_test.go, dialect.go, errmatch.go, query.go, query_test.go 等 10 个
//	[关键引用] BlobValue ← serialize_test.go；SerializeValue ← coverage_test.go, serialize_test.go；serializeMap ← coverage_test.go
//
// 主要符号
//
//	type BlobValue struct{…}
//	SerializeValue(v any) (any, error)
//	decodeJSON(raw json.RawMessage) (any, error)
//	serializeMap(in map[string]any) (map[string]any, error)
//	serializeDuckMap(m duckdb.Map) (any, error)
//	formatInterval(v duckdb.Interval) string
//	mustSerialize(v any) any
//	SerializeRow(row []any) ([]any, error)
//	... 另有 1 个顶层符号

package godatabase

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"time"

	duckdb "github.com/duckdb/duckdb-go/v2"
)

// BlobValue 表示二进制列值。JSON 编码为 {"type":"blob","base64":"..."}.
type BlobValue struct {
	Type   string `json:"type"`
	Base64 string `json:"base64"`
}

// SerializeValue 将驱动返回的列值转为 JSON 安全类型。
// DuckDB 类型映射见 planv2.0 §4.6。不支持的类型返回 ErrUnsupportedValue。
func SerializeValue(v any) (any, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case int:
		return int64(x), nil
	case int8:
		return int64(x), nil
	case int16:
		return int64(x), nil
	case int32:
		return int64(x), nil
	case int64:
		return x, nil
	case uint8:
		return int64(x), nil
	case uint16:
		return int64(x), nil
	case uint32:
		return int64(x), nil
	case uint64:
		return strconv.FormatUint(x, 10), nil
	case float32:
		return float64(x), nil
	case float64:
		return x, nil
	case bool:
		return x, nil
	case string:
		return x, nil
	case []byte:
		return BlobValue{Type: "blob", Base64: base64.StdEncoding.EncodeToString(x)}, nil
	case time.Time:
		return x.Format(time.RFC3339Nano), nil
	case *big.Int:
		if x == nil {
			return nil, nil
		}
		return x.String(), nil
	case json.Number:
		return x.String(), nil
	case json.RawMessage:
		return decodeJSON(x)
	case []any:
		return SerializeRow(x)
	case map[string]any:
		return serializeMap(x)
	case duckdb.Decimal:
		return x.String(), nil
	case duckdb.UUID:
		return x.String(), nil
	case *duckdb.UUID:
		if x == nil {
			return nil, nil
		}
		return x.String(), nil
	case duckdb.Interval:
		return formatInterval(x), nil
	case duckdb.Bit:
		return x.String(), nil
	case duckdb.Map:
		return serializeDuckMap(x)
	case duckdb.Union:
		return map[string]any{"tag": x.Tag, "value": mustSerialize(x.Value)}, nil
	default:
		if b, err := json.Marshal(v); err == nil && len(b) > 0 && (b[0] == '{' || b[0] == '[') {
			var out any
			if json.Unmarshal(b, &out) == nil {
				return out, nil
			}
		}
		return nil, fmt.Errorf("%w: type %T", ErrUnsupportedValue, v)
	}
}

func decodeJSON(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return string(raw), nil
	}
	return out, nil
}

func serializeMap(in map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(in))
	for k, v := range in {
		sv, err := SerializeValue(v)
		if err != nil {
			return nil, err
		}
		out[k] = sv
	}
	return out, nil
}

func serializeDuckMap(m duckdb.Map) (any, error) {
	out := make(map[string]any, len(m))
	for k, v := range m {
		sv, err := SerializeValue(v)
		if err != nil {
			return nil, err
		}
		out[fmt.Sprint(k)] = sv
	}
	return out, nil
}

func formatInterval(v duckdb.Interval) string {
	return fmt.Sprintf("%d months %d days %d micros", v.Months, v.Days, v.Micros)
}

func mustSerialize(v any) any {
	sv, err := SerializeValue(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return sv
}

// SerializeRow 对一行驱动值逐列序列化。
// 任一列失败则返回错误；调用方应记录并返回内部错误，不向客户端泄露类型细节。
func SerializeRow(row []any) ([]any, error) {
	out := make([]any, len(row))
	for i, v := range row {
		sv, err := SerializeValue(v)
		if err != nil {
			return nil, err
		}
		out[i] = sv
	}
	return out, nil
}

// SerializeRows 对全部行执行序列化。
func SerializeRows(rows [][]any) ([][]any, error) {
	out := make([][]any, len(rows))
	for i, row := range rows {
		sr, err := SerializeRow(row)
		if err != nil {
			return nil, err
		}
		out[i] = sr
	}
	return out, nil
}
