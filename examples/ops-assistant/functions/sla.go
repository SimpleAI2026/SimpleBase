//go:build ignore

package main

import "time"

type Ticket struct { DueAt string `json:"due_at"`; Priority string `json:"priority"` }
type EvaluateInput struct { Tickets []Ticket `json:"tickets"` }
type EvaluateResult struct { Levels []string `json:"levels"`; Version int `json:"version"` }
func Evaluate(in EvaluateInput) EvaluateResult {
	out := EvaluateResult{Levels: []string{}, Version: 1}
	for _, ticket := range in.Tickets {
		level := "ok"
		if due, err := time.Parse("2006-01-02T15:04:05Z07:00", ticket.DueAt); err == nil && due.Before(time.Now()) { level = "overdue" }
		out.Levels = append(out.Levels, level)
	}
	return out
}
