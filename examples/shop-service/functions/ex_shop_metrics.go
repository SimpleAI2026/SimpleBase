//go:build ignore

package main

import "time"

type TickInput struct{}
type Window struct {
  WindowStart string `json:"window_start"`
  WindowEnd string `json:"window_end"`
}
type Sale struct { Qty int64 `json:"qty"`; AmountMinor int64 `json:"amount_minor"` }
type ComputeInput struct { Rows []Sale `json:"rows"` }
type Summary struct { Units int64 `json:"units"`; AmountMinor int64 `json:"amount_minor"` }

func Tick(_ TickInput) Window {
  now := time.Now().UTC()
  end := now.Truncate(24 * time.Hour)
  return Window{WindowStart: end.Add(-24 * time.Hour).Format("2006-01-02T15:04:05Z07:00"), WindowEnd: end.Format("2006-01-02T15:04:05Z07:00")}
}
func Compute(in ComputeInput) Summary {
  var result Summary
  for _, row := range in.Rows { result.Units += row.Qty; result.AmountMinor += row.AmountMinor }
  return result
}
