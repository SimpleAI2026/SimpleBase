package database

import (
	"encoding/base64"
	"fmt"
	"time"
)

// SerializedValue 是列值的 JSON 安全表示。
// 标量类型（nil/int64/float64/bool/string/time.Time）直接放入 Value；
// []byte 使用 BlobValue 包装，避免被当作 UTF-8 字符串误判。
type SerializedValue struct {
	Value any
}

// BlobValue 表示二进制列值。JSON 编码为 {"type":"blob","base64":"..."}。
type BlobValue struct {
	Type   string `json:"type"`
	Base64 string `json:"base64"`
}

// SerializeValue 将驱动返回的列值转为 JSON 安全类型。
// 不支持的类型返回 ErrUnsupportedValue。
func SerializeValue(v any) (any, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case int64:
		return x, nil
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
	default:
		return nil, fmt.Errorf("%w: type %T", ErrUnsupportedValue, v)
	}
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
