//go:build ignore

package main

import "time"

type TickInput struct{}
type Window struct { WindowStart string `json:"window_start"`; WindowEnd string `json:"window_end"` }
type Ticket struct { TicketID string `json:"ticket_id"`; Status string `json:"status"`; DueAt string `json:"due_at"`; Priority int `json:"priority"` }
type ComputeInput struct { Now string `json:"now"`; Rows []Ticket `json:"rows"` }
type Alert struct { TicketID string `json:"ticket_id"`; Severity string `json:"severity"` }
type Result struct { Items []Alert `json:"items"` }
func Tick(_ TickInput) Window { end := time.Now().UTC().Truncate(time.Hour); return Window{end.Add(-time.Hour).Format("2006-01-02T15:04:05Z07:00"), end.Format("2006-01-02T15:04:05Z07:00")} }
func Compute(in ComputeInput) Result {
  out := Result{Items: []Alert{}}
  for _, row := range in.Rows { if row.Status == "open" && row.DueAt < in.Now { severity := "warning"; if row.Priority == 1 { severity = "urgent" }; out.Items = append(out.Items, Alert{row.TicketID, severity}) } }
  return out
}
