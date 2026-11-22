package sqlval

import "time"

// BoolInt 把布尔映射为 1/0（SQLite/DuckDB 的整数布尔列）。
func BoolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// NullTime 把可空时间映射为参数：nil → NULL，否则统一按 UTC 落库。
func NullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC()
}

// NullTimeValue 把 time.Time 映射为参数：零值 → NULL，否则统一按 UTC 落库。
func NullTimeValue(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC()
}

// NullString 把空串映射为 NULL（可空文本列）。
func NullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
