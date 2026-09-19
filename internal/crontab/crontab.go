// Package crontab 提供受限 5 字段 cron 解析与下次触发计算（ui-cronjob-plan §5.1）。
// 从 internal/cloudagent/cron.go 迁移而来，纯函数零依赖；cloudagent 保留薄封装兼容。
package crontab

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronSpec 是受限 5 字段 cron（分 时 日 月 周），分钟粒度，一律按 UTC 解释。
// 支持 *、*/n、n、a-b 与逗号列表组合；不支持名字（JAN/MON）与步进区间（a-b/n）。
type CronSpec struct {
	minutes  map[int]struct{}
	hours    map[int]struct{}
	days     map[int]struct{}
	months   map[int]struct{}
	weekdays map[int]struct{}
}

const cronSearchLimit = 2 * 365 * 24 * 60 // NextAfter 逐分钟搜索上限（2 年）

// ParseCron 解析并校验 cron 表达式。
func ParseCron(expr string) (CronSpec, error) {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return CronSpec{}, fmt.Errorf("invalid cron expression: want 5 fields (min hour day month weekday), got %d", len(fields))
	}
	ranges := []struct {
		text string
		lo   int
		hi   int
	}{
		{fields[0], 0, 59},
		{fields[1], 0, 23},
		{fields[2], 1, 31},
		{fields[3], 1, 12},
		{fields[4], 0, 7}, // 0 与 7 均为周日
	}
	sets := make([]map[int]struct{}, 0, 5)
	for _, r := range ranges {
		set, err := parseCronField(r.text, r.lo, r.hi)
		if err != nil {
			return CronSpec{}, err
		}
		sets = append(sets, set)
	}
	// 周字段：7 归一化为 0（周日）。
	if _, ok := sets[4][7]; ok {
		delete(sets[4], 7)
		sets[4][0] = struct{}{}
	}
	return CronSpec{minutes: sets[0], hours: sets[1], days: sets[2], months: sets[3], weekdays: sets[4]}, nil
}

// NextAfter 返回 t 之后（严格大于 t）的第一个触发时刻。
// 日与周同时受限时取并集（标准 cron 语义）；找不到（如 2 月 31 日）返回错误。
func (c CronSpec) NextAfter(t time.Time) (time.Time, error) {
	utc := t.UTC().Truncate(time.Minute)
	cursor := utc.Add(time.Minute)
	limit := utc.Add(time.Duration(cronSearchLimit) * time.Minute)
	for !cursor.After(limit) {
		if c.matches(cursor) {
			return cursor, nil
		}
		cursor = cursor.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("cron expression has no next occurrence within %d minutes", cronSearchLimit)
}

func (c CronSpec) matches(t time.Time) bool {
	if _, ok := c.minutes[t.Minute()]; !ok {
		return false
	}
	if _, ok := c.hours[t.Hour()]; !ok {
		return false
	}
	if _, ok := c.months[int(t.Month())]; !ok {
		return false
	}
	dayRestricted := len(c.days) < 31
	weekdayRestricted := len(c.weekdays) < 7
	switch {
	case dayRestricted && weekdayRestricted:
		// 标准语义：两者同时受限时取并集。
		if _, ok := c.days[t.Day()]; ok {
			return true
		}
		_, ok := c.weekdays[int(t.Weekday())]
		return ok
	case dayRestricted:
		_, ok := c.days[t.Day()]
		return ok
	case weekdayRestricted:
		_, ok := c.weekdays[int(t.Weekday())]
		return ok
	default:
		return true
	}
}

func parseCronField(text string, lo, hi int) (map[int]struct{}, error) {
	out := map[int]struct{}{}
	for _, part := range strings.Split(text, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("invalid cron field %q: empty list item", text)
		}
		step := 1
		rangeText := part
		if idx := strings.Index(part, "/"); idx >= 0 {
			rangeText = part[:idx]
			stepText := part[idx+1:]
			if stepText == "" {
				return nil, fmt.Errorf("invalid cron field %q: empty step", text)
			}
			n, err := strconv.Atoi(stepText)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("invalid cron field %q: bad step %q", text, stepText)
			}
			step = n
		}
		start, end := lo, hi
		switch {
		case rangeText == "*":
			// 全区间
		case strings.Contains(rangeText, "-"):
			bounds := strings.SplitN(rangeText, "-", 2)
			a, err1 := strconv.Atoi(bounds[0])
			b, err2 := strconv.Atoi(bounds[1])
			if err1 != nil || err2 != nil || len(bounds) != 2 {
				return nil, fmt.Errorf("invalid cron field %q: bad range %q", text, rangeText)
			}
			if a < lo || b > hi || a > b {
				return nil, fmt.Errorf("invalid cron field %q: range %q out of [%d,%d]", text, rangeText, lo, hi)
			}
			start, end = a, b
		default:
			n, err := strconv.Atoi(rangeText)
			if err != nil {
				return nil, fmt.Errorf("invalid cron field %q: bad value %q", text, rangeText)
			}
			if n < lo || n > hi {
				return nil, fmt.Errorf("invalid cron field %q: value %d out of [%d,%d]", text, n, lo, hi)
			}
			start, end = n, n
		}
		if rangeText == "*" && step == 1 {
			for v := lo; v <= hi; v++ {
				out[v] = struct{}{}
			}
			continue
		}
		for v := start; v <= end; v += step {
			out[v] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("invalid cron field %q: matches nothing", text)
	}
	return out, nil
}
