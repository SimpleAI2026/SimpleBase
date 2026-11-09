//go:build ignore

package main

import (
	"time"
)

type Ticket struct {
	ID       string `json:"id"`
	DueAt    string `json:"due_at"`
	Priority string `json:"priority"`
	Status   string `json:"status"`
}

type EvaluateInput struct {
	Tickets []Ticket `json:"tickets"`
}

type EvaluateResult struct {
	Levels  []string `json:"levels"`
	Overdue []string `json:"overdue"`
	Version int      `json:"version"`
	Error   string   `json:"error,omitempty"`
}

// Evaluate 按 SLA 截止时间判定工单等级：逾期 overdue、1 小时内 due_soon、其余 ok。
func Evaluate(in EvaluateInput) EvaluateResult {
	out := EvaluateResult{Levels: []string{}, Overdue: []string{}, Version: 2}
	now := time.Now()
	for i := 0; i < len(in.Tickets); i++ {
		ticket := in.Tickets[i]
		if ticket.Status == "closed" {
			out.Levels = append(out.Levels, "closed")
			continue
		}
		due, err := time.Parse("2006-01-02T15:04:05Z07:00", ticket.DueAt)
		if err != nil {
			out.Levels = append(out.Levels, "invalid_due")
			continue
		}
		level := "ok"
		if due.Before(now) {
			level = "overdue"
			out.Overdue = append(out.Overdue, ticket.ID)
		} else if due.Sub(now) <= time.Hour {
			level = "due_soon"
		}
		out.Levels = append(out.Levels, level)
	}
	return out
}

type SummaryInput struct {
	Open    int64 `json:"open"`
	Overdue int64 `json:"overdue"`
	Total   int64 `json:"total"`
}

type SummaryResult struct {
	Text  string `json:"text"`
	Level string `json:"level"`
}

// Summary 生成一条可直接展示的 SLA 概览文案，供 BFF 与助手使用。
func Summary(in SummaryInput) SummaryResult {
	level := "ok"
	if in.Overdue > 0 {
		level = "warn"
	}
	if in.Overdue >= 5 {
		level = "critical"
	}
	text := "工单总数 0"
	if in.Total > 0 {
		text = "工单 " + itoa(in.Total) + " 个，未关闭 " + itoa(in.Open) + " 个，逾期 " + itoa(in.Overdue) + " 个"
	}
	return SummaryResult{Text: text, Level: level}
}

// itoa 只转非负整数（解释器不含 strconv 的完整实现时也能用）。
func itoa(value int64) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 20)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}