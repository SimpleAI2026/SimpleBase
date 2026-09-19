package crontab

import (
	"testing"
	"time"
)

func mustParseCron(t *testing.T, expr string) CronSpec {
	t.Helper()
	spec, err := ParseCron(expr)
	if err != nil {
		t.Fatalf("ParseCron(%q) error: %v", expr, err)
	}
	return spec
}

func TestParseCronValid(t *testing.T) {
	cases := []string{
		"*/15 * * * *",
		"0 * * * *",
		"0 8 * * *",
		"0 8 * * 1",
		"0 8 * * 1,4",
		"0 8,20 * * 1-5",
		"30 4 1 * *",
		"0 0 29 2 *",
		"7 7 7 7 7",
		"* * * * *",
	}
	for _, expr := range cases {
		if _, err := ParseCron(expr); err != nil {
			t.Errorf("ParseCron(%q) unexpected error: %v", expr, err)
		}
	}
}

func TestParseCronInvalid(t *testing.T) {
	cases := []string{
		"",
		"* * * *",
		"* * * * * *",
		"60 * * * *",
		"* 24 * * *",
		"0 * 0 * *",
		"0 * 32 * *",
		"* * * 0 *",
		"* * * 13 *",
		"* * * * 8",
		"a * * * *",
		"*/0 * * * *",
		"1- * * * *",
		"5-1 * * * *",
		"1,,2 * * * *",
		"*/x * * * *",
	}
	for _, expr := range cases {
		if _, err := ParseCron(expr); err == nil {
			t.Errorf("ParseCron(%q) expected error, got nil", expr)
		}
	}
}

func TestNextAfterEvery15(t *testing.T) {
	spec := mustParseCron(t, "*/15 * * * *")
	base := time.Date(2026, 9, 18, 7, 59, 30, 0, time.UTC)
	next, err := spec.NextAfter(base)
	if err != nil {
		t.Fatalf("NextAfter: %v", err)
	}
	if want := time.Date(2026, 9, 18, 8, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Errorf("next = %v, want %v", next, want)
	}
}

func TestNextAfterDaily8UTC(t *testing.T) {
	spec := mustParseCron(t, "0 8 * * *")
	// 15:30 UTC → 次日 08:00
	base := time.Date(2026, 9, 18, 15, 30, 0, 0, time.UTC)
	next, err := spec.NextAfter(base)
	if err != nil {
		t.Fatalf("NextAfter: %v", err)
	}
	if want := time.Date(2026, 9, 19, 8, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Errorf("next = %v, want %v", next, want)
	}
	// 恰在 08:00 整点 → 严格大于，返回次日
	base = time.Date(2026, 9, 18, 8, 0, 0, 0, time.UTC)
	next, err = spec.NextAfter(base)
	if err != nil {
		t.Fatalf("NextAfter: %v", err)
	}
	if want := time.Date(2026, 9, 19, 8, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Errorf("next = %v, want %v", next, want)
	}
}

func TestNextAfterWeekdayList(t *testing.T) {
	spec := mustParseCron(t, "0 8 * * 1,4") // 周一、周四 08:00
	// 2026-09-18 是周五 → 下一个周一是 2026-09-21
	base := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	next, err := spec.NextAfter(base)
	if err != nil {
		t.Fatalf("NextAfter: %v", err)
	}
	if want := time.Date(2026, 9, 21, 8, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Errorf("next = %v, want %v", next, want)
	}
}

func TestNextAfterSundaySeven(t *testing.T) {
	spec := mustParseCron(t, "0 8 * * 7") // 7 归一化为周日
	// 2026-09-18 是周五 → 下一个周日是 09-20
	base := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	next, err := spec.NextAfter(base)
	if err != nil {
		t.Fatalf("NextAfter: %v", err)
	}
	if want := time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Errorf("next = %v, want %v", next, want)
	}
}

func TestNextAfterDayAndWeekdayUnion(t *testing.T) {
	// 日=1 且 周=周一：取并集 → 每月 1 号或每个周一的 08:00
	spec := mustParseCron(t, "0 8 1 * 1")
	// 2026-09-18 是周五 → 下一个周一是 09-21（先于 10-01）
	base := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	next, err := spec.NextAfter(base)
	if err != nil {
		t.Fatalf("NextAfter: %v", err)
	}
	if want := time.Date(2026, 9, 21, 8, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Errorf("next = %v, want %v", next, want)
	}
	// 从 09-22（周二）出发 → 下一个 1 号（10-01）是周四
	base = time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	next, err = spec.NextAfter(base)
	if err != nil {
		t.Fatalf("NextAfter: %v", err)
	}
	// 2026-09-28 是周一，先于 10-01
	if want := time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Errorf("next = %v, want %v", next, want)
	}
}

func TestNextAfterNonUTCInput(t *testing.T) {
	spec := mustParseCron(t, "0 8 * * *")
	// 东八区 2026-09-18 16:30 == UTC 08:30 → 下一次为 UTC 次日 08:00
	base := time.Date(2026, 9, 18, 16, 30, 0, 0, time.FixedZone("CST", 8*3600))
	next, err := spec.NextAfter(base)
	if err != nil {
		t.Fatalf("NextAfter: %v", err)
	}
	if want := time.Date(2026, 9, 19, 8, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Errorf("next = %v, want %v", next, want)
	}
}

func TestNextAfterImpossible(t *testing.T) {
	spec := mustParseCron(t, "0 0 30 2 *") // 2 月 30 日不存在
	_, err := spec.NextAfter(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Error("expected error for impossible cron")
	}
}
